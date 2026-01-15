// Package middleware provides HTTP middleware for PBS
package middleware

import (
	"context"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// IVTConfig holds Invalid Traffic detection configuration
// Thread-safety: Protected by IVTDetector.mu, not embedded mutex
type IVTConfig struct {
	MonitoringEnabled    bool     // Enable IVT detection, logging, and metrics
	BlockingEnabled      bool     // Block high-score traffic (requires MonitoringEnabled)
	CheckUserAgent       bool     // Validate user agent patterns
	CheckReferer         bool     // Validate referer against domain
	CheckGeo             bool     // Validate IP geo restrictions (requires GeoIP)
	CheckRateLimit       bool     // Already implemented in publisher_auth
	AllowedCountries     []string // Whitelist of country codes (empty = all allowed)
	BlockedCountries     []string // Blacklist of country codes
	SuspiciousUAPatterns []string // Regex patterns for suspicious user agents
	RequireReferer       bool     // Require referer header (strict mode)
}

// DefaultIVTConfig returns production-safe defaults with environment variable overrides
func DefaultIVTConfig() *IVTConfig {
	// Helper to parse bool env vars
	parseBool := func(envKey string, defaultVal bool) bool {
		if val := os.Getenv(envKey); val != "" {
			if parsed, err := strconv.ParseBool(val); err == nil {
				return parsed
			}
		}
		return defaultVal
	}

	// Helper to parse string slice env vars (comma-separated)
	parseStringSlice := func(envKey string) []string {
		if val := os.Getenv(envKey); val != "" {
			parts := strings.Split(val, ",")
			result := make([]string, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			return result
		}
		return []string{}
	}

	// Parse monitoring and blocking flags
	monitoringEnabled := parseBool("IVT_MONITORING_ENABLED", true)
	blockingEnabled := parseBool("IVT_BLOCKING_ENABLED", false)

	// If blocking is enabled, monitoring must be enabled too
	if blockingEnabled && !monitoringEnabled {
		monitoringEnabled = true
		log.Warn().Msg("IVT_BLOCKING_ENABLED requires IVT_MONITORING_ENABLED - enabling monitoring automatically")
	}

	config := &IVTConfig{
		// IVT_MONITORING_ENABLED: Enable IVT detection, logging, and metrics (default: true)
		MonitoringEnabled: monitoringEnabled,

		// IVT_BLOCKING_ENABLED: Block high-score traffic (default: false = monitoring only)
		BlockingEnabled: blockingEnabled,

		// Individual check toggles
		CheckUserAgent: parseBool("IVT_CHECK_UA", true),
		CheckReferer:   parseBool("IVT_CHECK_REFERER", true),
		CheckGeo:       parseBool("IVT_CHECK_GEO", false),
		CheckRateLimit: parseBool("IVT_CHECK_RATELIMIT", true),

		// Geographic restrictions
		// IVT_ALLOWED_COUNTRIES: Comma-separated country codes (e.g., "US,GB,CA")
		AllowedCountries: parseStringSlice("IVT_ALLOWED_COUNTRIES"),

		// IVT_BLOCKED_COUNTRIES: Comma-separated country codes (e.g., "CN,RU")
		BlockedCountries: parseStringSlice("IVT_BLOCKED_COUNTRIES"),

		// Default suspicious UA patterns (can be extended via code)
		SuspiciousUAPatterns: []string{
			// Bots and scrapers (common patterns)
			`(?i)bot`,
			`(?i)crawler`,
			`(?i)spider`,
			`(?i)scraper`,
			`(?i)curl`,
			`(?i)wget`,
			`(?i)python`,
			`(?i)\bjava\b`, // Match "java" as whole word (not in "javascript")
			`(?i)phantom`,
			`(?i)headless`,
			`(?i)selenium`,
			// Suspicious patterns
			`^$`,            // Empty UA
			`^Mozilla/4.0$`, // Ancient UA
			`(?i)test`,
			`(?i)scanner`,
		},

		// IVT_REQUIRE_REFERER: Strict mode - require referer header (default: false)
		RequireReferer: parseBool("IVT_REQUIRE_REFERER", false),
	}

	return config
}

// IVTSignal represents a detected IVT indicator
type IVTSignal struct {
	Type        string    // Type of signal (domain_mismatch, suspicious_ua, etc.)
	Severity    string    // low, medium, high
	Description string    // Human-readable description
	DetectedAt  time.Time // When detected
}

// IVTResult contains the validation result
type IVTResult struct {
	IsValid       bool          // Overall validity
	Signals       []IVTSignal   // All detected signals
	Score         int           // IVT score (0-100, higher = more suspicious)
	BlockReason   string        // Reason for blocking (if blocked)
	ShouldBlock   bool          // Whether to block this request
	PublisherID   string        // Publisher ID from request
	Domain        string        // Domain from request
	IPAddress     string        // Client IP
	UserAgent     string        // User agent
	DetectionTime time.Duration // Time taken to detect
}

// IVTDetector provides Invalid Traffic detection
type IVTDetector struct {
	config  *IVTConfig
	mu      sync.RWMutex
	metrics *IVTMetrics

	// Compiled regex patterns (cached for performance)
	uaPatterns   []*regexp.Regexp
	patternsOnce sync.Once
}

// IVTMetrics tracks IVT detection metrics
type IVTMetrics struct {
	mu sync.RWMutex

	// Detection counts
	TotalChecked int64 // Total requests checked
	TotalFlagged int64 // Requests flagged as IVT
	TotalBlocked int64 // Requests blocked

	// Signal counts
	DomainMismatches int64 // Domain validation failures
	SuspiciousUA     int64 // Suspicious user agents
	InvalidReferer   int64 // Invalid/missing referers
	GeoMismatches    int64 // Geographic restrictions
	RateLimitHits    int64 // Rate limit exceeded

	// Performance
	LastCheckTime    time.Time
	AvgCheckDuration time.Duration
}

// NewIVTDetector creates a new IVT detector
func NewIVTDetector(config *IVTConfig) *IVTDetector {
	if config == nil {
		config = DefaultIVTConfig()
	}

	return &IVTDetector{
		config:  config,
		metrics: &IVTMetrics{},
	}
}

// compilePatterns compiles regex patterns once for performance
func (d *IVTDetector) compilePatterns() {
	d.patternsOnce.Do(func() {
		d.mu.RLock()
		patterns := d.config.SuspiciousUAPatterns
		d.mu.RUnlock()

		d.uaPatterns = make([]*regexp.Regexp, 0, len(patterns))
		for _, pattern := range patterns {
			if re, err := regexp.Compile(pattern); err == nil {
				d.uaPatterns = append(d.uaPatterns, re)
			} else {
				log.Warn().Err(err).Str("pattern", pattern).Msg("Failed to compile IVT UA pattern")
			}
		}
	})
}

// Validate performs IVT detection on a request
func (d *IVTDetector) Validate(ctx context.Context, r *http.Request, publisherID, domain string) *IVTResult {
	startTime := time.Now()

	// Snapshot entire config once to reduce lock contention
	d.mu.RLock()
	cfg := *d.config
	d.mu.RUnlock()

	result := &IVTResult{
		IsValid:     true,
		Signals:     []IVTSignal{},
		Score:       0,
		PublisherID: publisherID,
		Domain:      domain,
		IPAddress:   getClientIP(r),
		UserAgent:   r.UserAgent(),
	}

	// Skip if monitoring disabled
	if !cfg.MonitoringEnabled {
		result.DetectionTime = time.Since(startTime)
		return result
	}

	// Run all checks with snapshotted config
	d.checkUserAgentWithConfig(r, result, &cfg)
	d.checkRefererWithConfig(r, domain, result, &cfg)
	d.checkGeoWithConfig(r, result, &cfg)

	// Calculate final score and decision
	result.Score = d.calculateScore(result.Signals)
	result.ShouldBlock = cfg.BlockingEnabled && result.Score >= 70 // Block at 70+ score
	result.IsValid = result.Score < 70                             // Valid if score < 70

	if result.ShouldBlock && len(result.Signals) > 0 {
		result.BlockReason = result.Signals[0].Description // Use first signal as reason
	}

	result.DetectionTime = time.Since(startTime)

	// Update metrics
	d.updateMetrics(result)

	return result
}

// checkUserAgentWithConfig validates user agent patterns using snapshotted config
func (d *IVTDetector) checkUserAgentWithConfig(r *http.Request, result *IVTResult, cfg *IVTConfig) {
	if !cfg.CheckUserAgent {
		return
	}

	ua := r.UserAgent()

	// Empty UA check
	if ua == "" {
		result.Signals = append(result.Signals, IVTSignal{
			Type:        "suspicious_ua",
			Severity:    "medium",
			Description: "missing user agent",
			DetectedAt:  time.Now(),
		})
		return
	}

	// Check against suspicious patterns
	d.compilePatterns()
	for _, pattern := range d.uaPatterns {
		if pattern.MatchString(ua) {
			result.Signals = append(result.Signals, IVTSignal{
				Type:        "suspicious_ua",
				Severity:    "high",
				Description: "suspicious user agent pattern detected",
				DetectedAt:  time.Now(),
			})
			return
		}
	}
}

// checkRefererWithConfig validates referer against domain using snapshotted config
func (d *IVTDetector) checkRefererWithConfig(r *http.Request, domain string, result *IVTResult, cfg *IVTConfig) {
	if !cfg.CheckReferer {
		return
	}

	referer := r.Referer()

	// Missing referer check (if required)
	if referer == "" {
		if cfg.RequireReferer {
			result.Signals = append(result.Signals, IVTSignal{
				Type:        "invalid_referer",
				Severity:    "medium",
				Description: "missing referer header",
				DetectedAt:  time.Now(),
			})
		}
		return
	}

	// Validate referer matches domain
	if domain != "" && !strings.Contains(referer, domain) {
		// Extract domain from referer
		refererDomain := extractDomain(referer)
		if refererDomain != domain {
			result.Signals = append(result.Signals, IVTSignal{
				Type:        "invalid_referer",
				Severity:    "high",
				Description: "referer domain mismatch",
				DetectedAt:  time.Now(),
			})
		}
	}
}

// checkGeoWithConfig validates geographic restrictions using snapshotted config
//
//nolint:unparam // result will be used when GeoIP lookup is implemented
func (d *IVTDetector) checkGeoWithConfig(r *http.Request, result *IVTResult, cfg *IVTConfig) {
	if !cfg.CheckGeo {
		return
	}

	// Extract client IP for future GeoIP lookup
	clientIP := getClientIP(r)
	if clientIP == "" {
		return
	}

	// TODO: Implement GeoIP lookup
	// This requires a GeoIP database (MaxMind, IP2Location, etc.)
	// When implemented:
	// country := geoip.Lookup(clientIP)
	// if len(cfg.AllowedCountries) > 0 && !contains(cfg.AllowedCountries, country) {
	//     result.Signals = append(result.Signals, IVTSignal{
	//         Type:        "geo_restricted",
	//         Severity:    "high",
	//         Description: fmt.Sprintf("country %s not in allowed list", country),
	//         DetectedAt:  time.Now(),
	//     })
	// }
	// if len(cfg.BlockedCountries) > 0 && contains(cfg.BlockedCountries, country) {
	//     result.Signals = append(result.Signals, IVTSignal{
	//         Type:        "geo_blocked",
	//         Severity:    "high",
	//         Description: fmt.Sprintf("country %s is blocked", country),
	//         DetectedAt:  time.Now(),
	//     })
	// }

	_ = cfg.AllowedCountries
	_ = cfg.BlockedCountries
}

// calculateScore computes IVT score from signals
func (d *IVTDetector) calculateScore(signals []IVTSignal) int {
	score := 0
	for _, signal := range signals {
		switch signal.Severity {
		case "low":
			score += 15
		case "medium":
			score += 35
		case "high":
			score += 50
		}
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

// updateMetrics updates detection metrics
func (d *IVTDetector) updateMetrics(result *IVTResult) {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()

	d.metrics.TotalChecked++
	d.metrics.LastCheckTime = time.Now()

	// Update average check duration
	if d.metrics.TotalChecked == 1 {
		d.metrics.AvgCheckDuration = result.DetectionTime
	} else {
		// Running average
		d.metrics.AvgCheckDuration = time.Duration(
			(int64(d.metrics.AvgCheckDuration)*(d.metrics.TotalChecked-1) + int64(result.DetectionTime)) / d.metrics.TotalChecked,
		)
	}

	if !result.IsValid {
		d.metrics.TotalFlagged++
	}

	if result.ShouldBlock {
		d.metrics.TotalBlocked++
	}

	// Update signal counts
	for _, signal := range result.Signals {
		switch signal.Type {
		case "domain_mismatch":
			d.metrics.DomainMismatches++
		case "suspicious_ua":
			d.metrics.SuspiciousUA++
		case "invalid_referer":
			d.metrics.InvalidReferer++
		case "geo_mismatch":
			d.metrics.GeoMismatches++
		case "rate_limit":
			d.metrics.RateLimitHits++
		}
	}
}

// GetMetrics returns current IVT metrics
func (d *IVTDetector) GetMetrics() IVTMetrics {
	d.metrics.mu.RLock()
	defer d.metrics.mu.RUnlock()

	return IVTMetrics{
		TotalChecked:     d.metrics.TotalChecked,
		TotalFlagged:     d.metrics.TotalFlagged,
		TotalBlocked:     d.metrics.TotalBlocked,
		DomainMismatches: d.metrics.DomainMismatches,
		SuspiciousUA:     d.metrics.SuspiciousUA,
		InvalidReferer:   d.metrics.InvalidReferer,
		GeoMismatches:    d.metrics.GeoMismatches,
		RateLimitHits:    d.metrics.RateLimitHits,
		LastCheckTime:    d.metrics.LastCheckTime,
		AvgCheckDuration: d.metrics.AvgCheckDuration,
	}
}

// SetConfig updates IVT configuration at runtime
func (d *IVTDetector) SetConfig(config *IVTConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.config = config

	// Reset compiled patterns to force recompile
	d.patternsOnce = sync.Once{}
}

// GetConfig returns current configuration
func (d *IVTDetector) GetConfig() *IVTConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config
}

// Helper functions

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr) //nolint:errcheck // RemoteAddr may not have port
	return ip
}

// extractDomain extracts domain from URL
func extractDomain(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove path
	if idx := strings.Index(url, "/"); idx > 0 {
		url = url[:idx]
	}

	// Remove port
	if idx := strings.Index(url, ":"); idx > 0 {
		url = url[:idx]
	}

	return url
}
