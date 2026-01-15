// Package floors provides price floor enforcement for bid requests
package floors

import (
	"context"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// FloorRule represents a price floor rule
type FloorRule struct {
	// Matching criteria (all non-empty fields must match)
	PublisherID string   `json:"publisher_id,omitempty"`
	Domain      string   `json:"domain,omitempty"`
	AdUnitCode  string   `json:"ad_unit_code,omitempty"`
	MediaType   string   `json:"media_type,omitempty"` // banner, video, native, audio
	Size        string   `json:"size,omitempty"`       // WxH format, e.g., "300x250"
	Country     string   `json:"country,omitempty"`    // ISO 3166-1 alpha-2
	DeviceType  string   `json:"device_type,omitempty"` // desktop, mobile, tablet, ctv

	// Floor value
	Floor    float64 `json:"floor"`
	Currency string  `json:"currency,omitempty"` // Default: USD
}

// FloorData contains floor rules for a publisher/domain
type FloorData struct {
	Rules       []FloorRule `json:"rules"`
	DefaultFloor float64    `json:"default_floor,omitempty"`
	Currency     string     `json:"currency,omitempty"` // Default currency for rules
	SchemaVersion int       `json:"schema_version,omitempty"`
	FetchedAt    time.Time  `json:"fetched_at,omitempty"`
	TTL          time.Duration `json:"ttl,omitempty"`
}

// FloorResult is the result of a floor lookup
type FloorResult struct {
	Floor       float64
	Currency    string
	RuleMatched *FloorRule // The rule that matched, nil if default
	Source      string     // "rule", "default", "imp", "none"
}

// Provider is the interface for floor data sources
// Implement this to integrate with pubX or other floor providers
type Provider interface {
	// GetFloors fetches floor data for a request
	// Returns nil if no floors are configured
	GetFloors(ctx context.Context, req *openrtb.BidRequest) (*FloorData, error)

	// Name returns the provider name for logging
	Name() string
}

// Enforcer enforces price floors on bid requests and responses
type Enforcer struct {
	mu        sync.RWMutex
	providers []Provider
	cache     *floorCache
	config    *Config
}

// Config holds floor enforcer configuration
type Config struct {
	// Enabled controls whether floor enforcement is active
	Enabled bool `json:"enabled"`

	// EnforceFloors controls whether bids below floor are rejected
	// If false, floors are only logged (soft floor mode)
	EnforceFloors bool `json:"enforce_floors"`

	// UseDynamicData enables fetching floors from providers
	// If false, only uses bid request floor data
	UseDynamicData bool `json:"use_dynamic_data"`

	// DefaultFloor is used when no floor is found
	DefaultFloor float64 `json:"default_floor"`

	// DefaultCurrency for floors without specified currency
	DefaultCurrency string `json:"default_currency"`

	// CacheTTL for floor data caching
	CacheTTL time.Duration `json:"cache_ttl"`

	// FetchTimeout for provider requests
	FetchTimeout time.Duration `json:"fetch_timeout"`
}

// DefaultConfig returns production-safe defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		EnforceFloors:   true,
		UseDynamicData:  true,
		DefaultFloor:    0.0, // No default floor
		DefaultCurrency: "USD",
		CacheTTL:        5 * time.Minute,
		FetchTimeout:    100 * time.Millisecond,
	}
}

// NewEnforcer creates a new floor enforcer
func NewEnforcer(config *Config, providers ...Provider) *Enforcer {
	if config == nil {
		config = DefaultConfig()
	}

	return &Enforcer{
		providers: providers,
		cache:     newFloorCache(config.CacheTTL),
		config:    config,
	}
}

// AddProvider adds a floor provider
func (e *Enforcer) AddProvider(p Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers = append(e.providers, p)
}

// EnrichRequest adds floor data to the bid request impressions
func (e *Enforcer) EnrichRequest(ctx context.Context, req *openrtb.BidRequest) error {
	if !e.config.Enabled || req == nil {
		return nil
	}

	// Fetch floor data from providers
	floorData := e.fetchFloors(ctx, req)
	if floorData == nil {
		return nil
	}

	// Apply floors to each impression
	for i := range req.Imp {
		imp := &req.Imp[i]
		result := e.getFloorForImp(floorData, req, imp)

		// Only update if we found a floor and imp doesn't already have one
		if result.Floor > 0 && imp.BidFloor == 0 {
			imp.BidFloor = result.Floor
			imp.BidFloorCur = result.Currency
			if imp.BidFloorCur == "" {
				imp.BidFloorCur = e.config.DefaultCurrency
			}
		}
	}

	return nil
}

// ValidateBid checks if a bid meets the floor requirement
// Returns true if bid is valid (meets or exceeds floor)
func (e *Enforcer) ValidateBid(bid *openrtb.Bid, imp *openrtb.Imp, bidCurrency string) (bool, string) {
	if !e.config.Enabled {
		return true, ""
	}

	floor := imp.BidFloor
	floorCur := imp.BidFloorCur
	if floorCur == "" {
		floorCur = e.config.DefaultCurrency
	}

	// No floor set
	if floor <= 0 {
		return true, ""
	}

	bidPrice := bid.Price

	// TODO: Currency conversion if bidCurrency != floorCur
	// For now, assume same currency or skip validation
	if bidCurrency != "" && bidCurrency != floorCur {
		logger.Log.Debug().
			Str("bid_currency", bidCurrency).
			Str("floor_currency", floorCur).
			Float64("bid_price", bidPrice).
			Float64("floor", floor).
			Msg("Skipping floor validation due to currency mismatch - currency conversion not yet implemented")
		return true, ""
	}

	if bidPrice < floor {
		reason := "bid_below_floor"
		if !e.config.EnforceFloors {
			// Soft floor mode - log but don't reject
			logger.Log.Info().
				Str("imp_id", imp.ID).
				Float64("bid_price", bidPrice).
				Float64("floor", floor).
				Str("currency", floorCur).
				Msg("Bid below floor (soft mode - not rejected)")
			return true, ""
		}
		return false, reason
	}

	return true, ""
}

// fetchFloors gets floor data from providers or cache
func (e *Enforcer) fetchFloors(ctx context.Context, req *openrtb.BidRequest) *FloorData {
	if !e.config.UseDynamicData {
		return nil
	}

	// Generate cache key
	cacheKey := e.getCacheKey(req)

	// Check cache first
	if cached := e.cache.get(cacheKey); cached != nil {
		return cached
	}

	// Fetch from providers
	e.mu.RLock()
	providers := e.providers
	e.mu.RUnlock()

	// Use timeout context for fetching
	fetchCtx, cancel := context.WithTimeout(ctx, e.config.FetchTimeout)
	defer cancel()

	for _, provider := range providers {
		floorData, err := provider.GetFloors(fetchCtx, req)
		if err != nil {
			logger.Log.Debug().
				Err(err).
				Str("provider", provider.Name()).
				Msg("Failed to fetch floors from provider")
			continue
		}

		if floorData != nil && len(floorData.Rules) > 0 {
			// Cache the result
			ttl := floorData.TTL
			if ttl == 0 {
				ttl = e.config.CacheTTL
			}
			e.cache.set(cacheKey, floorData, ttl)
			return floorData
		}
	}

	return nil
}

// getFloorForImp finds the best matching floor for an impression
func (e *Enforcer) getFloorForImp(data *FloorData, req *openrtb.BidRequest, imp *openrtb.Imp) FloorResult {
	result := FloorResult{
		Currency: data.Currency,
		Source:   "none",
	}

	if result.Currency == "" {
		result.Currency = e.config.DefaultCurrency
	}

	// Build match criteria from request/imp
	criteria := e.buildMatchCriteria(req, imp)

	// Find best matching rule (most specific match wins)
	var bestRule *FloorRule
	bestScore := -1

	for i := range data.Rules {
		rule := &data.Rules[i]
		score := e.matchScore(rule, criteria)
		if score > bestScore {
			bestScore = score
			bestRule = rule
		}
	}

	if bestRule != nil && bestScore > 0 {
		result.Floor = bestRule.Floor
		result.RuleMatched = bestRule
		result.Source = "rule"
		if bestRule.Currency != "" {
			result.Currency = bestRule.Currency
		}
		return result
	}

	// Fall back to default floor
	if data.DefaultFloor > 0 {
		result.Floor = data.DefaultFloor
		result.Source = "default"
		return result
	}

	// Check imp's existing floor
	if imp.BidFloor > 0 {
		result.Floor = imp.BidFloor
		result.Currency = imp.BidFloorCur
		result.Source = "imp"
		return result
	}

	return result
}

// matchCriteria holds values to match against rules
type matchCriteria struct {
	PublisherID string
	Domain      string
	AdUnitCode  string
	MediaType   string
	Size        string
	Country     string
	DeviceType  string
}

// buildMatchCriteria extracts matching criteria from request/imp
func (e *Enforcer) buildMatchCriteria(req *openrtb.BidRequest, imp *openrtb.Imp) matchCriteria {
	c := matchCriteria{}

	// Publisher ID
	if req.Site != nil && req.Site.Publisher != nil {
		c.PublisherID = req.Site.Publisher.ID
	} else if req.App != nil && req.App.Publisher != nil {
		c.PublisherID = req.App.Publisher.ID
	}

	// Domain
	if req.Site != nil {
		c.Domain = req.Site.Domain
	} else if req.App != nil {
		c.Domain = req.App.Bundle
	}

	// Ad unit code from tagid
	c.AdUnitCode = imp.TagID

	// Media type
	if imp.Banner != nil {
		c.MediaType = "banner"
		if imp.Banner.W > 0 && imp.Banner.H > 0 {
			c.Size = formatSize(imp.Banner.W, imp.Banner.H)
		}
	} else if imp.Video != nil {
		c.MediaType = "video"
		if imp.Video.W > 0 && imp.Video.H > 0 {
			c.Size = formatSize(imp.Video.W, imp.Video.H)
		}
	} else if imp.Native != nil {
		c.MediaType = "native"
	} else if imp.Audio != nil {
		c.MediaType = "audio"
	}

	// Country
	if req.Device != nil && req.Device.Geo != nil {
		c.Country = req.Device.Geo.Country
	}

	// Device type
	if req.Device != nil {
		c.DeviceType = mapDeviceType(req.Device.DeviceType)
	}

	return c
}

// matchScore calculates how well a rule matches the criteria
// Higher score = more specific match, 0 = no match, -1 = mismatch
func (e *Enforcer) matchScore(rule *FloorRule, c matchCriteria) int {
	score := 0

	// Each matching field adds to score
	// Non-matching non-empty field means no match
	if rule.PublisherID != "" {
		if rule.PublisherID == c.PublisherID {
			score += 10
		} else {
			return -1
		}
	}

	if rule.Domain != "" {
		if rule.Domain == c.Domain {
			score += 8
		} else {
			return -1
		}
	}

	if rule.AdUnitCode != "" {
		if rule.AdUnitCode == c.AdUnitCode {
			score += 6
		} else {
			return -1
		}
	}

	if rule.MediaType != "" {
		if rule.MediaType == c.MediaType {
			score += 4
		} else {
			return -1
		}
	}

	if rule.Size != "" {
		if rule.Size == c.Size {
			score += 3
		} else {
			return -1
		}
	}

	if rule.Country != "" {
		if rule.Country == c.Country {
			score += 2
		} else {
			return -1
		}
	}

	if rule.DeviceType != "" {
		if rule.DeviceType == c.DeviceType {
			score += 1
		} else {
			return -1
		}
	}

	return score
}

// getCacheKey generates a cache key for floor lookups
func (e *Enforcer) getCacheKey(req *openrtb.BidRequest) string {
	var pubID, domain string
	if req.Site != nil {
		domain = req.Site.Domain
		if req.Site.Publisher != nil {
			pubID = req.Site.Publisher.ID
		}
	} else if req.App != nil {
		domain = req.App.Bundle
		if req.App.Publisher != nil {
			pubID = req.App.Publisher.ID
		}
	}
	return pubID + ":" + domain
}

// formatSize formats width and height as "WxH"
func formatSize(w, h int) string {
	return itoa(w) + "x" + itoa(h)
}

// itoa converts int to string without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// mapDeviceType maps OpenRTB device type to string
func mapDeviceType(deviceType int) string {
	switch deviceType {
	case 1:
		return "mobile"
	case 2:
		return "desktop"
	case 3:
		return "ctv"
	case 4:
		return "phone"
	case 5:
		return "tablet"
	case 6:
		return "connected_device"
	case 7:
		return "set_top_box"
	default:
		return ""
	}
}

// floorCache provides thread-safe caching for floor data
type floorCache struct {
	mu      sync.RWMutex
	data    map[string]*cacheEntry
	defaultTTL time.Duration
}

type cacheEntry struct {
	data      *FloorData
	expiresAt time.Time
}

func newFloorCache(defaultTTL time.Duration) *floorCache {
	return &floorCache{
		data:       make(map[string]*cacheEntry),
		defaultTTL: defaultTTL,
	}
}

func (c *floorCache) get(key string) *FloorData {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.data
}

func (c *floorCache) set(key string, data *FloorData, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl == 0 {
		ttl = c.defaultTTL
	}

	c.data[key] = &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
}

func (c *floorCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*cacheEntry)
}

// GetConfig returns current configuration
func (e *Enforcer) GetConfig() *Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

// SetEnabled enables/disables floor enforcement
func (e *Enforcer) SetEnabled(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.Enabled = enabled
}

// ClearCache clears the floor cache
func (e *Enforcer) ClearCache() {
	e.cache.clear()
}
