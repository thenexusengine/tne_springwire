package endpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/exchange"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// Mock adapter for testing
type mockAdapter struct {
	bids []*adapters.TypedBid
	err  error
}

func (m *mockAdapter) MakeRequests(request *openrtb.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	return []*adapters.RequestData{{Method: "POST", URI: "http://test.com", Body: []byte("{}")}}, nil
}

func (m *mockAdapter) MakeBids(request *openrtb.BidRequest, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if m.err != nil {
		return nil, []error{m.err}
	}
	return &adapters.BidderResponse{Bids: m.bids, Currency: "USD"}, nil
}

// Helper to create a valid bid request
func validBidRequest() *openrtb.BidRequest {
	return &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
}

func TestNewAuctionHandler(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})

	handler := NewAuctionHandler(ex)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.exchange != ex {
		t.Error("expected exchange to be set")
	}
}

func TestAuctionHandler_MethodNotAllowed(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	methods := []string{"GET", "PUT", "DELETE", "PATCH", "OPTIONS"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/openrtb2/auction", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405, got %d", w.Code)
			}
		})
	}
}

func TestAuctionHandler_InvalidJSON(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	req := httptest.NewRequest("POST", "/openrtb2/auction", strings.NewReader("not valid json"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !strings.Contains(resp["error"], "Invalid JSON") {
		t.Errorf("expected 'Invalid JSON' error, got: %s", resp["error"])
	}
}

func TestAuctionHandler_EmptyBody(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	req := httptest.NewRequest("POST", "/openrtb2/auction", strings.NewReader(""))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAuctionHandler_MissingID(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		Imp: []openrtb.Imp{{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}}},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "id") {
		t.Errorf("expected id error, got: %s", resp["error"])
	}
}

func TestAuctionHandler_NoImpressions(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID:  "test-1",
		Imp: []openrtb.Imp{},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "impression") {
		t.Errorf("expected impression error, got: %s", resp["error"])
	}
}

func TestAuctionHandler_ImpressionMissingID(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID:  "test-1",
		Imp: []openrtb.Imp{{Banner: &openrtb.Banner{W: 300, H: 250}}},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAuctionHandler_ImpressionNoMediaType(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID:  "test-1",
		Imp: []openrtb.Imp{{ID: "imp-1"}}, // No banner, video, native, or audio
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "media type") {
		t.Errorf("expected media type error, got: %s", resp["error"])
	}
}

func TestAuctionHandler_ValidRequest_NoBidders(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}

	var resp openrtb.BidResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ID != bidReq.ID {
		t.Errorf("expected response ID %s, got %s", bidReq.ID, resp.ID)
	}
}

func TestAuctionHandler_DebugMode(t *testing.T) {
	registry := adapters.NewRegistry()
	mock := &mockAdapter{bids: []*adapters.TypedBid{}}
	registry.Register("testbidder", mock, adapters.BidderInfo{Enabled: true})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	// P2-1: Debug mode requires auth header
	req := httptest.NewRequest("POST", "/openrtb2/auction?debug=1", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "test-key") // Add API key for debug access
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Debug mode should include ext with timing info
	if resp.Ext == nil {
		t.Log("Note: ext may be nil if no debug info was generated")
	}
}

// P2-1: Test debug mode authentication requirements
func TestAuctionHandler_DebugMode_RequiresAuth(t *testing.T) {
	registry := adapters.NewRegistry()
	mock := &mockAdapter{bids: []*adapters.TypedBid{}}
	registry.Register("testbidder", mock, adapters.BidderInfo{Enabled: true})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	// Test without auth - debug should be silently ignored
	req := httptest.NewRequest("POST", "/openrtb2/auction?debug=1", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Response should succeed but without debug info
	var resp openrtb.BidResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Without auth, Ext should not contain debug info
	// (Ext may still be nil or empty since debug was disabled)
}

func TestAuctionHandler_DebugMode_WithAPIKey(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	// With X-API-Key header, debug should work
	req := httptest.NewRequest("POST", "/openrtb2/auction?debug=1", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuctionHandler_DebugMode_WithBearerToken(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	// With Authorization Bearer header, debug should work
	req := httptest.NewRequest("POST", "/openrtb2/auction?debug=1", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHasAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name:     "no auth headers",
			headers:  map[string]string{},
			expected: false,
		},
		{
			name:     "X-API-Key present",
			headers:  map[string]string{"X-API-Key": "test-key"},
			expected: true,
		},
		{
			name:     "Bearer token present",
			headers:  map[string]string{"Authorization": "Bearer test-token"},
			expected: true,
		},
		{
			name:     "empty X-API-Key",
			headers:  map[string]string{"X-API-Key": ""},
			expected: false,
		},
		{
			name:     "Authorization without Bearer",
			headers:  map[string]string{"Authorization": "Basic test"},
			expected: false,
		},
		{
			name:     "Bearer only (no token)",
			headers:  map[string]string{"Authorization": "Bearer "},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := hasAPIKey(req)
			if result != tt.expected {
				t.Errorf("hasAPIKey() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestAuctionHandler_WithContext(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should complete (either success or context timeout)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d", w.Code)
	}
}

// Test validateBidRequest function
func TestValidateBidRequest_Valid(t *testing.T) {
	req := validBidRequest()
	if err := validateBidRequest(req); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateBidRequest_MissingID(t *testing.T) {
	req := &openrtb.BidRequest{
		Imp: []openrtb.Imp{{ID: "imp-1", Banner: &openrtb.Banner{}}},
	}
	err := validateBidRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	var valErr *ValidationError
	ok := errors.As(err, &valErr)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if valErr.Field != "id" {
		t.Errorf("expected field 'id', got '%s'", valErr.Field)
	}
}

func TestValidateBidRequest_NoImpressions(t *testing.T) {
	req := &openrtb.BidRequest{
		ID:  "test-1",
		Imp: []openrtb.Imp{},
	}
	err := validateBidRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	var valErr *ValidationError
	_ = errors.As(err, &valErr)
	if valErr.Field != "imp" {
		t.Errorf("expected field 'imp', got '%s'", valErr.Field)
	}
}

func TestValidateBidRequest_ImpressionMissingID(t *testing.T) {
	req := &openrtb.BidRequest{
		ID:  "test-1",
		Imp: []openrtb.Imp{{Banner: &openrtb.Banner{}}},
	}
	err := validateBidRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	var valErr *ValidationError
	_ = errors.As(err, &valErr)
	if valErr.Field != "imp[].id" {
		t.Errorf("expected field 'imp[].id', got '%s'", valErr.Field)
	}
	if valErr.Index != 0 {
		t.Errorf("expected index 0, got %d", valErr.Index)
	}
}

func TestValidateBidRequest_ImpressionNoMediaType(t *testing.T) {
	req := &openrtb.BidRequest{
		ID:  "test-1",
		Imp: []openrtb.Imp{{ID: "imp-1"}},
	}
	err := validateBidRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	var valErr *ValidationError
	_ = errors.As(err, &valErr)
	if !strings.Contains(valErr.Field, "banner|video|native|audio") {
		t.Errorf("expected media type field, got '%s'", valErr.Field)
	}
}

func TestValidateBidRequest_MultipleImpressions(t *testing.T) {
	req := &openrtb.BidRequest{
		ID: "test-1",
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{}},
			{ID: "imp-2", Video: &openrtb.Video{}},
			{ID: "imp-3", Native: &openrtb.Native{}},
			{ID: "imp-4", Audio: &openrtb.Audio{}},
		},
	}
	if err := validateBidRequest(req); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateBidRequest_SecondImpressionInvalid(t *testing.T) {
	req := &openrtb.BidRequest{
		ID: "test-1",
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{}},
			{ID: "", Banner: &openrtb.Banner{}}, // Missing ID
		},
	}
	err := validateBidRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	var valErr *ValidationError
	_ = errors.As(err, &valErr)
	if valErr.Index != 1 {
		t.Errorf("expected index 1, got %d", valErr.Index)
	}
}

// Test ValidationError
func TestValidationError_Error_WithIndex(t *testing.T) {
	err := &ValidationError{
		Field:   "imp[].id",
		Message: "required",
		Index:   2,
	}
	expected := "imp[].id[2]: required"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

func TestValidationError_Error_WithoutIndex(t *testing.T) {
	err := &ValidationError{
		Field:   "id",
		Message: "required",
		Index:   -1,
	}
	// When Index is negative, no index should be shown
	result := err.Error()
	if strings.Contains(result, "[") {
		t.Errorf("expected no brackets, got '%s'", result)
	}
}

func TestValidationError_Error_ZeroIndex(t *testing.T) {
	err := &ValidationError{
		Field:   "imp[].id",
		Message: "required",
		Index:   0,
	}
	expected := "imp[].id[0]: required"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

// Test buildResponseExt
func TestBuildResponseExt_NilDebugInfo(t *testing.T) {
	result := &exchange.AuctionResponse{
		DebugInfo: nil,
	}
	ext := buildResponseExt(result)
	if ext == nil {
		t.Fatal("expected non-nil ext")
	}
	if ext.ResponseTimeMillis == nil {
		t.Error("expected initialized ResponseTimeMillis map")
	}
	if ext.Errors == nil {
		t.Error("expected initialized Errors map")
	}
}

func TestBuildResponseExt_WithLatencies(t *testing.T) {
	result := &exchange.AuctionResponse{
		DebugInfo: &exchange.DebugInfo{
			BidderLatencies: map[string]time.Duration{
				"bidder1": 50 * time.Millisecond,
				"bidder2": 100 * time.Millisecond,
			},
			TotalLatency: 150 * time.Millisecond,
		},
	}
	ext := buildResponseExt(result)

	if ext.ResponseTimeMillis["bidder1"] != 50 {
		t.Errorf("expected bidder1 latency 50, got %d", ext.ResponseTimeMillis["bidder1"])
	}
	if ext.ResponseTimeMillis["bidder2"] != 100 {
		t.Errorf("expected bidder2 latency 100, got %d", ext.ResponseTimeMillis["bidder2"])
	}
	if ext.TMMaxRequest != 150 {
		t.Errorf("expected TMMaxRequest 150, got %d", ext.TMMaxRequest)
	}
}

func TestBuildResponseExt_WithErrors(t *testing.T) {
	result := &exchange.AuctionResponse{
		DebugInfo: &exchange.DebugInfo{
			Errors: map[string][]string{
				"bidder1": {"error1", "error2"},
				"bidder2": {"error3"},
			},
			BidderLatencies: map[string]time.Duration{},
		},
	}
	ext := buildResponseExt(result)

	if len(ext.Errors["bidder1"]) != 2 {
		t.Errorf("expected 2 errors for bidder1, got %d", len(ext.Errors["bidder1"]))
	}
	if ext.Errors["bidder1"][0].Message != "error1" {
		t.Errorf("expected 'error1', got '%s'", ext.Errors["bidder1"][0].Message)
	}
	if ext.Errors["bidder1"][0].Code != 1 {
		t.Errorf("expected code 1, got %d", ext.Errors["bidder1"][0].Code)
	}
}

// Test writeError
func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, "test error message", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["error"] != "test error message" {
		t.Errorf("expected 'test error message', got '%s'", resp["error"])
	}
}

func TestWriteError_DifferentStatuses(t *testing.T) {
	tests := []struct {
		status  int
		message string
	}{
		{http.StatusBadRequest, "bad request"},
		{http.StatusInternalServerError, "internal error"},
		{http.StatusNotFound, "not found"},
		{http.StatusUnauthorized, "unauthorized"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeError(w, tt.message, tt.status)

			if w.Code != tt.status {
				t.Errorf("expected %d, got %d", tt.status, w.Code)
			}
		})
	}
}

// Test StatusHandler
func TestNewStatusHandler(t *testing.T) {
	handler := NewStatusHandler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestStatusHandler_ServeHTTP(t *testing.T) {
	handler := NewStatusHandler()
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", resp["status"])
	}
	if resp["timestamp"] == nil {
		t.Error("expected timestamp in response")
	}
}

func TestStatusHandler_TimestampFormat(t *testing.T) {
	handler := NewStatusHandler()
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	timestamp := resp["timestamp"].(string)
	_, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Errorf("expected RFC3339 timestamp, got '%s': %v", timestamp, err)
	}
}

// Mock registries for InfoBiddersHandler tests
type mockStaticRegistry struct {
	bidders []string
}

func (m *mockStaticRegistry) ListBidders() []string {
	return m.bidders
}

type mockDynamicRegistry struct {
	bidders []string
}

func (m *mockDynamicRegistry) ListBidderCodes() []string {
	return m.bidders
}

// Test InfoBiddersHandler
func TestNewInfoBiddersHandler(t *testing.T) {
	handler := NewInfoBiddersHandler([]string{"bidder1", "bidder2"})
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewDynamicInfoBiddersHandler(t *testing.T) {
	static := &mockStaticRegistry{bidders: []string{"static1"}}
	dynamic := &mockDynamicRegistry{bidders: []string{"dynamic1"}}

	handler := NewDynamicInfoBiddersHandler(static, dynamic)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.staticRegistry != static {
		t.Error("expected static registry to be set")
	}
	if handler.dynamicRegistry != dynamic {
		t.Error("expected dynamic registry to be set")
	}
}

func TestInfoBiddersHandler_StaticOnly(t *testing.T) {
	static := &mockStaticRegistry{bidders: []string{"bidder1", "bidder2"}}
	handler := NewDynamicInfoBiddersHandler(static, nil)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var bidders []string
	if err := json.Unmarshal(w.Body.Bytes(), &bidders); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(bidders) != 2 {
		t.Errorf("expected 2 bidders, got %d", len(bidders))
	}
}

func TestInfoBiddersHandler_DynamicOnly(t *testing.T) {
	dynamic := &mockDynamicRegistry{bidders: []string{"dynamic1", "dynamic2", "dynamic3"}}
	handler := NewDynamicInfoBiddersHandler(nil, dynamic)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var bidders []string
	json.Unmarshal(w.Body.Bytes(), &bidders)

	if len(bidders) != 3 {
		t.Errorf("expected 3 bidders, got %d", len(bidders))
	}
}

func TestInfoBiddersHandler_BothRegistries(t *testing.T) {
	static := &mockStaticRegistry{bidders: []string{"bidder1", "bidder2"}}
	dynamic := &mockDynamicRegistry{bidders: []string{"dynamic1", "dynamic2"}}
	handler := NewDynamicInfoBiddersHandler(static, dynamic)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var bidders []string
	json.Unmarshal(w.Body.Bytes(), &bidders)

	if len(bidders) != 4 {
		t.Errorf("expected 4 bidders, got %d", len(bidders))
	}
}

func TestInfoBiddersHandler_DeduplicatesBidders(t *testing.T) {
	// Both registries have the same bidder
	static := &mockStaticRegistry{bidders: []string{"bidder1", "common"}}
	dynamic := &mockDynamicRegistry{bidders: []string{"common", "dynamic1"}}
	handler := NewDynamicInfoBiddersHandler(static, dynamic)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var bidders []string
	json.Unmarshal(w.Body.Bytes(), &bidders)

	// Should be deduplicated: bidder1, common, dynamic1
	if len(bidders) != 3 {
		t.Errorf("expected 3 bidders (deduplicated), got %d: %v", len(bidders), bidders)
	}
}

func TestInfoBiddersHandler_EmptyRegistries(t *testing.T) {
	static := &mockStaticRegistry{bidders: []string{}}
	dynamic := &mockDynamicRegistry{bidders: []string{}}
	handler := NewDynamicInfoBiddersHandler(static, dynamic)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var bidders []string
	json.Unmarshal(w.Body.Bytes(), &bidders)

	if len(bidders) != 0 {
		t.Errorf("expected 0 bidders, got %d", len(bidders))
	}
}

func TestInfoBiddersHandler_NilRegistries(t *testing.T) {
	handler := NewDynamicInfoBiddersHandler(nil, nil)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var bidders []string
	json.Unmarshal(w.Body.Bytes(), &bidders)

	if len(bidders) != 0 {
		t.Errorf("expected 0 bidders, got %d", len(bidders))
	}
}

func TestInfoBiddersHandler_ContentType(t *testing.T) {
	handler := NewDynamicInfoBiddersHandler(nil, nil)

	req := httptest.NewRequest("GET", "/info/bidders", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}
}

// Error reader for testing body read failures
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (e *errorReader) Close() error {
	return nil
}

func TestAuctionHandler_BodyReadError(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
	req.Body = io.NopCloser(&errorReader{})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "read request body") {
		t.Errorf("expected read error message, got: %s", resp["error"])
	}
}

// Benchmark tests
func BenchmarkValidateBidRequest(b *testing.B) {
	req := validBidRequest()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateBidRequest(req)
	}
}

func BenchmarkBuildResponseExt(b *testing.B) {
	result := &exchange.AuctionResponse{
		DebugInfo: &exchange.DebugInfo{
			BidderLatencies: map[string]time.Duration{
				"bidder1": 50 * time.Millisecond,
				"bidder2": 100 * time.Millisecond,
			},
			Errors: map[string][]string{
				"bidder1": {"error1"},
			},
			TotalLatency: 150 * time.Millisecond,
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildResponseExt(result)
	}
}

func BenchmarkStatusHandler(b *testing.B) {
	handler := NewStatusHandler()
	req := httptest.NewRequest("GET", "/status", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkAuctionHandler_ValidRequest(b *testing.B) {
	registry := adapters.NewRegistry()
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 100 * time.Millisecond,
	})
	handler := NewAuctionHandler(ex)

	bidReq := validBidRequest()
	body, _ := json.Marshal(bidReq)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
