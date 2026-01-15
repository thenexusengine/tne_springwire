package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestIVTDetector_SuspiciousUA(t *testing.T) {
	detector := NewIVTDetector(nil) // Use defaults

	tests := []struct {
		name               string
		userAgent          string
		shouldBeSuspicious bool
	}{
		{"Normal Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36", false},
		{"Normal Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:89.0) Gecko/20100101 Firefox/89.0", false},
		{"Bot - explicit", "Googlebot/2.1", true},
		{"Bot - crawler", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", true},
		{"Scraper - curl", "curl/7.68.0", true},
		{"Scraper - python", "python-requests/2.25.1", true},
		{"Headless Chrome", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/91.0.4472.124 Safari/537.36", true},
		{"Empty UA", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
			req.Header.Set("User-Agent", tt.userAgent)

			result := detector.Validate(context.Background(), req, "test-pub", "example.com")

			hasSuspiciousUA := false
			for _, signal := range result.Signals {
				if signal.Type == "suspicious_ua" {
					hasSuspiciousUA = true
					break
				}
			}

			if hasSuspiciousUA != tt.shouldBeSuspicious {
				t.Errorf("UA: %q - expected suspicious=%v, got suspicious=%v (score=%d)",
					tt.userAgent, tt.shouldBeSuspicious, hasSuspiciousUA, result.Score)
			}
		})
	}
}

func TestIVTDetector_RefererValidation(t *testing.T) {
	detector := NewIVTDetector(nil)

	tests := []struct {
		name            string
		referer         string
		domain          string
		shouldBeInvalid bool
	}{
		{"Valid referer", "https://example.com/page", "example.com", false},
		{"Valid subdomain", "https://www.example.com/page", "example.com", false},
		{"Mismatch", "https://malicious.com/page", "example.com", true},
		{"Empty referer", "", "example.com", false}, // Not invalid unless RequireReferer is true
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (normal browser)")
			req.Header.Set("Referer", tt.referer)

			result := detector.Validate(context.Background(), req, "test-pub", tt.domain)

			hasInvalidReferer := false
			for _, signal := range result.Signals {
				if signal.Type == "invalid_referer" {
					hasInvalidReferer = true
					break
				}
			}

			if hasInvalidReferer != tt.shouldBeInvalid {
				t.Errorf("Referer: %q, Domain: %q - expected invalid=%v, got invalid=%v",
					tt.referer, tt.domain, tt.shouldBeInvalid, hasInvalidReferer)
			}
		})
	}
}

func TestIVTDetector_Scoring(t *testing.T) {
	detector := NewIVTDetector(nil)

	tests := []struct {
		name        string
		userAgent   string
		referer     string
		domain      string
		expectScore int // Approximate score
		expectBlock bool
	}{
		{
			name:        "Clean request",
			userAgent:   "Mozilla/5.0 (normal browser)",
			referer:     "https://example.com",
			domain:      "example.com",
			expectScore: 0,
			expectBlock: false,
		},
		{
			name:        "Bot UA only (high severity = 50 points)",
			userAgent:   "Googlebot/2.1",
			referer:     "https://example.com",
			domain:      "example.com",
			expectScore: 50,
			expectBlock: false, // Below 70 threshold
		},
		{
			name:        "Bot UA + Referer mismatch (50 + 50 = 100)",
			userAgent:   "curl/7.68.0",
			referer:     "https://malicious.com",
			domain:      "example.com",
			expectScore: 100,
			expectBlock: true, // >= 70 threshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Enable blocking for this test
			config := DefaultIVTConfig()
			config.BlockingEnabled = true
			detector.SetConfig(config)

			req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			req.Header.Set("Referer", tt.referer)

			result := detector.Validate(context.Background(), req, "test-pub", tt.domain)

			if result.Score != tt.expectScore {
				t.Errorf("Expected score %d, got %d (signals: %d)",
					tt.expectScore, result.Score, len(result.Signals))
			}

			if result.ShouldBlock != tt.expectBlock {
				t.Errorf("Expected block=%v, got block=%v (score=%d)",
					tt.expectBlock, result.ShouldBlock, result.Score)
			}
		})
	}
}

func TestIVTDetector_Disabled(t *testing.T) {
	config := DefaultIVTConfig()
	config.MonitoringEnabled = false
	detector := NewIVTDetector(config)

	req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
	req.Header.Set("User-Agent", "curl/7.68.0") // Suspicious

	result := detector.Validate(context.Background(), req, "test-pub", "example.com")

	if !result.IsValid {
		t.Error("Expected valid when IVT detection is disabled")
	}

	if len(result.Signals) > 0 {
		t.Errorf("Expected no signals when disabled, got %d", len(result.Signals))
	}
}

func TestIVTDetector_Metrics(t *testing.T) {
	config := DefaultIVTConfig()
	config.BlockingEnabled = false // Monitoring mode (don't block, just flag)
	detector := NewIVTDetector(config)

	// Run some clean validations
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Referer", "https://example.com")
		detector.Validate(context.Background(), req, "test-pub", "example.com")
	}

	// Run some suspicious ones with score >= 70 (should be flagged but not blocked in monitoring mode)
	// curl UA (50) + referer mismatch (50) = 100 score -> flagged
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/openrtb2/auction", nil)
		req.Header.Set("User-Agent", "curl/7.68.0")
		req.Header.Set("Referer", "https://malicious.com")
		detector.Validate(context.Background(), req, "test-pub", "example.com")
	}

	metrics := detector.GetMetrics()

	if metrics.TotalChecked != 15 {
		t.Errorf("Expected 15 total checks, got %d", metrics.TotalChecked)
	}

	if metrics.TotalFlagged != 5 {
		t.Errorf("Expected 5 flagged, got %d", metrics.TotalFlagged)
	}

	if metrics.SuspiciousUA != 5 {
		t.Errorf("Expected 5 suspicious UA signals, got %d", metrics.SuspiciousUA)
	}

	if metrics.InvalidReferer != 5 {
		t.Errorf("Expected 5 invalid referer signals, got %d", metrics.InvalidReferer)
	}

	if metrics.TotalBlocked != 0 {
		t.Errorf("Expected 0 blocked (monitoring mode), got %d", metrics.TotalBlocked)
	}
}

func TestIVTDetector_ClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		expectedIP string
	}{
		{"Direct connection", "192.168.1.1:12345", "", "", "192.168.1.1"},
		{"X-Forwarded-For", "127.0.0.1:12345", "203.0.113.1, 198.51.100.1", "", "203.0.113.1"},
		{"X-Real-IP", "127.0.0.1:12345", "", "203.0.113.1", "203.0.113.1"},
		{"Both headers (XFF wins)", "127.0.0.1:12345", "203.0.113.1", "198.51.100.1", "203.0.113.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			ip := getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP %s, got %s", tt.expectedIP, ip)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/path", "example.com"},
		{"http://example.com", "example.com"},
		{"https://www.example.com:8080/path", "www.example.com"},
		{"example.com/path?query=1", "example.com"},
		{"https://sub.domain.example.com/page", "sub.domain.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
