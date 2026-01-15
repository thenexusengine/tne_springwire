package idr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:5050", 0, "test-api-key")
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.timeout != 150*time.Millisecond {
		t.Errorf("expected default 150ms timeout, got %v", client.timeout)
	}

	if client.apiKey != "test-api-key" {
		t.Errorf("expected api key 'test-api-key', got %s", client.apiKey)
	}
}

func TestNewClientWithCircuitBreaker(t *testing.T) {
	cbConfig := &CircuitBreakerConfig{
		FailureThreshold: 10,
		SuccessThreshold: 5,
		Timeout:          time.Second,
	}

	client := NewClientWithCircuitBreaker("http://localhost:5050", 100*time.Millisecond, "", cbConfig)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.timeout != 100*time.Millisecond {
		t.Errorf("expected 100ms timeout, got %v", client.timeout)
	}

	stats := client.CircuitBreakerStats()
	if stats.State != StateClosed {
		t.Errorf("expected closed state, got %s", stats.State)
	}
}

func TestSelectPartnersSuccess(t *testing.T) {
	// Create mock IDR server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/select" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify API key header
		if r.Header.Get("X-Internal-API-Key") != "test-key" {
			t.Errorf("expected X-Internal-API-Key header, got %s", r.Header.Get("X-Internal-API-Key"))
		}

		var req SelectPartnersRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Return mock response
		resp := SelectPartnersResponse{
			SelectedBidders: []SelectedBidder{
				{BidderCode: "appnexus", Score: 0.95, Reason: "HIGH_SCORE"},
				{BidderCode: "rubicon", Score: 0.85, Reason: "DIVERSITY"},
			},
			ExcludedBidders: []ExcludedBidder{
				{BidderCode: "pubmatic", Score: 0.30, Reason: "LOW_SCORE"},
			},
			Mode:             "normal",
			ProcessingTimeMs: 5.2,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "test-key")

	req := json.RawMessage(`{"id":"test-1"}`)
	bidders := []string{"appnexus", "rubicon", "pubmatic"}

	resp, err := client.SelectPartners(context.Background(), req, bidders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if len(resp.SelectedBidders) != 2 {
		t.Errorf("expected 2 selected bidders, got %d", len(resp.SelectedBidders))
	}

	if resp.SelectedBidders[0].BidderCode != "appnexus" {
		t.Errorf("expected appnexus first, got %s", resp.SelectedBidders[0].BidderCode)
	}

	if len(resp.ExcludedBidders) != 1 {
		t.Errorf("expected 1 excluded bidder, got %d", len(resp.ExcludedBidders))
	}
}

func TestSelectPartnersServerError(t *testing.T) {
	// Create mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	req := json.RawMessage(`{"id":"test-1"}`)
	bidders := []string{"appnexus"}

	_, err := client.SelectPartners(context.Background(), req, bidders)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSelectPartnersCircuitOpen(t *testing.T) {
	// Create server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientWithCircuitBreaker(server.URL, 100*time.Millisecond, "", &CircuitBreakerConfig{
		FailureThreshold: 2, // Open after 2 failures
		SuccessThreshold: 1,
		Timeout:          time.Second,
	})

	req := json.RawMessage(`{"id":"test-1"}`)
	bidders := []string{"appnexus"}

	// Trigger failures to open circuit
	for i := 0; i < 2; i++ {
		client.SelectPartners(context.Background(), req, bidders)
	}

	// Circuit should be open now
	if !client.IsCircuitOpen() {
		t.Error("expected circuit to be open")
	}

	// Next call should return nil, nil (fail open)
	resp, err := client.SelectPartners(context.Background(), req, bidders)
	if err != nil {
		t.Errorf("expected nil error when circuit open, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when circuit open, got %v", resp)
	}
}

func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("unexpected health check error: %v", err)
	}
}

func TestHealthCheckFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	err := client.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy service")
	}
}

func TestGetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config" {
			config := map[string]interface{}{
				"mode":         "normal",
				"max_partners": 10,
				"fpd": map[string]interface{}{
					"enabled":      true,
					"eids_enabled": true,
					"eid_sources":  "liveramp.com,uidapi.com",
				},
			}
			json.NewEncoder(w).Encode(config)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	config, err := client.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config["mode"] != "normal" {
		t.Errorf("expected normal mode, got %v", config["mode"])
	}
}

func TestGetFPDConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"fpd": map[string]interface{}{
				"enabled":      true,
				"site_enabled": true,
				"user_enabled": true,
				"eids_enabled": true,
				"eid_sources":  "liveramp.com,id5-sync.com",
			},
		}
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	fpdConfig, err := client.GetFPDConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fpdConfig.Enabled {
		t.Error("expected FPD to be enabled")
	}
	if !fpdConfig.EIDsEnabled {
		t.Error("expected EIDs to be enabled")
	}
	if fpdConfig.EIDSources != "liveramp.com,id5-sync.com" {
		t.Errorf("unexpected EID sources: %s", fpdConfig.EIDSources)
	}
}

func TestGetFPDConfigNoFPDSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return config without FPD section
		config := map[string]interface{}{
			"mode": "normal",
		}
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	fpdConfig, err := client.GetFPDConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return defaults
	if !fpdConfig.Enabled {
		t.Error("expected default FPD to be enabled")
	}
}

func TestSetBypassMode(t *testing.T) {
	var receivedEnabled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/mode/bypass" && r.Method == "POST" {
			var body map[string]bool
			json.NewDecoder(r.Body).Decode(&body)
			receivedEnabled = body["enabled"]
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	err := client.SetBypassMode(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !receivedEnabled {
		t.Error("expected bypass mode to be enabled")
	}
}

func TestSetShadowMode(t *testing.T) {
	var receivedEnabled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/mode/shadow" && r.Method == "POST" {
			var body map[string]bool
			json.NewDecoder(r.Body).Decode(&body)
			receivedEnabled = body["enabled"]
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, 100*time.Millisecond, "")

	err := client.SetShadowMode(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !receivedEnabled {
		t.Error("expected shadow mode to be enabled")
	}
}

func TestResetCircuitBreaker(t *testing.T) {
	client := NewClient("http://localhost:5050", 100*time.Millisecond, "")

	// Force open the circuit
	client.circuitBreaker.ForceOpen()

	if !client.IsCircuitOpen() {
		t.Error("expected circuit to be open")
	}

	// Reset
	client.ResetCircuitBreaker()

	if client.IsCircuitOpen() {
		t.Error("expected circuit to be closed after reset")
	}
}

func TestContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, 1*time.Second, "")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := json.RawMessage(`{"id":"test-1"}`)
	bidders := []string{"appnexus"}

	_, err := client.SelectPartners(ctx, req, bidders)

	// Should get an error due to context timeout
	if err == nil {
		t.Error("expected error due to context timeout")
	}
}

// Additional tests for full coverage

func TestSelectPartnersMinimal_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/select" {
			t.Errorf("Expected path /internal/select, got %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Return mock response with correct structure
		response := SelectPartnersResponse{
			SelectedBidders: []SelectedBidder{
				{BidderCode: "appnexus", Score: 0.9},
				{BidderCode: "rubicon", Score: 0.8},
			},
			Mode: "normal",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second, "test-key")

	minReq := &MinimalRequest{
		ID: "test-123",
		Site: &MinimalSite{
			Domain:    "example.com",
			Publisher: "pub-1",
		},
		Imp: []MinimalImp{
			{ID: "imp-1", MediaTypes: []string{"banner"}, Sizes: []string{"300x250"}},
		},
	}

	result, err := client.SelectPartnersMinimal(context.Background(), minReq, []string{"appnexus", "rubicon", "pubmatic"})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if len(result.SelectedBidders) != 2 {
		t.Errorf("Expected 2 bidders, got %d", len(result.SelectedBidders))
	}

	if result.SelectedBidders[0].BidderCode != "appnexus" {
		t.Errorf("Expected first bidder appnexus, got %s", result.SelectedBidders[0].BidderCode)
	}
}

func TestSelectPartnersMinimal_CircuitOpen(t *testing.T) {
	// Create a server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cbConfig := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	client := NewClientWithCircuitBreaker(server.URL, 5*time.Second, "test-key", cbConfig)

	minReq := &MinimalRequest{
		ID: "test-123",
		Site: &MinimalSite{
			Domain: "example.com",
		},
		Imp: []MinimalImp{
			{ID: "imp-1"},
		},
	}

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		client.SelectPartnersMinimal(context.Background(), minReq, []string{"appnexus"})
	}

	// Circuit should be open now
	result, err := client.SelectPartnersMinimal(context.Background(), minReq, []string{"appnexus"})

	if err != nil {
		t.Errorf("Expected no error when circuit is open, got %v", err)
	}

	if result != nil {
		t.Error("Expected nil result when circuit is open")
	}
}

func TestBuildMinimalRequest_Site(t *testing.T) {
	impressions := []MinimalImp{
		{ID: "imp-1", MediaTypes: []string{"banner"}, Sizes: []string{"300x250"}},
		{ID: "imp-2", MediaTypes: []string{"video"}, Sizes: []string{"640x480"}},
	}

	req := BuildMinimalRequest(
		"req-123",
		"example.com",
		"pub-1",
		[]string{"news", "sports"},
		false, // isApp
		"",
		impressions,
		"US",
		"CA",
		"desktop",
	)

	if req.ID != "req-123" {
		t.Errorf("Expected ID req-123, got %s", req.ID)
	}

	if req.DeviceType != "desktop" {
		t.Errorf("Expected device type desktop, got %s", req.DeviceType)
	}

	if req.Site == nil {
		t.Fatal("Expected Site to be set")
	}

	if req.Site.Domain != "example.com" {
		t.Errorf("Expected domain example.com, got %s", req.Site.Domain)
	}

	if req.Site.Publisher != "pub-1" {
		t.Errorf("Expected publisher pub-1, got %s", req.Site.Publisher)
	}

	if len(req.Site.Categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(req.Site.Categories))
	}

	if req.App != nil {
		t.Error("Expected App to be nil for site request")
	}

	if req.Geo == nil {
		t.Fatal("Expected Geo to be set")
	}

	if req.Geo.Country != "US" {
		t.Errorf("Expected country US, got %s", req.Geo.Country)
	}

	if req.Geo.Region != "CA" {
		t.Errorf("Expected region CA, got %s", req.Geo.Region)
	}

	if len(req.Imp) != 2 {
		t.Errorf("Expected 2 impressions, got %d", len(req.Imp))
	}
}

func TestBuildMinimalRequest_App(t *testing.T) {
	impressions := []MinimalImp{
		{ID: "imp-1", MediaTypes: []string{"banner"}, Sizes: []string{"320x50"}},
	}

	req := BuildMinimalRequest(
		"req-456",
		"",
		"pub-2",
		[]string{"gaming"},
		true, // isApp
		"com.example.app",
		impressions,
		"GB",
		"",
		"mobile",
	)

	if req.App == nil {
		t.Fatal("Expected App to be set")
	}

	if req.App.Bundle != "com.example.app" {
		t.Errorf("Expected bundle com.example.app, got %s", req.App.Bundle)
	}

	if req.App.Publisher != "pub-2" {
		t.Errorf("Expected publisher pub-2, got %s", req.App.Publisher)
	}

	if len(req.App.Categories) != 1 {
		t.Errorf("Expected 1 category, got %d", len(req.App.Categories))
	}

	if req.Site != nil {
		t.Error("Expected Site to be nil for app request")
	}

	if req.Geo == nil {
		t.Fatal("Expected Geo to be set")
	}

	if req.Geo.Country != "GB" {
		t.Errorf("Expected country GB, got %s", req.Geo.Country)
	}

	if req.DeviceType != "mobile" {
		t.Errorf("Expected device type mobile, got %s", req.DeviceType)
	}
}

func TestBuildMinimalRequest_NoGeo(t *testing.T) {
	impressions := []MinimalImp{
		{ID: "imp-1"},
	}

	req := BuildMinimalRequest(
		"req-789",
		"example.com",
		"pub-1",
		[]string{},
		false,
		"",
		impressions,
		"", // no country
		"", // no region
		"desktop",
	)

	if req.Geo != nil {
		t.Error("Expected Geo to be nil when country and region are empty")
	}
}

func TestBuildMinimalImp(t *testing.T) {
	imp := BuildMinimalImp(
		"imp-123",
		[]string{"banner", "video"},
		[]string{"300x250", "640x480"},
	)

	if imp.ID != "imp-123" {
		t.Errorf("Expected ID imp-123, got %s", imp.ID)
	}

	if len(imp.MediaTypes) != 2 {
		t.Errorf("Expected 2 media types, got %d", len(imp.MediaTypes))
	}

	if imp.MediaTypes[0] != "banner" {
		t.Errorf("Expected first media type banner, got %s", imp.MediaTypes[0])
	}

	if len(imp.Sizes) != 2 {
		t.Errorf("Expected 2 sizes, got %d", len(imp.Sizes))
	}

	if imp.Sizes[0] != "300x250" {
		t.Errorf("Expected first size 300x250, got %s", imp.Sizes[0])
	}
}
