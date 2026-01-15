package floors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// StaticProvider provides floors from static configuration
type StaticProvider struct {
	mu   sync.RWMutex
	data map[string]*FloorData // keyed by publisher:domain
}

// NewStaticProvider creates a static floor provider
func NewStaticProvider() *StaticProvider {
	return &StaticProvider{
		data: make(map[string]*FloorData),
	}
}

// Name returns the provider name
func (p *StaticProvider) Name() string {
	return "static"
}

// GetFloors returns floor data from static configuration
func (p *StaticProvider) GetFloors(ctx context.Context, req *openrtb.BidRequest) (*FloorData, error) {
	key := p.getKey(req)

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Try exact match first
	if data, exists := p.data[key]; exists {
		return data, nil
	}

	// Try publisher-only match
	pubID := p.getPublisherID(req)
	if pubID != "" {
		if data, exists := p.data[pubID+":"]; exists {
			return data, nil
		}
	}

	// Try global default
	if data, exists := p.data[":"]; exists {
		return data, nil
	}

	return nil, nil
}

// SetFloors sets floor data for a publisher/domain
// Use empty domain for publisher-wide floors
// Use empty publisher AND domain for global default
func (p *StaticProvider) SetFloors(publisherID, domain string, data *FloorData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := publisherID + ":" + domain
	p.data[key] = data
}

// SetRules is a convenience method to set simple floor rules
func (p *StaticProvider) SetRules(publisherID, domain string, rules []FloorRule, defaultFloor float64, currency string) {
	if currency == "" {
		currency = "USD"
	}
	p.SetFloors(publisherID, domain, &FloorData{
		Rules:        rules,
		DefaultFloor: defaultFloor,
		Currency:     currency,
		FetchedAt:    time.Now(),
	})
}

// RemoveFloors removes floor data for a publisher/domain
func (p *StaticProvider) RemoveFloors(publisherID, domain string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.data, publisherID+":"+domain)
}

func (p *StaticProvider) getKey(req *openrtb.BidRequest) string {
	return p.getPublisherID(req) + ":" + p.getDomain(req)
}

func (p *StaticProvider) getPublisherID(req *openrtb.BidRequest) string {
	if req.Site != nil && req.Site.Publisher != nil {
		return req.Site.Publisher.ID
	}
	if req.App != nil && req.App.Publisher != nil {
		return req.App.Publisher.ID
	}
	return ""
}

func (p *StaticProvider) getDomain(req *openrtb.BidRequest) string {
	if req.Site != nil {
		return req.Site.Domain
	}
	if req.App != nil {
		return req.App.Bundle
	}
	return ""
}

// APIProvider fetches floors from an external API (e.g., pubX)
type APIProvider struct {
	mu         sync.RWMutex
	endpoint   string
	apiKey     string
	httpClient *http.Client
	cache      map[string]*cachedAPIResponse
	cacheTTL   time.Duration
}

type cachedAPIResponse struct {
	data      *FloorData
	fetchedAt time.Time
}

// APIProviderConfig holds configuration for the API provider
type APIProviderConfig struct {
	// Endpoint is the floor API URL
	// Supports template variables: {{publisher_id}}, {{domain}}
	Endpoint string `json:"endpoint"`

	// APIKey for authentication (sent as X-API-Key header)
	APIKey string `json:"api_key"`

	// Timeout for API requests
	Timeout time.Duration `json:"timeout"`

	// CacheTTL for caching API responses
	CacheTTL time.Duration `json:"cache_ttl"`
}

// DefaultAPIProviderConfig returns default config for API provider
func DefaultAPIProviderConfig() *APIProviderConfig {
	return &APIProviderConfig{
		Timeout:  100 * time.Millisecond,
		CacheTTL: 5 * time.Minute,
	}
}

// NewAPIProvider creates an API-based floor provider
// This is designed for integration with pubX or similar floor services
func NewAPIProvider(config *APIProviderConfig) *APIProvider {
	if config == nil {
		config = DefaultAPIProviderConfig()
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}

	cacheTTL := config.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	return &APIProvider{
		endpoint: config.Endpoint,
		apiKey:   config.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cache:    make(map[string]*cachedAPIResponse),
		cacheTTL: cacheTTL,
	}
}

// Name returns the provider name
func (p *APIProvider) Name() string {
	return "api"
}

// GetFloors fetches floor data from the external API
func (p *APIProvider) GetFloors(ctx context.Context, req *openrtb.BidRequest) (*FloorData, error) {
	if p.endpoint == "" {
		return nil, nil
	}

	// Build cache key
	cacheKey := p.getCacheKey(req)

	// Check cache
	p.mu.RLock()
	cached, exists := p.cache[cacheKey]
	p.mu.RUnlock()

	if exists && time.Since(cached.fetchedAt) < p.cacheTTL {
		return cached.data, nil
	}

	// Build request URL
	url := p.buildURL(req)

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	httpReq.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("X-API-Key", p.apiKey)
	}

	// Make request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		logger.Log.Debug().
			Err(err).
			Str("url", url).
			Msg("Failed to fetch floors from API")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Log.Debug().
			Int("status", resp.StatusCode).
			Str("url", url).
			Msg("Floor API returned non-200 status")
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var floorData FloorData
	if err := json.Unmarshal(body, &floorData); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	floorData.FetchedAt = time.Now()

	// Cache the result
	p.mu.Lock()
	p.cache[cacheKey] = &cachedAPIResponse{
		data:      &floorData,
		fetchedAt: time.Now(),
	}
	p.mu.Unlock()

	return &floorData, nil
}

// SetEndpoint updates the API endpoint
func (p *APIProvider) SetEndpoint(endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endpoint = endpoint
}

// SetAPIKey updates the API key
func (p *APIProvider) SetAPIKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apiKey = key
}

// ClearCache clears the response cache
func (p *APIProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*cachedAPIResponse)
}

func (p *APIProvider) buildURL(req *openrtb.BidRequest) string {
	url := p.endpoint

	// Replace template variables
	pubID := ""
	domain := ""

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

	// Simple string replacement for template variables
	url = replaceAll(url, "{{publisher_id}}", pubID)
	url = replaceAll(url, "{{domain}}", domain)

	return url
}

func (p *APIProvider) getCacheKey(req *openrtb.BidRequest) string {
	pubID := ""
	domain := ""

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

// replaceAll is a simple string replace helper
func replaceAll(s, old, new string) string {
	result := s
	for {
		idx := indexOf(result, old)
		if idx == -1 {
			break
		}
		result = result[:idx] + new + result[idx+len(old):]
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// PubXProvider is a specialized provider for pubX floors
// This wraps the APIProvider with pubX-specific configuration
type PubXProvider struct {
	*APIProvider
	accountID string
}

// PubXConfig holds pubX-specific configuration
type PubXConfig struct {
	// AccountID is your pubX account identifier
	AccountID string `json:"account_id"`

	// APIKey for pubX authentication
	APIKey string `json:"api_key"`

	// BaseURL for pubX API (default: https://api.pubx.ai)
	BaseURL string `json:"base_url"`

	// Timeout for API requests
	Timeout time.Duration `json:"timeout"`

	// CacheTTL for caching responses
	CacheTTL time.Duration `json:"cache_ttl"`
}

// NewPubXProvider creates a provider configured for pubX floors
func NewPubXProvider(config *PubXConfig) *PubXProvider {
	if config == nil {
		config = &PubXConfig{}
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.pubx.ai"
	}

	endpoint := fmt.Sprintf("%s/v1/floors/{{publisher_id}}/{{domain}}", baseURL)

	apiConfig := &APIProviderConfig{
		Endpoint: endpoint,
		APIKey:   config.APIKey,
		Timeout:  config.Timeout,
		CacheTTL: config.CacheTTL,
	}

	return &PubXProvider{
		APIProvider: NewAPIProvider(apiConfig),
		accountID:   config.AccountID,
	}
}

// Name returns the provider name
func (p *PubXProvider) Name() string {
	return "pubx"
}

// GetFloors fetches floor data from pubX
func (p *PubXProvider) GetFloors(ctx context.Context, req *openrtb.BidRequest) (*FloorData, error) {
	// Add account ID to request context or headers if needed
	return p.APIProvider.GetFloors(ctx, req)
}
