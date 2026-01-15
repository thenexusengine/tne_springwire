package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestPublisherAuth_Disabled(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled: false,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should have been called when auth is disabled")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestPublisherAuth_NonAuctionEndpoint(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled: true,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Non-auction endpoint should pass through
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should have been called for non-auction endpoints")
	}
}

func TestPublisherAuth_MissingPublisher(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: false,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request without publisher
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("Handler should NOT have been called without publisher when AllowUnregistered=false")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rr.Code)
	}
}

func TestPublisherAuth_AllowUnregistered(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: true,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request without publisher
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should have been called when AllowUnregistered=true")
	}
}

func TestPublisherAuth_RegisteredPublisher(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: true, // Allow since no DB configured
		RegisteredPubs:    map[string]string{"pub123": "example.com"},
		ValidateDomain:    false, // Don't validate domain for this test
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request with registered publisher
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
		"site": map[string]interface{}{
			"domain": "example.com",
			"publisher": map[string]interface{}{
				"id": "pub123",
			},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should have been called for registered publisher")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestPublisherAuth_UnregisteredPublisher(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: false,
		RegisteredPubs:    map[string]string{"pub123": "example.com"},
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request with unregistered publisher
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
		"site": map[string]interface{}{
			"domain": "example.com",
			"publisher": map[string]interface{}{
				"id": "unknown_pub", // Not in RegisteredPubs
			},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("Handler should NOT have been called for unregistered publisher")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rr.Code)
	}
}

func TestPublisherAuth_DomainValidation(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: false,
		RegisteredPubs:    map[string]string{"pub123": "allowed.com"},
		ValidateDomain:    true,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request with wrong domain
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
		"site": map[string]interface{}{
			"domain": "wrong.com", // Not in allowed domains
			"publisher": map[string]interface{}{
				"id": "pub123",
			},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("Handler should NOT have been called for wrong domain")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rr.Code)
	}
}

func TestPublisherAuth_DomainValidation_Allowed(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: true, // Allow since no DB configured
		RegisteredPubs:    map[string]string{"pub123": "allowed.com"},
		ValidateDomain:    true,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request with correct domain
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
		"site": map[string]interface{}{
			"domain": "allowed.com",
			"publisher": map[string]interface{}{
				"id": "pub123",
			},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should have been called for allowed domain")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestPublisherAuth_AppPublisher(t *testing.T) {
	config := &PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: true,                             // Allow since no DB configured
		RegisteredPubs:    map[string]string{"app_pub": ""}, // Deprecated but kept for backward compat
		ValidateDomain:    false,
	}
	auth := NewPublisherAuth(config)

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request with app publisher
	bidReq := map[string]interface{}{
		"id": "test-1",
		"imp": []map[string]interface{}{
			{"id": "imp1", "banner": map[string]interface{}{}},
		},
		"app": map[string]interface{}{
			"bundle": "com.example.app",
			"publisher": map[string]interface{}{
				"id": "app_pub",
			},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should have been called for registered app publisher")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestParsePublishers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:     "single with domain",
			input:    "pub1:example.com",
			expected: map[string]string{"pub1": "example.com"},
		},
		{
			name:     "single without domain",
			input:    "pub1",
			expected: map[string]string{"pub1": ""},
		},
		{
			name:  "multiple",
			input: "pub1:example.com,pub2:other.com",
			expected: map[string]string{
				"pub1": "example.com",
				"pub2": "other.com",
			},
		},
		{
			name:  "mixed with and without domains",
			input: "pub1:example.com,pub2,pub3:test.com",
			expected: map[string]string{
				"pub1": "example.com",
				"pub2": "",
				"pub3": "test.com",
			},
		},
		{
			name:  "with spaces",
			input: " pub1 : example.com , pub2 : other.com ",
			expected: map[string]string{
				"pub1": "example.com",
				"pub2": "other.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePublishers(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parsePublishers(%q) returned %d items, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("parsePublishers(%q)[%s] = %q, want %q", tt.input, k, result[k], v)
				}
			}
		})
	}
}

func TestDefaultPublisherAuthConfig(t *testing.T) {
	// Clear env vars
	os.Unsetenv("PUBLISHER_AUTH_ENABLED")
	os.Unsetenv("AUTH_ENABLED")
	os.Unsetenv("PUBLISHER_ALLOW_UNREGISTERED")
	os.Unsetenv("REGISTERED_PUBLISHERS")
	os.Unsetenv("PUBLISHER_VALIDATE_DOMAIN")
	os.Unsetenv("PUBLISHER_AUTH_USE_REDIS")

	config := DefaultPublisherAuthConfig()

	if !config.Enabled {
		t.Error("Expected auth to be enabled by default")
	}

	if config.RateLimitPerPub != 100 {
		t.Errorf("Expected default rate limit 100, got %d", config.RateLimitPerPub)
	}

	if !config.UseRedis {
		t.Error("Expected Redis to be enabled by default")
	}
}

func TestDefaultPublisherAuthConfig_Disabled(t *testing.T) {
	os.Setenv("PUBLISHER_AUTH_ENABLED", "false")
	defer os.Unsetenv("PUBLISHER_AUTH_ENABLED")

	config := DefaultPublisherAuthConfig()

	if config.Enabled {
		t.Error("Expected auth to be disabled when PUBLISHER_AUTH_ENABLED=false")
	}
}

func TestDefaultPublisherAuthConfig_DevMode(t *testing.T) {
	os.Setenv("AUTH_ENABLED", "false")
	defer os.Unsetenv("AUTH_ENABLED")

	config := DefaultPublisherAuthConfig()

	if !config.AllowUnregistered {
		t.Error("Expected AllowUnregistered=true in dev mode")
	}
}

func TestDefaultPublisherAuthConfig_AllowUnregistered(t *testing.T) {
	os.Setenv("PUBLISHER_ALLOW_UNREGISTERED", "true")
	defer os.Unsetenv("PUBLISHER_ALLOW_UNREGISTERED")

	config := DefaultPublisherAuthConfig()

	if !config.AllowUnregistered {
		t.Error("Expected AllowUnregistered=true when set")
	}
}

func TestDefaultPublisherAuthConfig_ValidateDomain(t *testing.T) {
	os.Setenv("PUBLISHER_VALIDATE_DOMAIN", "true")
	defer os.Unsetenv("PUBLISHER_VALIDATE_DOMAIN")

	config := DefaultPublisherAuthConfig()

	if !config.ValidateDomain {
		t.Error("Expected ValidateDomain=true when set")
	}
}

func TestDefaultPublisherAuthConfig_RegisteredPublishers(t *testing.T) {
	os.Setenv("REGISTERED_PUBLISHERS", "pub1:example.com,pub2:test.com")
	defer os.Unsetenv("REGISTERED_PUBLISHERS")

	config := DefaultPublisherAuthConfig()

	if len(config.RegisteredPubs) != 2 {
		t.Errorf("Expected 2 registered publishers, got %d", len(config.RegisteredPubs))
	}

	if config.RegisteredPubs["pub1"] != "example.com" {
		t.Error("Expected pub1 to have domain example.com")
	}
}

func TestNewPublisherAuth_NilConfig(t *testing.T) {
	auth := NewPublisherAuth(nil)

	if auth == nil {
		t.Fatal("Expected auth instance to be created")
	}

	if auth.config == nil {
		t.Error("Expected default config to be set")
	}

	if auth.rateLimits == nil {
		t.Error("Expected rate limits map to be initialized")
	}
}

func TestSetRedisClient(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled: true,
	})

	mockRedis := &mockRedisClient{}
	auth.SetRedisClient(mockRedis)

	if auth.redisClient == nil {
		t.Error("Expected Redis client to be set")
	}
}

func TestRegisterPublisher(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:        true,
		RegisteredPubs: map[string]string{},
	})

	auth.RegisterPublisher("pub123", "example.com")

	if auth.config.RegisteredPubs["pub123"] != "example.com" {
		t.Error("Expected publisher to be registered")
	}
}

func TestRegisterPublisher_NilMap(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:        true,
		RegisteredPubs: nil, // nil map
	})

	auth.RegisterPublisher("pub123", "example.com")

	if auth.config.RegisteredPubs == nil {
		t.Error("Expected map to be initialized")
	}

	if auth.config.RegisteredPubs["pub123"] != "example.com" {
		t.Error("Expected publisher to be registered")
	}
}

func TestUnregisterPublisher(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled: true,
		RegisteredPubs: map[string]string{
			"pub123": "example.com",
		},
	})

	auth.UnregisterPublisher("pub123")

	if _, exists := auth.config.RegisteredPubs["pub123"]; exists {
		t.Error("Expected publisher to be unregistered")
	}
}

func TestSetEnabled_PublisherAuth(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled: true,
	})

	auth.SetEnabled(false)

	if auth.config.Enabled {
		t.Error("Expected auth to be disabled")
	}

	auth.SetEnabled(true)

	if !auth.config.Enabled {
		t.Error("Expected auth to be enabled")
	}
}

func TestCheckRateLimit_Unlimited(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:         true,
		RateLimitPerPub: 0, // Unlimited
	})

	// Should always allow
	for i := 0; i < 1000; i++ {
		if !auth.checkRateLimit("pub123") {
			t.Error("Expected unlimited rate limit to always allow")
		}
	}
}

func TestCheckRateLimit_FirstRequest(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:         true,
		RateLimitPerPub: 10,
	})

	if !auth.checkRateLimit("pub123") {
		t.Error("Expected first request to be allowed")
	}
}

func TestCheckRateLimit_Exhaustion(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:         true,
		RateLimitPerPub: 5,
	})

	// Use up all tokens
	for i := 0; i < 5; i++ {
		if !auth.checkRateLimit("pub123") {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Next request should be denied
	if auth.checkRateLimit("pub123") {
		t.Error("Expected rate limit to be exceeded")
	}
}

func TestCheckRateLimit_TokenRefill(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:         true,
		RateLimitPerPub: 10,
	})

	// Use up all tokens
	for i := 0; i < 10; i++ {
		auth.checkRateLimit("pub123")
	}

	// Wait for tokens to refill (0.2 seconds = 2 tokens at 10 RPS)
	time.Sleep(200 * time.Millisecond)

	// Should allow 2 more requests
	if !auth.checkRateLimit("pub123") {
		t.Error("Expected token refill to allow request")
	}
	if !auth.checkRateLimit("pub123") {
		t.Error("Expected second refilled token to allow request")
	}
}

func TestCheckRateLimit_DifferentPublishers(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:         true,
		RateLimitPerPub: 2,
	})

	// Exhaust pub1
	auth.checkRateLimit("pub1")
	auth.checkRateLimit("pub1")

	// pub2 should still work
	if !auth.checkRateLimit("pub2") {
		t.Error("Expected different publishers to have independent rate limits")
	}
}

func TestDomainMatches_Exact(t *testing.T) {
	auth := NewPublisherAuth(nil)

	if !auth.domainMatches("example.com", "example.com") {
		t.Error("Expected exact domain match")
	}
}

func TestDomainMatches_Wildcard(t *testing.T) {
	auth := NewPublisherAuth(nil)

	testCases := []struct {
		domain  string
		allowed string
		match   bool
	}{
		{"sub.example.com", "*.example.com", true},
		{"deep.sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true}, // Base domain also matches
		{"wrongexample.com", "*.example.com", false},
		{"example.com.evil.com", "*.example.com", false},
	}

	for _, tc := range testCases {
		result := auth.domainMatches(tc.domain, tc.allowed)
		if result != tc.match {
			t.Errorf("domainMatches(%q, %q) = %v, expected %v",
				tc.domain, tc.allowed, result, tc.match)
		}
	}
}

func TestDomainMatches_MultipleAllowed(t *testing.T) {
	auth := NewPublisherAuth(nil)

	allowed := "example.com|test.com|demo.com"

	if !auth.domainMatches("example.com", allowed) {
		t.Error("Expected example.com to match")
	}
	if !auth.domainMatches("test.com", allowed) {
		t.Error("Expected test.com to match")
	}
	if !auth.domainMatches("demo.com", allowed) {
		t.Error("Expected demo.com to match")
	}
	if auth.domainMatches("other.com", allowed) {
		t.Error("Expected other.com NOT to match")
	}
}

func TestDomainMatches_EmptyDomain(t *testing.T) {
	auth := NewPublisherAuth(nil)

	if auth.domainMatches("", "example.com") {
		t.Error("Expected empty domain to never match")
	}
}

func TestDomainMatches_EmptyAllowed(t *testing.T) {
	auth := NewPublisherAuth(nil)

	if auth.domainMatches("example.com", "") {
		t.Error("Expected empty allowed to never match")
	}
}

func TestDomainMatches_Whitespace(t *testing.T) {
	auth := NewPublisherAuth(nil)

	allowed := " example.com | test.com "

	if !auth.domainMatches("example.com", allowed) {
		t.Error("Expected whitespace to be trimmed")
	}
}

func TestPublisherAuthError_Error(t *testing.T) {
	err := &PublisherAuthError{
		Code:    "test_error",
		Message: "Test message",
	}

	expected := "test_error: Test message"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestPublisherAuthError_ErrorNoCode(t *testing.T) {
	err := &PublisherAuthError{
		Message: "Test message",
	}

	if err.Error() != "Test message" {
		t.Errorf("Expected message only, got %q", err.Error())
	}
}

func TestPublisherAuthError_Unwrap(t *testing.T) {
	cause := &PublisherAuthError{Message: "cause"}
	err := &PublisherAuthError{
		Code:    "wrapper",
		Message: "Wrapper message",
		Cause:   cause,
	}

	if !errors.Is(err.Unwrap(), cause) {
		t.Error("Expected Unwrap to return cause")
	}
}

func TestPublisherAuthError_UnwrapNil(t *testing.T) {
	err := &PublisherAuthError{
		Code:    "test",
		Message: "No cause",
	}

	if err.Unwrap() != nil {
		t.Error("Expected Unwrap to return nil when no cause")
	}
}

func TestMiddleware_InvalidJSON(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled: true,
	})

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Invalid JSON should pass through to main handler
	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction",
		bytes.NewReader([]byte("invalid json{")))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Handler should be called for invalid JSON (let main handler deal with it)")
	}
}

func TestMiddleware_RateLimitExceeded(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: true,
		RateLimitPerPub:   2,
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	bidReq := map[string]interface{}{
		"id": "test-1",
		"site": map[string]interface{}{
			"domain": "example.com",
			"publisher": map[string]interface{}{
				"id": "pub123",
			},
		},
		"imp": []map[string]interface{}{
			{"id": "imp1"},
		},
	}
	body, _ := json.Marshal(bidReq)

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d should succeed, got %d", i, rr.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429, got %d", rr.Code)
	}
}

func TestMiddleware_PublisherIDInHeader(t *testing.T) {
	auth := NewPublisherAuth(&PublisherAuthConfig{
		Enabled:           true,
		AllowUnregistered: true,
	})

	var capturedHeader string
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Publisher-ID")
		w.WriteHeader(http.StatusOK)
	}))

	bidReq := map[string]interface{}{
		"id": "test-1",
		"site": map[string]interface{}{
			"publisher": map[string]interface{}{
				"id": "pub123",
			},
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest(http.MethodPost, "/openrtb2/auction", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedHeader != "pub123" {
		t.Errorf("Expected X-Publisher-ID header to be 'pub123', got %q", capturedHeader)
	}
}

// Mock Redis client for testing
type mockRedisClient struct {
	data map[string]map[string]string
}

func (m *mockRedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	if m.data == nil {
		return "", nil
	}
	if hash, ok := m.data[key]; ok {
		return hash[field], nil
	}
	return "", nil
}

func (m *mockRedisClient) Ping(ctx context.Context) error {
	return nil
}

// REMOVED: Redis validation tests
// Redis publisher validation was removed in favor of PostgreSQL-only architecture.
// These tests are deprecated. Publisher validation now requires a properly configured
// publisherStore (PostgreSQL). See commit 99b688c for migration details.
