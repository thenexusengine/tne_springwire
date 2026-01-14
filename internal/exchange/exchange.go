// Package exchange implements the auction exchange
package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/adapters/ortb"
	"github.com/thenexusengine/tne_springwire/internal/fpd"
	"github.com/thenexusengine/tne_springwire/internal/middleware"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/idr"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// MetricsRecorder interface for recording revenue/margin metrics
type MetricsRecorder interface {
	RecordMargin(publisher, bidder, mediaType string, originalPrice, adjustedPrice, platformCut float64)
	RecordFloorAdjustment(publisher string)
}

// Exchange orchestrates the auction process
type Exchange struct {
	registry         *adapters.Registry
	dynamicRegistry  *ortb.DynamicRegistry
	httpClient       adapters.HTTPClient
	idrClient        *idr.Client
	eventRecorder    *idr.EventRecorder
	config           *Config
	fpdProcessor     *fpd.Processor
	eidFilter        *fpd.EIDFilter
	metrics          MetricsRecorder

	// configMu protects dynamicRegistry, fpdProcessor, eidFilter, and config.FPD
	// for safe concurrent access during runtime config updates
	configMu sync.RWMutex
}

// AuctionType defines the type of auction to run
type AuctionType int

const (
	// FirstPriceAuction - winner pays their bid price
	FirstPriceAuction AuctionType = 1
	// SecondPriceAuction - winner pays second highest bid + increment
	SecondPriceAuction AuctionType = 2
)

// Default clone allocation limits (P1-3: prevent OOM from malicious requests)
// P3-1: These are now defaults; can be overridden via CloneLimits config
const (
	defaultMaxImpressionsPerRequest = 100 // Maximum impressions to clone
	defaultMaxEIDsPerUser           = 50  // Maximum EIDs to clone
	defaultMaxDataPerUser           = 20  // Maximum Data segments to clone
	defaultMaxDealsPerImp           = 50  // Maximum deals per impression
	defaultMaxSChainNodes           = 20  // Maximum supply chain nodes
)

// P1-4: Timeout bounds for dynamic adapter validation
const (
	minBidderTimeout = 10 * time.Millisecond  // Minimum reasonable timeout
	maxBidderTimeout = 5 * time.Second        // Maximum to prevent resource exhaustion
)

// maxAllowedTMax caps TMax at a reasonable maximum to prevent resource exhaustion (10 seconds)
const maxAllowedTMax = 10000

// P2-7: NBR codes consolidated in openrtb/response.go
// Use openrtb.NoBidXxx constants for all no-bid reasons

// CloneLimits holds configurable limits for request cloning (P3-1)
type CloneLimits struct {
	MaxImpressionsPerRequest int // Maximum impressions to clone (default: 100)
	MaxEIDsPerUser           int // Maximum EIDs to clone (default: 50)
	MaxDataPerUser           int // Maximum Data segments to clone (default: 20)
	MaxDealsPerImp           int // Maximum deals per impression (default: 50)
	MaxSChainNodes           int // Maximum supply chain nodes (default: 20)
}

// DefaultCloneLimits returns default clone limits
func DefaultCloneLimits() *CloneLimits {
	return &CloneLimits{
		MaxImpressionsPerRequest: defaultMaxImpressionsPerRequest,
		MaxEIDsPerUser:           defaultMaxEIDsPerUser,
		MaxDataPerUser:           defaultMaxDataPerUser,
		MaxDealsPerImp:           defaultMaxDealsPerImp,
		MaxSChainNodes:           defaultMaxSChainNodes,
	}
}

// Config holds exchange configuration
type Config struct {
	DefaultTimeout       time.Duration
	MaxBidders           int
	MaxConcurrentBidders int // P0-4: Limit concurrent bidder goroutines (0 = unlimited)
	IDREnabled           bool
	IDRServiceURL        string
	IDRAPIKey            string // Internal API key for IDR service-to-service calls
	EventRecordEnabled   bool
	EventBufferSize      int
	CurrencyConv         bool
	DefaultCurrency      string
	FPD                  *fpd.Config
	CloneLimits          *CloneLimits // P3-1: Configurable clone limits
	// Dynamic bidder configuration
	DynamicBiddersEnabled bool
	DynamicRefreshPeriod  time.Duration
	// Auction configuration
	AuctionType    AuctionType
	PriceIncrement float64 // For second-price auctions (typically 0.01)
	MinBidPrice    float64 // Minimum valid bid price
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultTimeout:        1000 * time.Millisecond,
		MaxBidders:            50,
		MaxConcurrentBidders:  10, // P0-4: Limit concurrent HTTP requests per auction
		IDREnabled:            true,
		IDRServiceURL:         "http://localhost:5050",
		EventRecordEnabled:    true,
		EventBufferSize:       100,
		CurrencyConv:          false,
		DefaultCurrency:       "USD",
		FPD:                   fpd.DefaultConfig(),
		CloneLimits:           DefaultCloneLimits(), // P3-1: Configurable clone limits
		DynamicBiddersEnabled: true,
		DynamicRefreshPeriod:  30 * time.Second,
		AuctionType:           FirstPriceAuction,
		PriceIncrement:        0.01,
		MinBidPrice:           0.0,
	}
}

// validateConfig validates config values and applies sensible defaults for invalid values
// P1-2: Prevent runtime panics or silent failures from bad configuration
func validateConfig(config *Config) *Config {
	defaults := DefaultConfig()

	// Timeout must be positive
	if config.DefaultTimeout <= 0 {
		config.DefaultTimeout = defaults.DefaultTimeout
	}

	// MaxBidders must be positive
	if config.MaxBidders <= 0 {
		config.MaxBidders = defaults.MaxBidders
	}

	// MaxConcurrentBidders must be non-negative (0 means unlimited)
	if config.MaxConcurrentBidders < 0 {
		config.MaxConcurrentBidders = defaults.MaxConcurrentBidders
	}

	// AuctionType must be valid
	if config.AuctionType != FirstPriceAuction && config.AuctionType != SecondPriceAuction {
		config.AuctionType = FirstPriceAuction
	}

	// PriceIncrement must be positive for second-price auctions
	if config.AuctionType == SecondPriceAuction && config.PriceIncrement <= 0 {
		config.PriceIncrement = defaults.PriceIncrement
	}

	// MinBidPrice should not be negative
	if config.MinBidPrice < 0 {
		config.MinBidPrice = 0
	}

	// EventBufferSize must be positive if event recording is enabled
	if config.EventRecordEnabled && config.EventBufferSize <= 0 {
		config.EventBufferSize = defaults.EventBufferSize
	}

	// P3-1: Initialize CloneLimits if nil and validate values
	if config.CloneLimits == nil {
		config.CloneLimits = DefaultCloneLimits()
	} else {
		defaultLimits := DefaultCloneLimits()
		if config.CloneLimits.MaxImpressionsPerRequest <= 0 {
			config.CloneLimits.MaxImpressionsPerRequest = defaultLimits.MaxImpressionsPerRequest
		}
		if config.CloneLimits.MaxEIDsPerUser <= 0 {
			config.CloneLimits.MaxEIDsPerUser = defaultLimits.MaxEIDsPerUser
		}
		if config.CloneLimits.MaxDataPerUser <= 0 {
			config.CloneLimits.MaxDataPerUser = defaultLimits.MaxDataPerUser
		}
		if config.CloneLimits.MaxDealsPerImp <= 0 {
			config.CloneLimits.MaxDealsPerImp = defaultLimits.MaxDealsPerImp
		}
		if config.CloneLimits.MaxSChainNodes <= 0 {
			config.CloneLimits.MaxSChainNodes = defaultLimits.MaxSChainNodes
		}
	}

	return config
}

// New creates a new exchange
func New(registry *adapters.Registry, config *Config) *Exchange {
	if config == nil {
		config = DefaultConfig()
	}

	// P1-2: Validate and apply defaults for critical config fields
	config = validateConfig(config)

	// Initialize FPD config if not provided
	fpdConfig := config.FPD
	if fpdConfig == nil {
		fpdConfig = fpd.DefaultConfig()
	}

	ex := &Exchange{
		registry:     registry,
		httpClient:   adapters.NewHTTPClient(config.DefaultTimeout),
		config:       config,
		fpdProcessor: fpd.NewProcessor(fpdConfig),
		eidFilter:    fpd.NewEIDFilter(fpdConfig),
	}

	if config.IDREnabled && config.IDRServiceURL != "" {
		ex.idrClient = idr.NewClient(config.IDRServiceURL, 50*time.Millisecond, config.IDRAPIKey)
	}

	if config.EventRecordEnabled && config.IDRServiceURL != "" {
		ex.eventRecorder = idr.NewEventRecorder(config.IDRServiceURL, config.EventBufferSize)
	}

	return ex
}

// SetDynamicRegistry sets the dynamic bidder registry
func (e *Exchange) SetDynamicRegistry(dr *ortb.DynamicRegistry) {
	e.configMu.Lock()
	defer e.configMu.Unlock()
	e.dynamicRegistry = dr
}

// SetMetrics sets the metrics recorder for tracking revenue/margins
func (e *Exchange) SetMetrics(m MetricsRecorder) {
	e.configMu.Lock()
	defer e.configMu.Unlock()
	e.metrics = m
}

// GetDynamicRegistry returns the dynamic registry
func (e *Exchange) GetDynamicRegistry() *ortb.DynamicRegistry {
	e.configMu.RLock()
	defer e.configMu.RUnlock()
	return e.dynamicRegistry
}

// Close shuts down the exchange and flushes pending events
func (e *Exchange) Close() error {
	if e.eventRecorder != nil {
		return e.eventRecorder.Close()
	}
	return nil
}

// AuctionRequest contains auction parameters
type AuctionRequest struct {
	BidRequest *openrtb.BidRequest
	Timeout    time.Duration
	Account    string
	Debug      bool
}

// AuctionResponse contains auction results
type AuctionResponse struct {
	BidResponse   *openrtb.BidResponse
	BidderResults map[string]*BidderResult
	IDRResult     *idr.SelectPartnersResponse
	DebugInfo     *DebugInfo
}

// BidderResult contains results from a single bidder
type BidderResult struct {
	BidderCode string
	Bids       []*adapters.TypedBid
	Errors     []error
	Latency    time.Duration
	Selected   bool
	Score      float64
	TimedOut   bool // P2-2: indicates if the bidder request timed out
}

// DebugInfo contains debug information
type DebugInfo struct {
	RequestTime       time.Time
	TotalLatency      time.Duration
	IDRLatency        time.Duration
	BidderLatencies   map[string]time.Duration
	SelectedBidders   []string
	ExcludedBidders   []string
	Errors            map[string][]string
	errorsMu          sync.Mutex // Protects concurrent access to Errors map
}

// AddError safely adds errors to the Errors map with mutex protection
func (d *DebugInfo) AddError(key string, errors []string) {
	d.errorsMu.Lock()
	defer d.errorsMu.Unlock()
	d.Errors[key] = errors
}

// AppendError safely appends an error to the Errors map with mutex protection
func (d *DebugInfo) AppendError(key string, errMsg string) {
	d.errorsMu.Lock()
	defer d.errorsMu.Unlock()
	d.Errors[key] = append(d.Errors[key], errMsg)
}

// RequestValidationError represents a bid request validation failure
type RequestValidationError struct {
	Field  string
	Reason string
}

func (e *RequestValidationError) Error() string {
	return fmt.Sprintf("invalid request: %s - %s", e.Field, e.Reason)
}

// ValidateRequest performs OpenRTB 2.x request validation
// Returns nil if valid, or a RequestValidationError describing the issue
func ValidateRequest(req *openrtb.BidRequest) *RequestValidationError {
	if req == nil {
		return &RequestValidationError{Field: "request", Reason: "nil request"}
	}

	// Validate required field: ID
	if req.ID == "" {
		return &RequestValidationError{Field: "id", Reason: "missing required field"}
	}

	// Validate required field: at least one impression
	if len(req.Imp) == 0 {
		return &RequestValidationError{Field: "imp", Reason: "at least one impression is required"}
	}

	// Validate impression IDs are unique and non-empty
	impIDs := make(map[string]struct{}, len(req.Imp))
	for i, imp := range req.Imp {
		if imp.ID == "" {
			return &RequestValidationError{
				Field:  fmt.Sprintf("imp[%d].id", i),
				Reason: "impression ID is required",
			}
		}
		if _, exists := impIDs[imp.ID]; exists {
			return &RequestValidationError{
				Field:  fmt.Sprintf("imp[%d].id", i),
				Reason: fmt.Sprintf("duplicate impression ID: %s", imp.ID),
			}
		}
		impIDs[imp.ID] = struct{}{}
	}

	// Validate Site XOR App (exactly one must be present, not both, not neither)
	hasSite := req.Site != nil
	hasApp := req.App != nil
	if hasSite && hasApp {
		return &RequestValidationError{
			Field:  "site/app",
			Reason: "request cannot contain both site and app objects",
		}
	}
	if !hasSite && !hasApp {
		return &RequestValidationError{
			Field:  "site/app",
			Reason: "request must contain either site or app object",
		}
	}

	// Validate TMax if present (reasonable bounds: 0 means no limit, otherwise 10ms-30000ms)
	if req.TMax < 0 {
		return &RequestValidationError{
			Field:  "tmax",
			Reason: fmt.Sprintf("tmax cannot be negative: %d", req.TMax),
		}
	}
	if req.TMax > 0 && req.TMax < 10 {
		return &RequestValidationError{
			Field:  "tmax",
			Reason: fmt.Sprintf("tmax too small (minimum 10ms): %d", req.TMax),
		}
	}
	if req.TMax > 30000 {
		return &RequestValidationError{
			Field:  "tmax",
			Reason: fmt.Sprintf("tmax too large (maximum 30000ms): %d", req.TMax),
		}
	}

	return nil
}

// BidValidationError represents a bid validation failure
type BidValidationError struct {
	BidID   string
	ImpID   string
	Reason  string
	BidderCode string
}

func (e *BidValidationError) Error() string {
	return fmt.Sprintf("invalid bid from %s (bid=%s, imp=%s): %s", e.BidderCode, e.BidID, e.ImpID, e.Reason)
}

// validateBid checks if a bid meets OpenRTB requirements and exchange rules
func (e *Exchange) validateBid(bid *openrtb.Bid, bidderCode string, impIDs map[string]float64) *BidValidationError {
	if bid == nil {
		return &BidValidationError{BidderCode: bidderCode, Reason: "nil bid"}
	}

	// Check required field: Bid.ID
	if bid.ID == "" {
		return &BidValidationError{
			ImpID:      bid.ImpID,
			BidderCode: bidderCode,
			Reason:     "missing required field: id",
		}
	}

	// Check required field: Bid.ImpID
	if bid.ImpID == "" {
		return &BidValidationError{
			BidID:      bid.ID,
			BidderCode: bidderCode,
			Reason:     "missing required field: impid",
		}
	}

	// Validate ImpID exists in request
	floor, validImp := impIDs[bid.ImpID]
	if !validImp {
		return &BidValidationError{
			BidID:      bid.ID,
			ImpID:      bid.ImpID,
			BidderCode: bidderCode,
			Reason:     fmt.Sprintf("impid %q not found in request", bid.ImpID),
		}
	}

	// Check price is non-negative
	if bid.Price < 0 {
		return &BidValidationError{
			BidID:      bid.ID,
			ImpID:      bid.ImpID,
			BidderCode: bidderCode,
			Reason:     fmt.Sprintf("negative price: %.4f", bid.Price),
		}
	}

	// Check price meets minimum
	if bid.Price < e.config.MinBidPrice {
		return &BidValidationError{
			BidID:      bid.ID,
			ImpID:      bid.ImpID,
			BidderCode: bidderCode,
			Reason:     fmt.Sprintf("price %.4f below minimum %.4f", bid.Price, e.config.MinBidPrice),
		}
	}

	// Check price meets floor
	if floor > 0 && bid.Price < floor {
		return &BidValidationError{
			BidID:      bid.ID,
			ImpID:      bid.ImpID,
			BidderCode: bidderCode,
			Reason:     fmt.Sprintf("price %.4f below floor %.4f", bid.Price, floor),
		}
	}

	// P2-1: Validate that bid has creative content (AdM or NURL required)
	// OpenRTB 2.x requires either inline markup (adm) or a URL to fetch it (nurl)
	if bid.AdM == "" && bid.NURL == "" {
		return &BidValidationError{
			BidID:      bid.ID,
			ImpID:      bid.ImpID,
			BidderCode: bidderCode,
			Reason:     "bid must have either adm or nurl",
		}
	}

	return nil
}

// buildImpFloorMap creates a map of impression IDs to their floor prices
// If publisher has a bid_multiplier, floors are MULTIPLIED to ensure platform gets its cut
// Example: floor=$1, multiplier=1.05 → adjusted_floor=$1.05 (DSPs must bid at least $1.05)
func (e *Exchange) buildImpFloorMap(ctx context.Context, req *openrtb.BidRequest) map[string]float64 {
	impFloors := make(map[string]float64, len(req.Imp))

	// Get publisher's bid multiplier
	var multiplier float64 = 1.0
	var publisherID string
	if pub := middleware.PublisherFromContext(ctx); pub != nil {
		if v, ok := extractBidMultiplier(pub); ok && v >= 1.0 && v <= 10.0 {
			multiplier = v
		}
		// Extract publisher ID for metrics
		if pid, ok := extractPublisherID(pub); ok {
			publisherID = pid
		}
	}

	// Build floor map with multiplier applied
	floorsAdjusted := 0
	for _, imp := range req.Imp {
		baseFloor := imp.BidFloor
		if multiplier != 1.0 && baseFloor > 0 {
			// Multiply floor so DSPs must bid higher to cover platform's cut
			impFloors[imp.ID] = roundToCents(baseFloor * multiplier)
			floorsAdjusted++

			logger.Log.Debug().
				Str("impID", imp.ID).
				Float64("base_floor", baseFloor).
				Float64("multiplier", multiplier).
				Float64("adjusted_floor", impFloors[imp.ID]).
				Msg("Applied multiplier to floor price")
		} else {
			impFloors[imp.ID] = baseFloor
		}
	}

	// Record floor adjustments metric
	if floorsAdjusted > 0 && publisherID != "" {
		e.configMu.RLock()
		if e.metrics != nil {
			for i := 0; i < floorsAdjusted; i++ {
				e.metrics.RecordFloorAdjustment(publisherID)
			}
		}
		e.configMu.RUnlock()
	}

	return impFloors
}

// ValidatedBid wraps a bid with validation status
type ValidatedBid struct {
	Bid        *adapters.TypedBid
	BidderCode string
	DemandType adapters.DemandType // platform (obfuscated) or publisher (transparent)
}

// runAuctionLogic applies auction rules (first-price or second-price) to validated bids
// Returns bids grouped by impression with prices adjusted according to auction type
func (e *Exchange) runAuctionLogic(validBids []ValidatedBid, impFloors map[string]float64) map[string][]ValidatedBid {
	// Group bids by impression
	bidsByImp := make(map[string][]ValidatedBid)
	for _, vb := range validBids {
		impID := vb.Bid.Bid.ImpID
		bidsByImp[impID] = append(bidsByImp[impID], vb)
	}

	// Apply auction logic per impression
	for impID, bids := range bidsByImp {
		if len(bids) == 0 {
			continue
		}

		// Sort by price descending
		sortBidsByPrice(bids)

		if e.config.AuctionType == SecondPriceAuction {
			var winningPrice float64
			originalBidPrice := bids[0].Bid.Bid.Price

			if len(bids) > 1 {
				// Multiple bids: winner pays second highest + increment
				// Use integer arithmetic to avoid floating-point precision errors (P0-2)
				secondPrice := bids[1].Bid.Bid.Price
				winningPrice = roundToCents(secondPrice + e.config.PriceIncrement)
			} else {
				// P0-6: Single bid - use floor as "second price" for consistent auction semantics
				floor := impFloors[impID]
				if floor > 0 {
					winningPrice = roundToCents(floor + e.config.PriceIncrement)
				} else {
					// No floor - winner pays minimum bid price + increment
					winningPrice = roundToCents(e.config.MinBidPrice + e.config.PriceIncrement)
				}
			}

			// P2-2: If winning price exceeds bid, reject the bid entirely
			// A bid that can't meet the second-price threshold shouldn't win
			if winningPrice > originalBidPrice {
				// P2-3: Log bid rejection for debugging auction behavior
				logger.Log.Debug().
					Str("impID", impID).
					Str("bidder", bids[0].BidderCode).
					Float64("bidPrice", originalBidPrice).
					Float64("clearingPrice", winningPrice).
					Float64("floor", impFloors[impID]).
					Float64("increment", e.config.PriceIncrement).
					Msg("bid rejected: clearing price exceeds bid in second-price auction")
				bidsByImp[impID] = nil
				continue
			}
			bids[0].Bid.Bid.Price = winningPrice
		}
		// First-price: winner pays their bid (no adjustment needed)

		bidsByImp[impID] = bids
	}

	return bidsByImp
}

// sortBidsByPrice sorts bids in descending order by price (highest first)
// Includes defensive nil checks to prevent panics
func sortBidsByPrice(bids []ValidatedBid) {
	// Simple insertion sort - typically small number of bids per impression
	for i := 1; i < len(bids); i++ {
		j := i
		for j > 0 {
			// Defensive nil checks (P1-5)
			if bids[j].Bid == nil || bids[j].Bid.Bid == nil ||
				bids[j-1].Bid == nil || bids[j-1].Bid.Bid == nil {
				break
			}
			if bids[j].Bid.Bid.Price > bids[j-1].Bid.Bid.Price {
				bids[j], bids[j-1] = bids[j-1], bids[j]
				j--
			} else {
				break
			}
		}
	}
}

// roundToCents rounds a price to 2 decimal places
// P2-NEW-3: Use math.Round for correct rounding of all values including edge cases
func roundToCents(price float64) float64 {
	// math.Round correctly handles all cases including negative numbers and .5 values
	return math.Round(price*100) / 100.0
}

// applyBidMultiplier applies the publisher's bid multiplier to all bids
// This allows the platform to take a revenue share before returning bids to the publisher
// Bid prices are DIVIDED by the multiplier
// For example: multiplier = 1.05 means publisher gets ~95%, platform keeps ~5% of bid price
func (e *Exchange) applyBidMultiplier(ctx context.Context, bidsByImp map[string][]ValidatedBid) map[string][]ValidatedBid {
	// Get publisher from context (set by publisher_auth middleware)
	pub := middleware.PublisherFromContext(ctx)
	if pub == nil {
		return bidsByImp // No publisher configured, no multiplier to apply
	}

	// Extract bid multiplier and publisher ID from publisher
	var multiplier float64 = 1.0 // Default: no adjustment
	var publisherID string

	// Try to extract via struct field access
	type publisherWithMultiplier struct {
		BidMultiplier float64
	}

	// Use type switch to handle different publisher types
	switch p := pub.(type) {
	case *publisherWithMultiplier:
		multiplier = p.BidMultiplier
	default:
		// Try to extract via reflection for any struct with BidMultiplier field
		// This handles the actual storage.Publisher type
		if v, ok := extractBidMultiplier(pub); ok {
			multiplier = v
		}
	}

	// Extract publisher ID for metrics
	if pid, ok := extractPublisherID(pub); ok {
		publisherID = pid
	}

	// If multiplier is 1.0 (or 0, meaning default), no adjustment needed
	if multiplier == 0 || multiplier == 1.0 {
		return bidsByImp
	}

	// Validate multiplier is in reasonable range (1.0 to 10.0)
	if multiplier < 1.0 || multiplier > 10.0 {
		logger.Log.Warn().
			Float64("multiplier", multiplier).
			Msg("Invalid bid multiplier, ignoring")
		return bidsByImp
	}

	// Apply multiplier to all bid prices (DIVIDE to reduce what publisher sees)
	for impID, bids := range bidsByImp {
		for i := range bids {
			if bids[i].Bid != nil && bids[i].Bid.Bid != nil {
				originalPrice := bids[i].Bid.Bid.Price
				adjustedPrice := roundToCents(originalPrice / multiplier)
				platformCut := originalPrice - adjustedPrice

				// Determine media type from bid
				mediaType := "banner" // default
				if bids[i].Bid.BidType == adapters.BidTypeVideo {
					mediaType = "video"
				} else if bids[i].Bid.BidType == adapters.BidTypeNative {
					mediaType = "native"
				} else if bids[i].Bid.BidType == adapters.BidTypeAudio {
					mediaType = "audio"
				}

				// Log the adjustment for transparency (debug level)
				logger.Log.Debug().
					Str("impID", impID).
					Str("bidder", bids[i].BidderCode).
					Float64("original_price", originalPrice).
					Float64("multiplier", multiplier).
					Float64("adjusted_price", adjustedPrice).
					Float64("platform_cut", platformCut).
					Msg("Applied bid multiplier")

				// Record margin metrics
				if publisherID != "" {
					e.configMu.RLock()
					if e.metrics != nil {
						e.metrics.RecordMargin(publisherID, bids[i].BidderCode, mediaType, originalPrice, adjustedPrice, platformCut)
					}
					e.configMu.RUnlock()
				}

				bids[i].Bid.Bid.Price = adjustedPrice
			}
		}
	}

	return bidsByImp
}

// extractBidMultiplier safely extracts BidMultiplier field from any struct
func extractBidMultiplier(v interface{}) (float64, bool) {
	// Type assert to common publisher interface patterns
	type bidMultiplierGetter interface {
		GetBidMultiplier() float64
	}

	if getter, ok := v.(bidMultiplierGetter); ok {
		return getter.GetBidMultiplier(), true
	}

	// Direct type assertion for storage.Publisher (avoids expensive JSON round-trip)
	type publisherWithMultiplier interface {
		GetBidMultiplier() float64
	}
	if p, ok := v.(publisherWithMultiplier); ok {
		return p.GetBidMultiplier(), true
	}

	// Try concrete struct with BidMultiplier field via reflection-free approach
	type hasBidMultiplier struct {
		BidMultiplier float64
	}
	if p, ok := v.(*hasBidMultiplier); ok {
		return p.BidMultiplier, true
	}

	return 0, false
}

// extractPublisherID safely extracts PublisherID field from publisher struct
func extractPublisherID(v interface{}) (string, bool) {
	// Type assert to interface with GetPublisherID method (avoids expensive JSON round-trip)
	type publisherIDGetter interface {
		GetPublisherID() string
	}
	if getter, ok := v.(publisherIDGetter); ok {
		id := getter.GetPublisherID()
		return id, id != ""
	}

	// Try concrete struct with PublisherID field
	type hasPublisherID struct {
		PublisherID string
	}
	if p, ok := v.(*hasPublisherID); ok {
		return p.PublisherID, p.PublisherID != ""
	}

	return "", false
}

// RunAuction executes the auction
func (e *Exchange) RunAuction(ctx context.Context, req *AuctionRequest) (*AuctionResponse, error) {
	startTime := time.Now()

	// P0-7: Validate required BidRequest fields per OpenRTB 2.x spec
	if req.BidRequest == nil {
		return nil, fmt.Errorf("invalid auction request: missing bid request")
	}
	if req.BidRequest.ID == "" {
		return nil, fmt.Errorf("invalid bid request: missing required field 'id'")
	}
	if len(req.BidRequest.Imp) == 0 {
		return nil, fmt.Errorf("invalid bid request: must have at least one impression")
	}

	// P1-2: Validate impression count early to prevent OOM from malicious requests
	// This check must happen BEFORE allocating maps based on impression count
	if len(req.BidRequest.Imp) > defaultMaxImpressionsPerRequest {
		return nil, fmt.Errorf("invalid bid request: too many impressions (max %d, got %d)",
			defaultMaxImpressionsPerRequest, len(req.BidRequest.Imp))
	}

	// P2-3: Validate Site/App mutual exclusivity per OpenRTB 2.5 section 3.2.1
	hasSite := req.BidRequest.Site != nil
	hasApp := req.BidRequest.App != nil
	if !hasSite && !hasApp {
		return nil, fmt.Errorf("invalid bid request: must have either 'site' or 'app' object (OpenRTB 2.5)")
	}
	if hasSite && hasApp {
		return nil, fmt.Errorf("invalid bid request: cannot have both 'site' and 'app' objects (OpenRTB 2.5)")
	}

	// P1-NEW-2: Validate impression IDs are unique and non-empty per OpenRTB 2.5 section 3.2.4
	seenImpIDs := make(map[string]bool, len(req.BidRequest.Imp))
	for i, imp := range req.BidRequest.Imp {
		if imp.ID == "" {
			return nil, fmt.Errorf("invalid bid request: impression[%d] has empty id (required by OpenRTB 2.5)", i)
		}
		if seenImpIDs[imp.ID] {
			return nil, fmt.Errorf("invalid bid request: duplicate impression id %q (must be unique per OpenRTB 2.5)", imp.ID)
		}
		seenImpIDs[imp.ID] = true

		// P2-1: Validate impression has at least one media type per OpenRTB 2.5 section 3.2.4
		if imp.Banner == nil && imp.Video == nil && imp.Audio == nil && imp.Native == nil {
			return nil, fmt.Errorf("invalid bid request: impression[%d] has no media type (banner/video/audio/native required)", i)
		}

		// P1-NEW-4: Validate banner dimensions per OpenRTB 2.5 section 3.2.6
		if imp.Banner != nil {
			hasExplicitSize := imp.Banner.W > 0 && imp.Banner.H > 0
			hasFormat := len(imp.Banner.Format) > 0
			if !hasExplicitSize && !hasFormat {
				return nil, fmt.Errorf("invalid bid request: impression[%d] banner must have either w/h or format array (OpenRTB 2.5)", i)
			}
		}
	}

	response := &AuctionResponse{
		BidderResults: make(map[string]*BidderResult),
		DebugInfo: &DebugInfo{
			RequestTime:     startTime,
			BidderLatencies: make(map[string]time.Duration),
			Errors:          make(map[string][]string),
		},
	}

	// Validate the bid request per OpenRTB 2.x specification
	if validationErr := ValidateRequest(req.BidRequest); validationErr != nil {
		response.DebugInfo.TotalLatency = time.Since(startTime)
		return response, validationErr
	}

	// Get timeout from request or config
	// P1-NEW-1: Validate TMax bounds to prevent abuse
	timeout := req.Timeout
	if timeout == 0 && req.BidRequest.TMax > 0 {
		tmax := req.BidRequest.TMax
		// Cap TMax at reasonable maximum to prevent resource exhaustion
		if tmax > maxAllowedTMax {
			tmax = maxAllowedTMax
		}
		timeout = time.Duration(tmax) * time.Millisecond
	}
	if timeout == 0 {
		timeout = e.config.DefaultTimeout
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get available bidders from static registry
	availableBidders := e.registry.ListEnabledBidders()

	// Snapshot config-protected fields under lock for consistent view during auction
	e.configMu.RLock()
	dynamicRegistry := e.dynamicRegistry
	fpdProcessor := e.fpdProcessor
	eidFilter := e.eidFilter
	e.configMu.RUnlock()

	// Add dynamic bidders if enabled
	if e.config.DynamicBiddersEnabled && dynamicRegistry != nil {
		dynamicCodes := dynamicRegistry.ListEnabledBidderCodes()
		availableBidders = append(availableBidders, dynamicCodes...)
	}

	if len(availableBidders) == 0 {
		response.BidResponse = e.buildEmptyResponse(req.BidRequest, openrtb.NoBidNoBiddersAvailable)
		return response, nil
	}

	// Run IDR selection if enabled
	selectedBidders := availableBidders
	if e.idrClient != nil && e.config.IDREnabled {
		idrStart := time.Now()

		// P1-15: Build minimal request to reduce payload size
		minReq := e.buildMinimalIDRRequest(req.BidRequest)
		idrResult, err := e.idrClient.SelectPartnersMinimal(ctx, minReq, availableBidders)

		response.DebugInfo.IDRLatency = time.Since(idrStart)

		if err == nil && idrResult != nil {
			response.IDRResult = idrResult
			selectedBidders = make([]string, 0, len(idrResult.SelectedBidders))
			for _, sb := range idrResult.SelectedBidders {
				selectedBidders = append(selectedBidders, sb.BidderCode)
			}

			for _, eb := range idrResult.ExcludedBidders {
				response.DebugInfo.ExcludedBidders = append(response.DebugInfo.ExcludedBidders, eb.BidderCode)
			}
		}
		// If IDR fails, fall back to all bidders
	}

	response.DebugInfo.SelectedBidders = selectedBidders

	// Process FPD and filter EIDs (using snapshotted processor/filter for consistency)
	var bidderFPD fpd.BidderFPD
	if fpdProcessor != nil {
		// Filter EIDs first
		if eidFilter != nil {
			eidFilter.ProcessRequestEIDs(req.BidRequest)
		}

		// Process FPD for each bidder
		var err error
		bidderFPD, err = fpdProcessor.ProcessRequest(req.BidRequest, selectedBidders)
		if err != nil {
			// Log error but continue - FPD is not critical
			response.DebugInfo.AddError("fpd", []string{err.Error()})
		}
	}

	// Call bidders in parallel
	results := e.callBiddersWithFPD(ctx, req.BidRequest, selectedBidders, timeout, bidderFPD)

	// Extract request context for event recording
	var country, deviceType, mediaType, adSize, publisherID string
	if req.BidRequest.Device != nil && req.BidRequest.Device.Geo != nil {
		country = req.BidRequest.Device.Geo.Country
	}
	if req.BidRequest.Device != nil {
		switch req.BidRequest.Device.DeviceType {
		case 1:
			deviceType = "mobile"
		case 2:
			deviceType = "desktop"
		case 3:
			deviceType = "ctv"
		default:
			deviceType = "unknown"
		}
	}
	if len(req.BidRequest.Imp) > 0 {
		imp := req.BidRequest.Imp[0]
		if imp.Banner != nil {
			mediaType = "banner"
			if imp.Banner.W > 0 && imp.Banner.H > 0 {
				adSize = fmt.Sprintf("%dx%d", imp.Banner.W, imp.Banner.H)
			}
		} else if imp.Video != nil {
			mediaType = "video"
		} else if imp.Native != nil {
			mediaType = "native"
		}
	}
	if req.BidRequest.Site != nil && req.BidRequest.Site.Publisher != nil {
		publisherID = req.BidRequest.Site.Publisher.ID
	}

	// P1-2: Check context deadline before expensive validation work
	// If we've already timed out, return early with whatever we have
	select {
	case <-ctx.Done():
		response.DebugInfo.TotalLatency = time.Since(startTime)
		response.BidResponse = e.buildEmptyResponse(req.BidRequest, openrtb.NoBidTimeout)
		return response, nil // Return empty response rather than error on timeout
	default:
		// Context still valid, proceed with validation
	}

	// Build impression floor map for bid validation (with multiplier applied to floors)
	impFloors := e.buildImpFloorMap(ctx, req.BidRequest)

	// Track seen bid IDs for deduplication
	seenBidIDs := make(map[string]struct{})

	// Collect and validate all bids
	var validBids []ValidatedBid
	var validationErrors []error

	// Collect results
	for bidderCode, result := range results {
		response.BidderResults[bidderCode] = result
		response.DebugInfo.BidderLatencies[bidderCode] = result.Latency

		if len(result.Errors) > 0 {
			errStrs := make([]string, len(result.Errors))
			for i, err := range result.Errors {
				errStrs[i] = err.Error()
			}
			response.DebugInfo.AddError(bidderCode, errStrs)
		}

		// Record event to IDR
		if e.eventRecorder != nil {
			hadBid := len(result.Bids) > 0
			var bidCPM *float64
			if hadBid && len(result.Bids) > 0 {
				cpm := result.Bids[0].Bid.Price
				bidCPM = &cpm
			}
			hadError := len(result.Errors) > 0
			var errorMsg string
			if hadError {
				// P2-7: Aggregate all errors instead of just the first
				if len(result.Errors) == 1 {
					errorMsg = result.Errors[0].Error()
				} else {
					errMsgs := make([]string, len(result.Errors))
					for i, err := range result.Errors {
						errMsgs[i] = err.Error()
					}
					errorMsg = fmt.Sprintf("%d errors: %s", len(result.Errors), strings.Join(errMsgs, "; "))
				}
			}

			e.eventRecorder.RecordBidResponse(
				req.BidRequest.ID,
				bidderCode,
				float64(result.Latency.Milliseconds()),
				hadBid,
				bidCPM,
				nil, // floor price
				country,
				deviceType,
				mediaType,
				adSize,
				publisherID,
				result.TimedOut, // P2-2: use actual timeout status
				hadError,
				errorMsg,
			)
		}

		// Validate and deduplicate bids
		for _, tb := range result.Bids {
			// Skip nil bids
			if tb == nil || tb.Bid == nil {
				continue
			}

			// Validate bid
			if validErr := e.validateBid(tb.Bid, bidderCode, impFloors); validErr != nil {
				// P3-1: Log bid validation failures for debugging
				logger.Log.Debug().
					Str("bidder", bidderCode).
					Str("bidID", tb.Bid.ID).
					Str("impID", tb.Bid.ImpID).
					Float64("price", tb.Bid.Price).
					Err(validErr).
					Msg("bid validation failed")
				validationErrors = append(validationErrors, validErr)
				response.DebugInfo.AppendError(bidderCode, validErr.Error())
				continue
			}

			// Check for duplicate bid IDs
			if _, seen := seenBidIDs[tb.Bid.ID]; seen {
				dupErr := &BidValidationError{
					BidID:      tb.Bid.ID,
					ImpID:      tb.Bid.ImpID,
					BidderCode: bidderCode,
					Reason:     "duplicate bid ID",
				}
				validationErrors = append(validationErrors, dupErr)
				response.DebugInfo.AppendError(bidderCode, dupErr.Error())
				continue
			}
			seenBidIDs[tb.Bid.ID] = struct{}{}

			// Add to valid bids with demand type
			validBids = append(validBids, ValidatedBid{
				Bid:        tb,
				BidderCode: bidderCode,
				DemandType: e.getDemandType(bidderCode, dynamicRegistry),
			})
		}
	}

	// Apply auction logic (first-price or second-price)
	auctionedBids := e.runAuctionLogic(validBids, impFloors)

	// Apply bid multiplier if publisher is configured with one
	auctionedBids = e.applyBidMultiplier(ctx, auctionedBids)

	// Build seat bids with demand type obfuscation:
	// - Platform demand: aggregated into single "thenexusengine" seat (highest bid per impression)
	// - Publisher demand: shown transparently with original bidder codes
	seatBidMap := make(map[string]*openrtb.SeatBid)

	for _, impBids := range auctionedBids {
		// Separate platform and publisher bids for this impression
		var platformBids []ValidatedBid
		var publisherBids []ValidatedBid

		for _, vb := range impBids {
			if vb.DemandType == adapters.DemandTypePublisher {
				publisherBids = append(publisherBids, vb)
			} else {
				// Default to platform for obfuscation
				platformBids = append(platformBids, vb)
			}
		}

		// Add highest platform bid to "thenexusengine" seat (obfuscated)
		if len(platformBids) > 0 {
			// Find highest CPM platform bid for this impression
			highestPlatformBid := platformBids[0]
			for _, vb := range platformBids[1:] {
				if vb.Bid.Bid.Price > highestPlatformBid.Bid.Bid.Price {
					highestPlatformBid = vb
				}
			}

			// Get or create the thenexusengine seat
			nexusSeat, ok := seatBidMap[adapters.PlatformSeatName]
			if !ok {
				nexusSeat = &openrtb.SeatBid{
					Seat: adapters.PlatformSeatName,
					Bid:  []openrtb.Bid{},
				}
				seatBidMap[adapters.PlatformSeatName] = nexusSeat
			}

			// Create obfuscated bid with "thenexusengine" branding in targeting
			bid := *highestPlatformBid.Bid.Bid
			bidExt := e.buildBidExtension(highestPlatformBid)
			if extBytes, err := json.Marshal(bidExt); err == nil {
				bid.Ext = extBytes
			}
			nexusSeat.Bid = append(nexusSeat.Bid, bid)
		}

		// Add all publisher bids transparently
		for _, vb := range publisherBids {
			sb, ok := seatBidMap[vb.BidderCode]
			if !ok {
				sb = &openrtb.SeatBid{
					Seat: vb.BidderCode,
					Bid:  []openrtb.Bid{},
				}
				seatBidMap[vb.BidderCode] = sb
			}

			// Create bid copy with Prebid extension for targeting
			bid := *vb.Bid.Bid
			bidExt := e.buildBidExtension(vb)
			if extBytes, err := json.Marshal(bidExt); err == nil {
				bid.Ext = extBytes
			}
			sb.Bid = append(sb.Bid, bid)
		}
	}

	// Convert seat bid map to slice
	allBids := make([]openrtb.SeatBid, 0, len(seatBidMap))
	for _, sb := range seatBidMap {
		allBids = append(allBids, *sb)
	}

	// Build response
	response.BidResponse = &openrtb.BidResponse{
		ID:      req.BidRequest.ID,
		SeatBid: allBids,
		Cur:     e.config.DefaultCurrency,
	}

	response.DebugInfo.TotalLatency = time.Since(startTime)

	// P3-1: Log auction completion with summary stats
	totalBids := 0
	for _, sb := range allBids {
		totalBids += len(sb.Bid)
	}
	logger.Log.Debug().
		Str("requestID", req.BidRequest.ID).
		Int("bidders", len(selectedBidders)).
		Int("impressions", len(req.BidRequest.Imp)).
		Int("bids", totalBids).
		Dur("latency", response.DebugInfo.TotalLatency).
		Msg("auction completed")

	return response, nil
}

// callBidders calls all selected bidders in parallel (legacy, without FPD)
func (e *Exchange) callBidders(ctx context.Context, req *openrtb.BidRequest, bidders []string, timeout time.Duration) map[string]*BidderResult {
	return e.callBiddersWithFPD(ctx, req, bidders, timeout, nil)
}

// callBiddersWithFPD calls all selected bidders in parallel with FPD support
// P0-1: Uses sync.Map for thread-safe result collection
// P0-4: Uses semaphore to limit concurrent bidder goroutines
func (e *Exchange) callBiddersWithFPD(ctx context.Context, req *openrtb.BidRequest, bidders []string, timeout time.Duration, bidderFPD fpd.BidderFPD) map[string]*BidderResult {
	var results sync.Map // P0-1: Thread-safe map for concurrent writes
	var wg sync.WaitGroup

	// Snapshot dynamicRegistry for consistent access during bidder calls
	e.configMu.RLock()
	dynamicRegistry := e.dynamicRegistry
	e.configMu.RUnlock()

	// P0-4: Create semaphore to limit concurrent bidder calls
	maxConcurrent := e.config.MaxConcurrentBidders
	if maxConcurrent <= 0 {
		maxConcurrent = 10 // Default limit
	}
	sem := make(chan struct{}, maxConcurrent)

	for _, bidderCode := range bidders {
		// Try static registry first
		adapterWithInfo, ok := e.registry.Get(bidderCode)
		if ok {
			wg.Add(1)
			go func(code string, awi adapters.AdapterWithInfo) {
				defer wg.Done()

				// P0-4: Acquire semaphore (blocks if at capacity)
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }() // Release on completion
				case <-ctx.Done():
					// Context cancelled while waiting for semaphore
					results.Store(code, &BidderResult{
						BidderCode: code,
						Errors:     []error{ctx.Err()},
						TimedOut:   true,
					})
					return
				}

				// Check geo-aware consent filtering (GDPR, CCPA, etc.)
				gvlID := awi.Info.GVLVendorID
				if middleware.ShouldFilterBidderByGeo(req, gvlID) {
					// Detect which regulation applies
					regulation := middleware.RegulationNone
					if req.Device != nil && req.Device.Geo != nil {
						regulation = middleware.DetectRegulationFromGeo(req.Device.Geo)
					}

					logger.Log.Info().
						Str("bidder", code).
						Int("gvl_id", gvlID).
						Str("request_id", req.ID).
						Str("regulation", string(regulation)).
						Str("country", func() string {
							if req.Device != nil && req.Device.Geo != nil {
								return req.Device.Geo.Country
							}
							return ""
						}()).
						Str("region", func() string {
							if req.Device != nil && req.Device.Geo != nil {
								return req.Device.Geo.Region
							}
							return ""
						}()).
						Msg("Skipping bidder - no consent for user's geographic location")

					results.Store(code, &BidderResult{
						BidderCode: code,
						Errors:     []error{fmt.Errorf("no %s consent for vendor %d", regulation, gvlID)},
					})
					return
				}

				// Clone request and apply bidder-specific FPD
				bidderReq := e.cloneRequestWithFPD(req, code, bidderFPD)

				result := e.callBidder(ctx, bidderReq, code, awi.Adapter, timeout)

				results.Store(code, result) // P0-1: Thread-safe store
			}(bidderCode, adapterWithInfo)
			continue
		}

		// Try dynamic registry (using snapshotted reference)
		if dynamicRegistry != nil {
			dynamicAdapter, found := dynamicRegistry.Get(bidderCode)
			if found {
				wg.Add(1)
				go func(code string, da *ortb.GenericAdapter) {
					defer wg.Done()

					// P0-4: Acquire semaphore (blocks if at capacity)
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }() // Release on completion
					case <-ctx.Done():
						// Context cancelled while waiting for semaphore
						results.Store(code, &BidderResult{
							BidderCode: code,
							Errors:     []error{ctx.Err()},
							TimedOut:   true,
						})
						return
					}

					// Check geo-aware consent filtering (GDPR, CCPA, etc.)
					gvlID := da.GetGVLVendorID()
					if middleware.ShouldFilterBidderByGeo(req, gvlID) {
						// Detect which regulation applies
						regulation := middleware.RegulationNone
						if req.Device != nil && req.Device.Geo != nil {
							regulation = middleware.DetectRegulationFromGeo(req.Device.Geo)
						}

						logger.Log.Info().
							Str("bidder", code).
							Int("gvl_id", gvlID).
							Str("request_id", req.ID).
							Str("regulation", string(regulation)).
							Str("country", func() string {
								if req.Device != nil && req.Device.Geo != nil {
									return req.Device.Geo.Country
								}
								return ""
							}()).
							Str("region", func() string {
								if req.Device != nil && req.Device.Geo != nil {
									return req.Device.Geo.Region
								}
								return ""
							}()).
							Msg("Skipping dynamic bidder - no consent for user's geographic location")

						results.Store(code, &BidderResult{
							BidderCode: code,
							Errors:     []error{fmt.Errorf("no %s consent for vendor %d", regulation, gvlID)},
						})
						return
					}

					// Clone request and apply bidder-specific FPD
					bidderReq := e.cloneRequestWithFPD(req, code, bidderFPD)

					// P1-4: Use dynamic adapter's timeout with validation bounds
					// P2-4: Always validate bounds, then use smaller of dynamic or parent timeout
					bidderTimeout := timeout
					if da.GetTimeout() > 0 {
						dynamicTimeout := da.GetTimeout()
						// Enforce minimum timeout to prevent crashes
						if dynamicTimeout < minBidderTimeout {
							dynamicTimeout = minBidderTimeout
						}
						// Enforce maximum timeout to prevent resource exhaustion
						if dynamicTimeout > maxBidderTimeout {
							dynamicTimeout = maxBidderTimeout
						}
						// Use the smaller of validated dynamic timeout or parent timeout
						if dynamicTimeout < bidderTimeout {
							bidderTimeout = dynamicTimeout
						}
					}

					result := e.callBidder(ctx, bidderReq, code, da, bidderTimeout)

					results.Store(code, result) // P0-1: Thread-safe store
				}(bidderCode, dynamicAdapter)
			}
		}
	}

	wg.Wait()

	// P0-1: Convert sync.Map to regular map for return
	finalResults := make(map[string]*BidderResult)
	results.Range(func(key, value interface{}) bool {
		finalResults[key.(string)] = value.(*BidderResult)
		return true
	})
	return finalResults
}

// cloneRequestWithFPD creates a selective copy of the request with bidder-specific FPD applied
// and enforces USD currency for all bid requests.
// PERF: Only clones fields that are modified (Cur, Imp, Site/App/User if FPD applies).
// Shared fields (Device, Regs, Source, etc.) are NOT copied - adapters must not mutate them.
func (e *Exchange) cloneRequestWithFPD(req *openrtb.BidRequest, bidderCode string, bidderFPD fpd.BidderFPD) *openrtb.BidRequest {
	// Shallow copy - shares pointers to Device, Regs, Source, etc.
	clone := *req

	// Clone Cur slice (we overwrite it)
	clone.Cur = []string{e.config.DefaultCurrency}

	// Clone Imp slice - we modify BidFloorCur on each impression
	// Only clone the slice and the structs we modify, share Banner/Video/etc. pointers
	if len(req.Imp) > 0 {
		limits := e.config.CloneLimits
		impCount := len(req.Imp)
		if impCount > limits.MaxImpressionsPerRequest {
			impCount = limits.MaxImpressionsPerRequest
		}
		clone.Imp = make([]openrtb.Imp, impCount)
		for i := 0; i < impCount; i++ {
			clone.Imp[i] = req.Imp[i] // Shallow copy of Imp struct
			clone.Imp[i].BidFloorCur = e.config.DefaultCurrency
		}
	}

	// Check if FPD will be applied (requires cloning Site/App/User)
	var fpdData *fpd.ResolvedFPD
	if bidderFPD != nil {
		fpdData, _ = bidderFPD[bidderCode]
	}
	hasFPD := fpdData != nil && e.fpdProcessor != nil

	// Clone Site only if FPD will modify it
	if req.Site != nil && hasFPD && fpdData.Site != nil {
		siteCopy := *req.Site
		clone.Site = &siteCopy
	}

	// Clone App only if FPD will modify it
	if req.App != nil && hasFPD && fpdData.App != nil {
		appCopy := *req.App
		clone.App = &appCopy
	}

	// Clone User only if FPD will modify it
	if req.User != nil && hasFPD && fpdData.User != nil {
		userCopy := *req.User
		clone.User = &userCopy
	}

	// Apply FPD if available (now safe since we cloned the affected objects)
	if hasFPD {
		e.fpdProcessor.ApplyFPDToRequest(&clone, bidderCode, fpdData)
	}

	return &clone
}

// deepCloneRequest creates a deep copy of the BidRequest to avoid race conditions
// when multiple bidders modify request data concurrently
// P3-1: Uses configurable limits to bound allocations
func deepCloneRequest(req *openrtb.BidRequest, limits *CloneLimits) *openrtb.BidRequest {
	clone := *req

	// P1-NEW-2: Deep copy top-level string slices to prevent shared references
	if len(req.Cur) > 0 {
		clone.Cur = make([]string, len(req.Cur))
		copy(clone.Cur, req.Cur)
	}
	if len(req.WSeat) > 0 {
		clone.WSeat = make([]string, len(req.WSeat))
		copy(clone.WSeat, req.WSeat)
	}
	if len(req.BSeat) > 0 {
		clone.BSeat = make([]string, len(req.BSeat))
		copy(clone.BSeat, req.BSeat)
	}
	if len(req.WLang) > 0 {
		clone.WLang = make([]string, len(req.WLang))
		copy(clone.WLang, req.WLang)
	}
	if len(req.BCat) > 0 {
		clone.BCat = make([]string, len(req.BCat))
		copy(clone.BCat, req.BCat)
	}
	if len(req.BAdv) > 0 {
		clone.BAdv = make([]string, len(req.BAdv))
		copy(clone.BAdv, req.BAdv)
	}
	if len(req.BApp) > 0 {
		clone.BApp = make([]string, len(req.BApp))
		copy(clone.BApp, req.BApp)
	}

	// Deep copy Site
	if req.Site != nil {
		siteCopy := *req.Site
		if req.Site.Publisher != nil {
			pubCopy := *req.Site.Publisher
			siteCopy.Publisher = &pubCopy
		}
		if req.Site.Content != nil {
			contentCopy := *req.Site.Content
			// P2-5: Clone and limit Content.Data segments
			if len(req.Site.Content.Data) > 0 {
				dataCount := len(req.Site.Content.Data)
				if dataCount > limits.MaxDataPerUser {
					dataCount = limits.MaxDataPerUser
				}
				contentCopy.Data = make([]openrtb.Data, dataCount)
				copy(contentCopy.Data, req.Site.Content.Data[:dataCount])
			}
			siteCopy.Content = &contentCopy
		}
		clone.Site = &siteCopy
	}

	// Deep copy App
	if req.App != nil {
		appCopy := *req.App
		if req.App.Publisher != nil {
			pubCopy := *req.App.Publisher
			appCopy.Publisher = &pubCopy
		}
		if req.App.Content != nil {
			contentCopy := *req.App.Content
			// P2-5: Clone and limit Content.Data segments
			if len(req.App.Content.Data) > 0 {
				dataCount := len(req.App.Content.Data)
				if dataCount > limits.MaxDataPerUser {
					dataCount = limits.MaxDataPerUser
				}
				contentCopy.Data = make([]openrtb.Data, dataCount)
				copy(contentCopy.Data, req.App.Content.Data[:dataCount])
			}
			appCopy.Content = &contentCopy
		}
		clone.App = &appCopy
	}

	// Deep copy User
	if req.User != nil {
		userCopy := *req.User
		if req.User.Geo != nil {
			geoCopy := *req.User.Geo
			userCopy.Geo = &geoCopy
		}
		// Deep copy EIDs slice (P1-3: bounded allocation)
		if len(req.User.EIDs) > 0 {
			eidCount := len(req.User.EIDs)
			if eidCount > limits.MaxEIDsPerUser {
				eidCount = limits.MaxEIDsPerUser
			}
			userCopy.EIDs = make([]openrtb.EID, eidCount)
			copy(userCopy.EIDs, req.User.EIDs[:eidCount])
		}
		// Deep copy Data slice (P1-3: bounded allocation)
		if len(req.User.Data) > 0 {
			dataCount := len(req.User.Data)
			if dataCount > limits.MaxDataPerUser {
				dataCount = limits.MaxDataPerUser
			}
			userCopy.Data = make([]openrtb.Data, dataCount)
			copy(userCopy.Data, req.User.Data[:dataCount])
		}
		clone.User = &userCopy
	}

	// Deep copy Device
	if req.Device != nil {
		deviceCopy := *req.Device
		if req.Device.Geo != nil {
			geoCopy := *req.Device.Geo
			deviceCopy.Geo = &geoCopy
		}
		clone.Device = &deviceCopy
	}

	// Deep copy Regs
	if req.Regs != nil {
		regsCopy := *req.Regs
		clone.Regs = &regsCopy
	}

	// Deep copy Source
	if req.Source != nil {
		sourceCopy := *req.Source
		if req.Source.SChain != nil {
			schainCopy := *req.Source.SChain
			// P1-3: bounded allocation for supply chain nodes
			if len(req.Source.SChain.Nodes) > 0 {
				nodeCount := len(req.Source.SChain.Nodes)
				if nodeCount > limits.MaxSChainNodes {
					nodeCount = limits.MaxSChainNodes
				}
				schainCopy.Nodes = make([]openrtb.SupplyChainNode, nodeCount)
				copy(schainCopy.Nodes, req.Source.SChain.Nodes[:nodeCount])
			}
			sourceCopy.SChain = &schainCopy
		}
		clone.Source = &sourceCopy
	}

	// Deep copy Imp slice (P1-3: bounded allocation)
	if len(req.Imp) > 0 {
		impCount := len(req.Imp)
		if impCount > limits.MaxImpressionsPerRequest {
			impCount = limits.MaxImpressionsPerRequest
		}
		clone.Imp = make([]openrtb.Imp, impCount)
		for i := 0; i < impCount; i++ {
			imp := req.Imp[i]
			impCopy := imp
			if imp.Banner != nil {
				bannerCopy := *imp.Banner
				impCopy.Banner = &bannerCopy
			}
			if imp.Video != nil {
				videoCopy := *imp.Video
				impCopy.Video = &videoCopy
			}
			if imp.Audio != nil {
				audioCopy := *imp.Audio
				impCopy.Audio = &audioCopy
			}
			if imp.Native != nil {
				nativeCopy := *imp.Native
				impCopy.Native = &nativeCopy
			}
			if imp.PMP != nil {
				pmpCopy := *imp.PMP
				// P1-3: bounded allocation for deals
				if len(imp.PMP.Deals) > 0 {
					dealCount := len(imp.PMP.Deals)
					if dealCount > limits.MaxDealsPerImp {
						dealCount = limits.MaxDealsPerImp
					}
					pmpCopy.Deals = make([]openrtb.Deal, dealCount)
					copy(pmpCopy.Deals, imp.PMP.Deals[:dealCount])
				}
				impCopy.PMP = &pmpCopy
			}
			clone.Imp[i] = impCopy
		}
	}

	return &clone
}

// callBidder calls a single bidder
func (e *Exchange) callBidder(ctx context.Context, req *openrtb.BidRequest, bidderCode string, adapter adapters.Adapter, timeout time.Duration) *BidderResult {
	start := time.Now()
	result := &BidderResult{
		BidderCode: bidderCode,
		Selected:   true,
	}

	// Build requests
	extraInfo := &adapters.ExtraRequestInfo{
		BidderCoreName: bidderCode,
	}

	requests, errs := adapter.MakeRequests(req, extraInfo)
	if len(errs) > 0 {
		result.Errors = append(result.Errors, errs...)
	}

	// P1-NEW-6: Check context after potentially expensive MakeRequests operation
	select {
	case <-ctx.Done():
		// P3-1: Log bidder timeout after MakeRequests
		logger.Log.Debug().
			Str("bidder", bidderCode).
			Dur("elapsed", time.Since(start)).
			Msg("bidder timed out after MakeRequests")
		result.Errors = append(result.Errors, ctx.Err())
		result.Latency = time.Since(start)
		result.TimedOut = true
		return result
	default:
		// Context still valid, continue
	}

	if len(requests) == 0 {
		result.Latency = time.Since(start)
		return result
	}

	// Execute requests (could parallelize for multi-request adapters)
	allBids := make([]*adapters.TypedBid, 0)
	for _, reqData := range requests {
		// Check if context has expired before each request to avoid wasted work
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.Latency = time.Since(start)
			result.TimedOut = true // P2-2: mark as timed out
			return result
		default:
			// Context still valid, proceed with request
		}

		// Handle mock requests (e.g., demo adapter) - use request body as response
		var resp *adapters.ResponseData
		if reqData.Method == "MOCK" {
			resp = &adapters.ResponseData{
				StatusCode: 200,
				Body:       reqData.Body,
				Headers:    reqData.Headers,
			}
		} else {
			var err error
			resp, err = e.httpClient.Do(ctx, reqData, timeout)
			if err != nil {
				// P3-1: Log HTTP request failures with context
				isTimeout := err == context.DeadlineExceeded || err == context.Canceled
				logger.Log.Debug().
					Str("bidder", bidderCode).
					Str("uri", reqData.URI).
					Dur("elapsed", time.Since(start)).
					Bool("timeout", isTimeout).
					Err(err).
					Msg("bidder HTTP request failed")
				result.Errors = append(result.Errors, err)
				// P2-2: Check if this was a timeout error
				if isTimeout {
					result.TimedOut = true
				}
				continue
			}
		}

		bidderResp, errs := adapter.MakeBids(req, resp)
		if len(errs) > 0 {
			result.Errors = append(result.Errors, errs...)
		}

		if bidderResp != nil {
			// P2-5: Validate BidResponse.ID matches BidRequest.ID (OpenRTB 2.x requirement)
			// Per spec, response ID must echo request ID - reject on mismatch
			if bidderResp.ResponseID != "" && bidderResp.ResponseID != req.ID {
				result.Errors = append(result.Errors, fmt.Errorf(
					"response ID mismatch from %s: expected %q, got %q (bids rejected)",
					bidderCode, req.ID, bidderResp.ResponseID,
				))
				continue // Reject all bids from this response
			}

			// P1-NEW-3: Normalize and validate response currency
			// Per OpenRTB 2.5 spec section 7.2, empty currency means USD
			responseCurrency := bidderResp.Currency
			if responseCurrency == "" {
				responseCurrency = "USD" // OpenRTB 2.5 default
			}

			// P1-NEW-4: Defensive check for exchange currency misconfiguration
			// Normalize exchange currency to USD if empty to prevent silent validation bypass
			exchangeCurrency := e.config.DefaultCurrency
			if exchangeCurrency == "" {
				exchangeCurrency = "USD" // Fallback if misconfigured
			}

			if responseCurrency != exchangeCurrency {
				result.Errors = append(result.Errors, fmt.Errorf(
					"currency mismatch from %s: expected %s, got %s (bids rejected)",
					bidderCode, exchangeCurrency, responseCurrency,
				))
				// Skip bids with wrong currency - can't safely compare prices
				continue
			}

			allBids = append(allBids, bidderResp.Bids...)
		}
	}

	result.Bids = allBids
	result.Latency = time.Since(start)
	return result
}

// buildEmptyResponse creates an empty bid response with optional NBR code
// P2-7: Using consolidated NoBidReason type from openrtb package
func (e *Exchange) buildEmptyResponse(req *openrtb.BidRequest, nbr openrtb.NoBidReason) *openrtb.BidResponse {
	return &openrtb.BidResponse{
		ID:      req.ID,
		SeatBid: []openrtb.SeatBid{},
		Cur:     e.config.DefaultCurrency,
		NBR:     int(nbr),
	}
}

// buildBidExtension creates the Prebid extension for a bid including targeting keys
// This is required for Prebid.js integration to work correctly
func (e *Exchange) buildBidExtension(vb ValidatedBid) *openrtb.BidExt {
	bid := vb.Bid.Bid
	bidType := string(vb.Bid.BidType)

	// Generate price bucket using medium granularity
	priceBucket := formatPriceBucket(bid.Price)

	// Determine display bidder code based on demand type:
	// - Platform demand: use "thenexusengine" (obfuscated)
	// - Publisher demand: use original bidder code (transparent)
	displayBidderCode := vb.BidderCode
	if vb.DemandType != adapters.DemandTypePublisher {
		displayBidderCode = adapters.PlatformSeatName // "thenexusengine"
	}

	// Build targeting keys that Prebid.js expects
	targeting := map[string]string{
		"hb_pb":     priceBucket,
		"hb_bidder": displayBidderCode,
		"hb_size":   fmt.Sprintf("%dx%d", bid.W, bid.H),
		"hb_pb_" + displayBidderCode:     priceBucket,
		"hb_bidder_" + displayBidderCode: displayBidderCode,
		"hb_size_" + displayBidderCode:   fmt.Sprintf("%dx%d", bid.W, bid.H),
	}

	// Add deal ID if present
	if bid.DealID != "" {
		targeting["hb_deal"] = bid.DealID
		targeting["hb_deal_"+displayBidderCode] = bid.DealID
	}

	return &openrtb.BidExt{
		Prebid: &openrtb.ExtBidPrebid{
			Type:      bidType,
			Targeting: targeting,
			Meta: &openrtb.ExtBidPrebidMeta{
				MediaType: bidType,
			},
		},
	}
}

// formatPriceBucket formats price using medium granularity (per Prebid.js spec)
// - $0.01 increments up to $5
// - $0.05 increments from $5-$10
// - $0.50 increments from $10-$20
// - Caps at $20
func formatPriceBucket(price float64) string {
	if price <= 0 {
		return "0.00"
	}
	if price > 20 {
		price = 20
	}

	var bucket float64
	switch {
	case price <= 5:
		bucket = float64(int(price*100)) / 100 // $0.01 increments
	case price <= 10:
		bucket = float64(int(price*20)) / 20 // $0.05 increments
	case price <= 20:
		bucket = float64(int(price*2)) / 2 // $0.50 increments
	default:
		bucket = 20
	}
	return fmt.Sprintf("%.2f", bucket)
}

// buildMinimalIDRRequest extracts only essential fields for IDR partner selection
// P1-15: Significantly reduces payload size vs sending full OpenRTB request
func (e *Exchange) buildMinimalIDRRequest(req *openrtb.BidRequest) *idr.MinimalRequest {
	// Extract domain/publisher info
	var domain, publisher, appBundle string
	var categories []string
	isApp := false

	if req.Site != nil {
		domain = req.Site.Domain
		categories = req.Site.Cat
		if req.Site.Publisher != nil {
			publisher = req.Site.Publisher.ID
		}
	} else if req.App != nil {
		isApp = true
		appBundle = req.App.Bundle
		categories = req.App.Cat
		if req.App.Publisher != nil {
			publisher = req.App.Publisher.ID
		}
	}

	// Extract geo info
	var country, region string
	if req.Device != nil && req.Device.Geo != nil {
		country = req.Device.Geo.Country
		region = req.Device.Geo.Region
	} else if req.User != nil && req.User.Geo != nil {
		country = req.User.Geo.Country
		region = req.User.Geo.Region
	}

	// Extract device type
	var deviceType string
	if req.Device != nil {
		switch req.Device.DeviceType {
		case 1:
			deviceType = "mobile"
		case 2:
			deviceType = "pc"
		case 3:
			deviceType = "ctv"
		case 4:
			deviceType = "phone"
		case 5:
			deviceType = "tablet"
		case 6:
			deviceType = "connected_device"
		case 7:
			deviceType = "set_top_box"
		}
	}

	// Build minimal impressions
	impressions := make([]idr.MinimalImp, 0, len(req.Imp))
	for _, imp := range req.Imp {
		mediaTypes := make([]string, 0, 4)
		var sizes []string

		if imp.Banner != nil {
			mediaTypes = append(mediaTypes, "banner")
			if imp.Banner.W > 0 && imp.Banner.H > 0 {
				sizes = append(sizes, fmt.Sprintf("%dx%d", imp.Banner.W, imp.Banner.H))
			}
			for _, f := range imp.Banner.Format {
				if f.W > 0 && f.H > 0 {
					sizes = append(sizes, fmt.Sprintf("%dx%d", f.W, f.H))
				}
			}
		}
		if imp.Video != nil {
			mediaTypes = append(mediaTypes, "video")
			if imp.Video.W > 0 && imp.Video.H > 0 {
				sizes = append(sizes, fmt.Sprintf("%dx%d", imp.Video.W, imp.Video.H))
			}
		}
		if imp.Native != nil {
			mediaTypes = append(mediaTypes, "native")
		}
		if imp.Audio != nil {
			mediaTypes = append(mediaTypes, "audio")
		}

		impressions = append(impressions, idr.BuildMinimalImp(imp.ID, mediaTypes, sizes))
	}

	return idr.BuildMinimalRequest(
		req.ID,
		domain,
		publisher,
		categories,
		isApp,
		appBundle,
		impressions,
		country,
		region,
		deviceType,
	)
}

// UpdateFPDConfig updates the FPD configuration at runtime
func (e *Exchange) UpdateFPDConfig(config *fpd.Config) {
	if config == nil {
		return
	}

	// Create new processor and filter before acquiring lock to minimize lock hold time
	newProcessor := fpd.NewProcessor(config)
	newFilter := fpd.NewEIDFilter(config)

	e.configMu.Lock()
	defer e.configMu.Unlock()
	e.config.FPD = config
	e.fpdProcessor = newProcessor
	e.eidFilter = newFilter
}

// GetFPDConfig returns the current FPD configuration
func (e *Exchange) GetFPDConfig() *fpd.Config {
	e.configMu.RLock()
	defer e.configMu.RUnlock()
	if e.config == nil {
		return nil
	}
	return e.config.FPD
}

// GetIDRClient returns the IDR client (for metrics/admin)
func (e *Exchange) GetIDRClient() *idr.Client {
	return e.idrClient
}

// getDemandType returns the demand type for a bidder (platform or publisher)
// Platform demand is obfuscated under "thenexusengine" seat, publisher demand is transparent
// Checks static registry first, then dynamic registry, defaults to platform
func (e *Exchange) getDemandType(bidderCode string, dynamicRegistry *ortb.DynamicRegistry) adapters.DemandType {
	// Check static registry first
	if awi, ok := e.registry.Get(bidderCode); ok {
		return awi.Info.DemandType
	}

	// Check dynamic registry
	if dynamicRegistry != nil {
		if adapter, ok := dynamicRegistry.Get(bidderCode); ok {
			return adapter.GetDemandType()
		}
	}

	// Default to platform (obfuscated) for unknown bidders
	return adapters.DemandTypePlatform
}
