package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSecurityMiddleware_AllHeaders(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:                 true,
		XFrameOptions:           "DENY",
		XContentTypeOptions:     "nosniff",
		XXSSProtection:          "1; mode=block",
		ContentSecurityPolicy:   "default-src 'none'",
		ReferrerPolicy:          "strict-origin-when-cross-origin",
		StrictTransportSecurity: "max-age=31536000",
		PermissionsPolicy:       "geolocation=()",
		CacheControl:            "no-store",
	})

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/openrtb2/auction", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Content-Security-Policy", "default-src 'none'"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Strict-Transport-Security", "max-age=31536000"},
		{"Permissions-Policy", "geolocation=()"},
		{"Cache-Control", "no-store"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			if got := rr.Header().Get(tt.header); got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.header, got, tt.expected)
			}
		})
	}
}

func TestSecurityMiddleware_Disabled(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:             false,
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
	})

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/openrtb2/auction", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// No security headers when disabled
	if got := rr.Header().Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options should be empty when disabled, got %q", got)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "" {
		t.Errorf("X-Content-Type-Options should be empty when disabled, got %q", got)
	}
}

func TestSecurityMiddleware_DefaultConfig(t *testing.T) {
	security := NewSecurity(nil) // Use defaults

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/openrtb2/auction", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Check essential headers are present with defaults
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rr.Header().Get("X-XSS-Protection"); got != "1; mode=block" {
		t.Errorf("X-XSS-Protection = %q, want '1; mode=block'", got)
	}
}

func TestSecurityMiddleware_MetricsPathNoCacheControl(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:      true,
		CacheControl: "no-store",
	})

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Metrics path should not have Cache-Control
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control should be empty for /metrics, got %q", got)
	}

	// API path should have Cache-Control
	req2 := httptest.NewRequest("GET", "/openrtb2/auction", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if got := rr2.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestSecurityMiddleware_SetHSTS(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled: true,
	})

	// Initially no HSTS
	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should be empty initially, got %q", got)
	}

	// Enable HSTS
	security.SetHSTS("max-age=31536000; includeSubDomains")

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)

	if got := rr2.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS = %q, want 'max-age=31536000; includeSubDomains'", got)
	}
}

func TestSecurityMiddleware_EmptyHeadersSkipped(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:             true,
		XFrameOptions:       "", // Empty - should not be set
		XContentTypeOptions: "nosniff",
	})

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Empty config values should not set headers
	if got := rr.Header().Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options should not be set when empty, got %q", got)
	}

	// Non-empty values should be set
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

// Additional tests for full coverage

func TestEnvOrDefault_WithValue(t *testing.T) {
	os.Setenv("TEST_VAR", "custom_value")
	defer os.Unsetenv("TEST_VAR")

	result := envOrDefault("TEST_VAR", "default")

	if result != "custom_value" {
		t.Errorf("Expected custom_value, got %s", result)
	}
}

func TestEnvOrDefault_WithoutValue(t *testing.T) {
	os.Unsetenv("TEST_VAR")

	result := envOrDefault("TEST_VAR", "default")

	if result != "default" {
		t.Errorf("Expected default, got %s", result)
	}
}

func TestEnvOrDefault_EmptyString(t *testing.T) {
	os.Setenv("TEST_VAR", "")
	defer os.Unsetenv("TEST_VAR")

	result := envOrDefault("TEST_VAR", "default")

	// Empty string should return default
	if result != "default" {
		t.Errorf("Expected default for empty string, got %s", result)
	}
}

func TestSetEnabled_Security(t *testing.T) {
	security := NewSecurity(&SecurityConfig{Enabled: true})

	security.SetEnabled(false)

	if security.config.Enabled {
		t.Error("Expected security to be disabled")
	}

	security.SetEnabled(true)

	if !security.config.Enabled {
		t.Error("Expected security to be enabled")
	}
}

func TestSetCSP(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:               true,
		ContentSecurityPolicy: "default-src 'self'",
	})

	newCSP := "default-src 'none'; script-src 'self'"
	security.SetCSP(newCSP)

	if security.config.ContentSecurityPolicy != newCSP {
		t.Errorf("Expected CSP to be %s, got %s", newCSP, security.config.ContentSecurityPolicy)
	}
}

func TestGetConfig(t *testing.T) {
	originalConfig := &SecurityConfig{
		Enabled:                 true,
		XFrameOptions:           "DENY",
		ContentSecurityPolicy:   "default-src 'self'",
		StrictTransportSecurity: "max-age=31536000",
	}

	security := NewSecurity(originalConfig)

	// Get a copy of the config
	config := security.GetConfig()

	// Verify all fields match
	if config.Enabled != originalConfig.Enabled {
		t.Error("Expected Enabled to match")
	}

	if config.XFrameOptions != originalConfig.XFrameOptions {
		t.Error("Expected XFrameOptions to match")
	}

	if config.ContentSecurityPolicy != originalConfig.ContentSecurityPolicy {
		t.Error("Expected ContentSecurityPolicy to match")
	}

	if config.StrictTransportSecurity != originalConfig.StrictTransportSecurity {
		t.Error("Expected StrictTransportSecurity to match")
	}
}

func TestMiddleware_SetCSPDynamically(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:               true,
		ContentSecurityPolicy: "default-src 'self'",
	})

	// Update CSP dynamically
	newCSP := "default-src 'none'"
	security.SetCSP(newCSP)

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Security-Policy") != newCSP {
		t.Errorf("Expected CSP %s, got %s", newCSP, w.Header().Get("Content-Security-Policy"))
	}
}

func TestMiddleware_EnabledToggle(t *testing.T) {
	security := NewSecurity(&SecurityConfig{
		Enabled:       true,
		XFrameOptions: "DENY",
	})

	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with headers enabled
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("Expected X-Frame-Options header when enabled")
	}

	// Disable security headers
	security.SetEnabled(false)

	// Test with headers disabled
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Frame-Options") != "" {
		t.Error("Expected no X-Frame-Options header when disabled")
	}
}
