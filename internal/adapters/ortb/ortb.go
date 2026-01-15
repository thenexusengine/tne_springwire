// Package ortb provides a generic OpenRTB 2.5/2.6 adapter
// that can be configured dynamically from Redis.
//
// This adapter allows creating custom bidder integrations
// without writing Go code for each bidder.
package ortb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// BidderConfig represents a dynamic bidder configuration
// loaded from Redis. This mirrors the Python BidderConfig model.
type BidderConfig struct {
	BidderCode        string                  `json:"bidder_code"`
	Name              string                  `json:"name"`
	Description       string                  `json:"description"`
	Endpoint          EndpointConfig          `json:"endpoint"`
	Capabilities      CapabilitiesConfig      `json:"capabilities"`
	RateLimits        RateLimitsConfig        `json:"rate_limits"`
	RequestTransform  RequestTransformConfig  `json:"request_transform"`
	ResponseTransform ResponseTransformConfig `json:"response_transform"`
	Status            string                  `json:"status"`
	GVLVendorID       *int                    `json:"gvl_vendor_id"`
	Priority          int                     `json:"priority"`
	MaintainerEmail   string                  `json:"maintainer_email"`
	AllowedPublishers []string                `json:"allowed_publishers"`
	BlockedPublishers []string                `json:"blocked_publishers"`
	AllowedCountries  []string                `json:"allowed_countries"`
	BlockedCountries  []string                `json:"blocked_countries"`
	DemandType        string                  `json:"demand_type"` // "platform" or "publisher"
}

// EndpointConfig holds endpoint configuration
type EndpointConfig struct {
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	TimeoutMS       int               `json:"timeout_ms"`
	ProtocolVersion string            `json:"protocol_version"`
	AuthType        string            `json:"auth_type"`
	AuthUsername    string            `json:"auth_username"`
	AuthPassword    string            `json:"auth_password"`
	AuthToken       string            `json:"auth_token"`
	AuthHeaderName  string            `json:"auth_header_name"`
	AuthHeaderValue string            `json:"auth_header_value"`
	CustomHeaders   map[string]string `json:"custom_headers"`
}

// CapabilitiesConfig holds capability information
type CapabilitiesConfig struct {
	MediaTypes     []string `json:"media_types"`
	Currencies     []string `json:"currencies"`
	SiteEnabled    bool     `json:"site_enabled"`
	AppEnabled     bool     `json:"app_enabled"`
	VideoProtocols []int    `json:"video_protocols"`
	VideoMimes     []string `json:"video_mimes"`
	SupportsGDPR   bool     `json:"supports_gdpr"`
	SupportsCCPA   bool     `json:"supports_ccpa"`
	SupportsCOPPA  bool     `json:"supports_coppa"`
	SupportsGPP    bool     `json:"supports_gpp"`
	SupportsSChain bool     `json:"supports_schain"`
	SupportsEIDs   bool     `json:"supports_eids"`
	SupportsFPD    bool     `json:"supports_first_party_data"`
	SupportsCTV    bool     `json:"supports_ctv"`
	SupportsAdPods bool     `json:"supports_ad_pods"`
}

// RateLimitsConfig holds rate limiting configuration
type RateLimitsConfig struct {
	QPSLimit        int `json:"qps_limit"`
	DailyLimit      int `json:"daily_limit"`
	ConcurrentLimit int `json:"concurrent_limit"`
}

// SChainNodeConfig holds a single supply chain node configuration
type SChainNodeConfig struct {
	ASI    string                 `json:"asi"`              // Canonical domain of the SSP/Exchange
	SID    string                 `json:"sid"`              // Seller ID
	HP     int                    `json:"hp"`               // Header bidding partner (1=yes, 0=no)
	RID    string                 `json:"rid,omitempty"`    // Request ID (optional)
	Name   string                 `json:"name,omitempty"`   // Entity name (optional)
	Domain string                 `json:"domain,omitempty"` // Entity domain (optional)
	Ext    map[string]interface{} `json:"ext,omitempty"`    // Extensions (optional)
}

// SChainAugmentConfig holds supply chain augmentation configuration
type SChainAugmentConfig struct {
	Enabled  bool               `json:"enabled"`            // Whether augmentation is enabled
	Nodes    []SChainNodeConfig `json:"nodes"`              // Nodes to append
	Complete *int               `json:"complete,omitempty"` // Override complete flag (nil = preserve)
	Version  string             `json:"version"`            // SChain version (default "1.0")
}

// RequestTransformConfig holds request transformation rules
type RequestTransformConfig struct {
	FieldMappings      map[string]string      `json:"field_mappings"`
	FieldAdditions     map[string]interface{} `json:"field_additions"`
	FieldRemovals      []string               `json:"field_removals"`
	ImpExtTemplate     map[string]interface{} `json:"imp_ext_template"`
	RequestExtTemplate map[string]interface{} `json:"request_ext_template"`
	SiteExtTemplate    map[string]interface{} `json:"site_ext_template"`
	UserExtTemplate    map[string]interface{} `json:"user_ext_template"`
	SeatID             string                 `json:"seat_id"`
	SChainAugment      SChainAugmentConfig    `json:"schain_augment"`
}

// ResponseTransformConfig holds response transformation rules
type ResponseTransformConfig struct {
	BidFieldMappings        map[string]string `json:"bid_field_mappings"`
	PriceAdjustment         float64           `json:"price_adjustment"`
	CurrencyConversion      bool              `json:"currency_conversion"`
	CreativeTypeMappings    map[string]string `json:"creative_type_mappings"`
	ExtractDurationFromVAST bool              `json:"extract_duration_from_vast"`
}

// GenericAdapter implements the Adapter interface for dynamic bidders
type GenericAdapter struct {
	config *BidderConfig
	mu     sync.RWMutex
}

// New creates a new generic adapter with the given configuration
func New(config *BidderConfig) *GenericAdapter {
	return &GenericAdapter{
		config: config,
	}
}

// UpdateConfig updates the adapter configuration (thread-safe)
func (a *GenericAdapter) UpdateConfig(config *BidderConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = config
}

// GetConfig returns the current configuration (thread-safe)
func (a *GenericAdapter) GetConfig() *BidderConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

// GetDemandType returns the demand type for this adapter (platform or publisher)
// Platform demand is obfuscated under "thenexusengine" seat, publisher demand is transparent
func (a *GenericAdapter) GetDemandType() adapters.DemandType {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.config == nil {
		return adapters.DemandTypePlatform // Default to platform (obfuscated)
	}
	switch a.config.DemandType {
	case "publisher":
		return adapters.DemandTypePublisher
	default:
		return adapters.DemandTypePlatform
	}
}

// MakeRequests builds HTTP requests for the bidder
func (a *GenericAdapter) MakeRequests(request *openrtb.BidRequest, extraInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	a.mu.RLock()
	config := a.config
	a.mu.RUnlock()

	var errors []error

	// Clone request for modification
	reqCopy := a.transformRequest(request, config)

	// Marshal request body
	requestBody, err := json.Marshal(reqCopy)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to marshal request: %w", err)}
	}

	// Build headers
	headers := a.buildHeaders(config)

	return []*adapters.RequestData{
		{
			Method:  config.Endpoint.Method,
			URI:     config.Endpoint.URL,
			Body:    requestBody,
			Headers: headers,
		},
	}, errors
}

// MakeBids parses bidder responses into bids
func (a *GenericAdapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	a.mu.RLock()
	config := a.config
	a.mu.RUnlock()

	// Handle no-bid response
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	// Handle bad request
	if responseData.StatusCode == http.StatusBadRequest {
		return nil, []error{fmt.Errorf("bad request from %s: %s", config.BidderCode, string(responseData.Body))}
	}

	// Handle other errors
	if responseData.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("unexpected status from %s: %d", config.BidderCode, responseData.StatusCode)}
	}

	// Parse response
	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{fmt.Errorf("failed to parse response from %s: %w", config.BidderCode, err)}
	}

	// Build adapter response
	response := &adapters.BidderResponse{
		Currency:   bidResp.Cur,
		ResponseID: bidResp.ID, // P2-5: Pass through for validation
		Bids:       make([]*adapters.TypedBid, 0),
	}

	// P2-NEW-1: Use BuildImpMap for O(1) bid type lookup instead of O(n) per bid
	impMap := adapters.BuildImpMap(request.Imp)

	// Process each bid
	for _, seatBid := range bidResp.SeatBid {
		for i := range seatBid.Bid {
			bid := &seatBid.Bid[i]

			// Apply response transformations
			a.transformBid(bid, config)

			response.Bids = append(response.Bids, &adapters.TypedBid{
				Bid:     bid,
				BidType: adapters.GetBidTypeFromMap(bid, impMap),
			})
		}
	}

	return response, nil
}

// transformRequest applies request transformations
func (a *GenericAdapter) transformRequest(request *openrtb.BidRequest, config *BidderConfig) *openrtb.BidRequest {
	// Create a copy to modify
	reqCopy := *request

	// Apply request extension template
	if len(config.RequestTransform.RequestExtTemplate) > 0 {
		reqCopy.Ext = mergeJSONExt(reqCopy.Ext, config.RequestTransform.RequestExtTemplate)
	}

	// Apply impression extension templates
	if len(config.RequestTransform.ImpExtTemplate) > 0 {
		// Deep copy impressions to avoid modifying original
		impCopy := make([]openrtb.Imp, len(reqCopy.Imp))
		copy(impCopy, reqCopy.Imp)
		for i := range impCopy {
			impCopy[i].Ext = mergeJSONExt(impCopy[i].Ext, config.RequestTransform.ImpExtTemplate)
		}
		reqCopy.Imp = impCopy
	}

	// Apply site extension template
	if reqCopy.Site != nil && len(config.RequestTransform.SiteExtTemplate) > 0 {
		siteCopy := *reqCopy.Site
		siteCopy.Ext = mergeJSONExt(siteCopy.Ext, config.RequestTransform.SiteExtTemplate)
		reqCopy.Site = &siteCopy
	}

	// Apply user extension template
	if reqCopy.User != nil && len(config.RequestTransform.UserExtTemplate) > 0 {
		userCopy := *reqCopy.User
		userCopy.Ext = mergeJSONExt(userCopy.Ext, config.RequestTransform.UserExtTemplate)
		reqCopy.User = &userCopy
	}

	// Apply schain augmentation
	if config.RequestTransform.SChainAugment.Enabled && len(config.RequestTransform.SChainAugment.Nodes) > 0 {
		reqCopy.Source = a.augmentSChain(reqCopy.Source, &config.RequestTransform.SChainAugment)
	}

	return &reqCopy
}

// augmentSChain adds supply chain nodes to the request's schain
func (a *GenericAdapter) augmentSChain(source *openrtb.Source, augment *SChainAugmentConfig) *openrtb.Source {
	// Create source if not present
	var sourceCopy openrtb.Source
	if source != nil {
		sourceCopy = *source
	}

	// Initialize schain if not present
	if sourceCopy.SChain == nil {
		version := augment.Version
		if version == "" {
			version = "1.0"
		}
		sourceCopy.SChain = &openrtb.SupplyChain{
			Ver:      version,
			Complete: 1, // Default to complete
			Nodes:    make([]openrtb.SupplyChainNode, 0),
		}
	} else {
		// Deep copy existing schain to avoid modifying original
		schainCopy := *sourceCopy.SChain
		if len(sourceCopy.SChain.Nodes) > 0 {
			schainCopy.Nodes = make([]openrtb.SupplyChainNode, len(sourceCopy.SChain.Nodes))
			copy(schainCopy.Nodes, sourceCopy.SChain.Nodes)
		}
		sourceCopy.SChain = &schainCopy
	}

	// Override complete flag if specified
	if augment.Complete != nil {
		sourceCopy.SChain.Complete = *augment.Complete
	}

	// Override version if specified and non-empty
	if augment.Version != "" {
		sourceCopy.SChain.Ver = augment.Version
	}

	// Append configured nodes
	for _, nodeConfig := range augment.Nodes {
		node := openrtb.SupplyChainNode{
			ASI:    nodeConfig.ASI,
			SID:    nodeConfig.SID,
			HP:     nodeConfig.HP,
			RID:    nodeConfig.RID,
			Name:   nodeConfig.Name,
			Domain: nodeConfig.Domain,
		}

		// Convert ext map to json.RawMessage if present
		if len(nodeConfig.Ext) > 0 {
			extBytes, err := json.Marshal(nodeConfig.Ext)
			if err == nil {
				node.Ext = extBytes
			}
		}

		sourceCopy.SChain.Nodes = append(sourceCopy.SChain.Nodes, node)
	}

	return &sourceCopy
}

// mergeJSONExt merges additional fields into an existing json.RawMessage
func mergeJSONExt(existing json.RawMessage, additions map[string]interface{}) json.RawMessage {
	if len(additions) == 0 {
		return existing
	}

	// Start with existing data or empty object
	base := make(map[string]interface{})
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &base) //nolint:errcheck
	}

	// Merge additions
	for k, v := range additions {
		base[k] = v
	}

	// Marshal back
	result, err := json.Marshal(base)
	if err != nil {
		return existing
	}
	return result
}

// transformBid applies response transformations to a bid
func (a *GenericAdapter) transformBid(bid *openrtb.Bid, config *BidderConfig) {
	// Apply price adjustment
	if config.ResponseTransform.PriceAdjustment != 0 && config.ResponseTransform.PriceAdjustment != 1.0 {
		bid.Price = bid.Price * config.ResponseTransform.PriceAdjustment
	}
}

// buildHeaders creates HTTP headers for the request
func (a *GenericAdapter) buildHeaders(config *BidderConfig) http.Header {
	headers := http.Header{}

	// Standard OpenRTB headers
	headers.Set("Content-Type", "application/json;charset=utf-8")
	headers.Set("Accept", "application/json")
	headers.Set("X-OpenRTB-Version", config.Endpoint.ProtocolVersion)

	// Authentication headers
	switch config.Endpoint.AuthType {
	case "basic":
		if config.Endpoint.AuthUsername != "" {
			credentials := config.Endpoint.AuthUsername + ":" + config.Endpoint.AuthPassword
			encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
			headers.Set("Authorization", "Basic "+encoded)
		}
	case "bearer":
		if config.Endpoint.AuthToken != "" {
			headers.Set("Authorization", "Bearer "+config.Endpoint.AuthToken)
		}
	case "header":
		if config.Endpoint.AuthHeaderName != "" && config.Endpoint.AuthHeaderValue != "" {
			headers.Set(config.Endpoint.AuthHeaderName, config.Endpoint.AuthHeaderValue)
		}
	}

	// Custom headers
	for k, v := range config.Endpoint.CustomHeaders {
		headers.Set(k, v)
	}

	return headers
}

// Info returns bidder information based on the configuration
func (a *GenericAdapter) Info() adapters.BidderInfo {
	a.mu.RLock()
	config := a.config
	a.mu.RUnlock()

	info := adapters.BidderInfo{
		Enabled: config.Status == "active" || config.Status == "testing",
		Maintainer: &adapters.MaintainerInfo{
			Email: config.MaintainerEmail,
		},
		Endpoint: config.Endpoint.URL,
	}

	// Set GVL Vendor ID if present
	if config.GVLVendorID != nil {
		info.GVLVendorID = *config.GVLVendorID
	}

	// Build capabilities
	info.Capabilities = &adapters.CapabilitiesInfo{}

	mediaTypes := make([]adapters.BidType, 0)
	for _, mt := range config.Capabilities.MediaTypes {
		switch strings.ToLower(mt) {
		case "banner":
			mediaTypes = append(mediaTypes, adapters.BidTypeBanner)
		case "video":
			mediaTypes = append(mediaTypes, adapters.BidTypeVideo)
		case "native":
			mediaTypes = append(mediaTypes, adapters.BidTypeNative)
		case "audio":
			mediaTypes = append(mediaTypes, adapters.BidTypeAudio)
		}
	}

	if config.Capabilities.SiteEnabled {
		info.Capabilities.Site = &adapters.PlatformInfo{
			MediaTypes: mediaTypes,
		}
	}

	if config.Capabilities.AppEnabled {
		info.Capabilities.App = &adapters.PlatformInfo{
			MediaTypes: mediaTypes,
		}
	}

	return info
}

// IsEnabled checks if the bidder is enabled
func (a *GenericAdapter) IsEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config.Status == "active" || a.config.Status == "testing"
}

// GetTimeout returns the configured timeout duration
func (a *GenericAdapter) GetTimeout() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return time.Duration(a.config.Endpoint.TimeoutMS) * time.Millisecond
}

// GetGVLVendorID returns the Global Vendor List ID for TCF consent checking
func (a *GenericAdapter) GetGVLVendorID() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.config.GVLVendorID != nil {
		return *a.config.GVLVendorID
	}
	return 0
}

// CanBidForPublisher checks if this bidder can bid for a specific publisher
func (a *GenericAdapter) CanBidForPublisher(publisherID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check blocked publishers
	for _, blocked := range a.config.BlockedPublishers {
		if blocked == publisherID {
			return false
		}
	}

	// Check allowed publishers (empty = all allowed)
	if len(a.config.AllowedPublishers) > 0 {
		for _, allowed := range a.config.AllowedPublishers {
			if allowed == publisherID {
				return true
			}
		}
		return false
	}

	return true
}

// CanBidForCountry checks if this bidder can bid for a specific country
func (a *GenericAdapter) CanBidForCountry(country string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	country = strings.ToUpper(country)

	// Check blocked countries
	for _, blocked := range a.config.BlockedCountries {
		if strings.ToUpper(blocked) == country {
			return false
		}
	}

	// Check allowed countries (empty = all allowed)
	if len(a.config.AllowedCountries) > 0 {
		for _, allowed := range a.config.AllowedCountries {
			if strings.ToUpper(allowed) == country {
				return true
			}
		}
		return false
	}

	return true
}
