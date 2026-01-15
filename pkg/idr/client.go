// Package idr provides a client for the Python IDR service
package idr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// P2-4: Maximum IDR response size to prevent OOM from malformed responses
const maxIDRResponseSize = 1024 * 1024 // 1MB - plenty for partner selection

// Client communicates with the Python IDR service
type Client struct {
	baseURL        string
	apiKey         string // Internal API key for service-to-service auth
	httpClient     *http.Client
	timeout        time.Duration
	circuitBreaker *CircuitBreaker
}

// newIDRTransport creates a connection-pooled transport for IDR requests
// P1-14: Optimize for low-latency, high-frequency calls to local IDR service
func newIDRTransport(timeout time.Duration) *http.Transport {
	return &http.Transport{
		// Connection pool - IDR is a single host, so per-host settings matter most
		MaxIdleConns:        20,                // Keep connections ready
		MaxIdleConnsPerHost: 20,                // All connections are to IDR
		MaxConnsPerHost:     100,               // Allow concurrent requests during load spikes
		IdleConnTimeout:     120 * time.Second, // Keep connections alive longer for reuse

		// Low timeouts for fast local service
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 500 * time.Millisecond,

		// Keep-alive for connection reuse
		DisableKeepAlives: false,

		// Disable compression - IDR responses are small, compression adds latency
		DisableCompression: true,
	}
}

// NewClient creates a new IDR client with connection pooling
func NewClient(baseURL string, timeout time.Duration, apiKey string) *Client {
	if timeout == 0 {
		timeout = 150 * time.Millisecond // IDR timeout - allows for Python processing
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: newIDRTransport(timeout),
		},
		timeout:        timeout,
		circuitBreaker: NewCircuitBreaker(DefaultCircuitBreakerConfig()),
	}
}

// NewClientWithCircuitBreaker creates a new IDR client with custom circuit breaker config
func NewClientWithCircuitBreaker(baseURL string, timeout time.Duration, apiKey string, cbConfig *CircuitBreakerConfig) *Client {
	if timeout == 0 {
		timeout = 150 * time.Millisecond // IDR timeout - allows for Python processing
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: newIDRTransport(timeout),
		},
		timeout:        timeout,
		circuitBreaker: NewCircuitBreaker(cbConfig),
	}
}

// SelectPartnersRequest is the request to select partners
type SelectPartnersRequest struct {
	Request          json.RawMessage `json:"request"`           // OpenRTB request
	AvailableBidders []string        `json:"available_bidders"` // Bidders to consider
}

// MinimalRequest contains only the fields IDR needs for partner selection
// P1-15: Reduces payload size significantly vs sending full OpenRTB request
type MinimalRequest struct {
	ID         string       `json:"id"`
	Site       *MinimalSite `json:"site,omitempty"`
	App        *MinimalApp  `json:"app,omitempty"`
	Imp        []MinimalImp `json:"imp"`
	Geo        *MinimalGeo  `json:"geo,omitempty"`
	DeviceType string       `json:"device_type,omitempty"`
}

// MinimalSite contains essential site info for partner selection
type MinimalSite struct {
	Domain     string   `json:"domain,omitempty"`
	Publisher  string   `json:"publisher,omitempty"`
	Categories []string `json:"cat,omitempty"`
}

// MinimalApp contains essential app info for partner selection
type MinimalApp struct {
	Bundle     string   `json:"bundle,omitempty"`
	Publisher  string   `json:"publisher,omitempty"`
	Categories []string `json:"cat,omitempty"`
}

// MinimalImp contains essential impression info
type MinimalImp struct {
	ID         string   `json:"id"`
	MediaTypes []string `json:"media_types"`     // "banner", "video", "native", "audio"
	Sizes      []string `json:"sizes,omitempty"` // "300x250", "728x90", etc.
}

// MinimalGeo contains essential geo info
type MinimalGeo struct {
	Country string `json:"country,omitempty"`
	Region  string `json:"region,omitempty"`
}

// SelectPartnersResponse is the response from partner selection
type SelectPartnersResponse struct {
	SelectedBidders  []SelectedBidder `json:"selected_bidders"`
	ExcludedBidders  []ExcludedBidder `json:"excluded_bidders,omitempty"`
	Mode             string           `json:"mode"` // "normal", "shadow", "bypass"
	ProcessingTimeMs float64          `json:"processing_time_ms"`
}

// SelectedBidder represents a selected bidder
type SelectedBidder struct {
	BidderCode string  `json:"bidder_code"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`             // ANCHOR, HIGH_SCORE, DIVERSITY, EXPLORATION, etc.
	Category   string  `json:"category,omitempty"` // Bidder category for diversity
}

// ExcludedBidder represents an excluded bidder (shadow mode)
type ExcludedBidder struct {
	BidderCode string  `json:"bidder_code"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
}

// SelectPartners calls the IDR service to select optimal bidders
// Protected by circuit breaker - returns nil if circuit is open (fail open)
func (c *Client) SelectPartners(ctx context.Context, ortbRequest json.RawMessage, availableBidders []string) (*SelectPartnersResponse, error) {
	var result *SelectPartnersResponse
	var callErr error

	err := c.circuitBreaker.Execute(func() error {
		reqBody := SelectPartnersRequest{
			Request:          ortbRequest,
			AvailableBidders: availableBidders,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		url := c.baseURL + "/internal/select"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("X-Internal-API-Key", c.apiKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to call IDR service: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Read error response body for better debugging
			if errBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024)); err == nil && len(errBody) > 0 {
				return fmt.Errorf("IDR service returned status %d: %s", resp.StatusCode, string(errBody))
			}
			return fmt.Errorf("IDR service returned status %d", resp.StatusCode)
		}

		// P2-4: Limit response size to prevent OOM from malformed responses
		limitedReader := io.LimitReader(resp.Body, maxIDRResponseSize)
		var response SelectPartnersResponse
		if err := json.NewDecoder(limitedReader).Decode(&response); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		result = &response
		return nil
	})

	// If circuit is open, fail open (return nil, allowing all bidders)
	if errors.Is(err, ErrCircuitOpen) {
		return nil, nil // Caller should fall back to all bidders
	}

	if err != nil {
		callErr = err
	}

	return result, callErr
}

// SelectPartnersMinimal calls IDR with a minimal payload for better performance
// P1-15: Uses MinimalRequest instead of full OpenRTB to reduce payload size
func (c *Client) SelectPartnersMinimal(ctx context.Context, minReq *MinimalRequest, availableBidders []string) (*SelectPartnersResponse, error) {
	var result *SelectPartnersResponse
	var callErr error

	err := c.circuitBreaker.Execute(func() error {
		reqJSON, err := json.Marshal(minReq)
		if err != nil {
			return fmt.Errorf("failed to marshal minimal request: %w", err)
		}

		reqBody := SelectPartnersRequest{
			Request:          reqJSON,
			AvailableBidders: availableBidders,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		url := c.baseURL + "/internal/select"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("X-Internal-API-Key", c.apiKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to call IDR service: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("IDR service returned status %d", resp.StatusCode)
		}

		limitedReader := io.LimitReader(resp.Body, maxIDRResponseSize)
		var response SelectPartnersResponse
		if err := json.NewDecoder(limitedReader).Decode(&response); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		result = &response
		return nil
	})

	if errors.Is(err, ErrCircuitOpen) {
		return nil, nil
	}

	if err != nil {
		callErr = err
	}

	return result, callErr
}

// CircuitBreakerStats returns the current circuit breaker statistics
func (c *Client) CircuitBreakerStats() CircuitBreakerStats {
	return c.circuitBreaker.Stats()
}

// IsCircuitOpen returns true if the circuit breaker is open
func (c *Client) IsCircuitOpen() bool {
	return c.circuitBreaker.IsOpen()
}

// ResetCircuitBreaker resets the circuit breaker to closed state
func (c *Client) ResetCircuitBreaker() {
	c.circuitBreaker.Reset()
}

// GetConfig retrieves current IDR configuration
func (c *Client) GetConfig(ctx context.Context) (map[string]interface{}, error) {
	url := c.baseURL + "/api/config"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call IDR service: %w", err)
	}
	defer resp.Body.Close()

	var config map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return config, nil
}

// SetBypassMode enables/disables bypass mode
func (c *Client) SetBypassMode(ctx context.Context, enabled bool) error {
	return c.setMode(ctx, "/api/mode/bypass", enabled)
}

// SetShadowMode enables/disables shadow mode
func (c *Client) SetShadowMode(ctx context.Context, enabled bool) error {
	return c.setMode(ctx, "/api/mode/shadow", enabled)
}

func (c *Client) setMode(ctx context.Context, path string, enabled bool) error {
	body, err := json.Marshal(map[string]bool{"enabled": enabled})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call IDR service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error response body for better debugging
		if errBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024)); err == nil && len(errBody) > 0 {
			return fmt.Errorf("IDR service returned status %d: %s", resp.StatusCode, string(errBody))
		}
		return fmt.Errorf("IDR service returned status %d", resp.StatusCode)
	}

	return nil
}

// HealthCheck checks if IDR service is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	url := c.baseURL + "/health"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IDR service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

// FPDConfig represents First Party Data configuration from the IDR service
type FPDConfig struct {
	Enabled             bool   `json:"enabled"`
	SiteEnabled         bool   `json:"site_enabled"`
	UserEnabled         bool   `json:"user_enabled"`
	ImpEnabled          bool   `json:"imp_enabled"`
	GlobalEnabled       bool   `json:"global_enabled"`
	BidderConfigEnabled bool   `json:"bidderconfig_enabled"`
	ContentEnabled      bool   `json:"content_enabled"`
	EIDsEnabled         bool   `json:"eids_enabled"`
	EIDSources          string `json:"eid_sources"` // Comma-separated list
}

// GetFPDConfig retrieves FPD configuration from the IDR service
func (c *Client) GetFPDConfig(ctx context.Context) (*FPDConfig, error) {
	config, err := c.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Extract FPD section from config
	fpdSection, ok := config["fpd"].(map[string]interface{})
	if !ok {
		// Return default config if no FPD section
		return &FPDConfig{
			Enabled:        true,
			SiteEnabled:    true,
			UserEnabled:    true,
			ImpEnabled:     true,
			ContentEnabled: true,
			EIDsEnabled:    true,
			EIDSources:     "liveramp.com,uidapi.com,id5-sync.com,criteo.com",
		}, nil
	}

	// Parse FPD config
	fpd := &FPDConfig{}

	if v, ok := fpdSection["enabled"].(bool); ok {
		fpd.Enabled = v
	}
	if v, ok := fpdSection["site_enabled"].(bool); ok {
		fpd.SiteEnabled = v
	}
	if v, ok := fpdSection["user_enabled"].(bool); ok {
		fpd.UserEnabled = v
	}
	if v, ok := fpdSection["imp_enabled"].(bool); ok {
		fpd.ImpEnabled = v
	}
	if v, ok := fpdSection["global_enabled"].(bool); ok {
		fpd.GlobalEnabled = v
	}
	if v, ok := fpdSection["bidderconfig_enabled"].(bool); ok {
		fpd.BidderConfigEnabled = v
	}
	if v, ok := fpdSection["content_enabled"].(bool); ok {
		fpd.ContentEnabled = v
	}
	if v, ok := fpdSection["eids_enabled"].(bool); ok {
		fpd.EIDsEnabled = v
	}
	if v, ok := fpdSection["eid_sources"].(string); ok {
		fpd.EIDSources = v
	}

	return fpd, nil
}

// BuildMinimalRequest creates a MinimalRequest from extracted OpenRTB fields
// Helper for callers who need to manually construct the minimal request
func BuildMinimalRequest(
	requestID string,
	domain string,
	publisher string,
	categories []string,
	isApp bool,
	appBundle string,
	impressions []MinimalImp,
	country string,
	region string,
	deviceType string,
) *MinimalRequest {
	req := &MinimalRequest{
		ID:         requestID,
		Imp:        impressions,
		DeviceType: deviceType,
	}

	if isApp {
		req.App = &MinimalApp{
			Bundle:     appBundle,
			Publisher:  publisher,
			Categories: categories,
		}
	} else {
		req.Site = &MinimalSite{
			Domain:     domain,
			Publisher:  publisher,
			Categories: categories,
		}
	}

	if country != "" || region != "" {
		req.Geo = &MinimalGeo{
			Country: country,
			Region:  region,
		}
	}

	return req
}

// BuildMinimalImp creates a MinimalImp from impression data
func BuildMinimalImp(impID string, mediaTypes []string, sizes []string) MinimalImp {
	return MinimalImp{
		ID:         impID,
		MediaTypes: mediaTypes,
		Sizes:      sizes,
	}
}
