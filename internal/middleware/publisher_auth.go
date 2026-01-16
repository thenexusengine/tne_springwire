// Package middleware provides HTTP middleware for PBS
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// PublisherAuthConfig holds publisher authentication configuration
type PublisherAuthConfig struct {
	Enabled           bool              // Enable publisher validation
	AllowUnregistered bool              // Allow requests without publisher ID (for testing)
	RegisteredPubs    map[string]string // publisher_id -> allowed domains (comma-separated, empty = any)
	ValidateDomain    bool              // Validate request domain matches registered domains
	RateLimitPerPub   int               // Requests per second per publisher (0 = unlimited)
	UseRedis          bool              // Use Redis for publisher validation
}

// DefaultPublisherAuthConfig returns default config
// SECURITY: Publisher auth is ENABLED by default in production mode
// Set PUBLISHER_AUTH_ENABLED=false explicitly to disable (development only)
func DefaultPublisherAuthConfig() *PublisherAuthConfig {
	// Production-secure default: enabled unless explicitly disabled
	enabled := os.Getenv("PUBLISHER_AUTH_ENABLED") != "false"

	// In development mode (AUTH_ENABLED=false), also allow unregistered publishers
	// This maintains backward compatibility while being secure by default
	devMode := os.Getenv("AUTH_ENABLED") == "false"
	allowUnregistered := os.Getenv("PUBLISHER_ALLOW_UNREGISTERED") == "true" || devMode

	return &PublisherAuthConfig{
		Enabled:           enabled,
		AllowUnregistered: allowUnregistered,
		RegisteredPubs:    parsePublishers(os.Getenv("REGISTERED_PUBLISHERS")),
		ValidateDomain:    os.Getenv("PUBLISHER_VALIDATE_DOMAIN") == "true",
		RateLimitPerPub:   100, // Default 100 RPS per publisher
		UseRedis:          os.Getenv("PUBLISHER_AUTH_USE_REDIS") != "false",
	}
}

// parsePublishers parses "pub1:domain1.com,pub2:domain2.com" format
func parsePublishers(envValue string) map[string]string {
	pubs := make(map[string]string)
	if envValue == "" {
		return pubs
	}

	pairs := strings.Split(envValue, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			pubs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		} else if len(parts) == 1 && parts[0] != "" {
			// Publisher without domain restriction
			pubs[strings.TrimSpace(parts[0])] = ""
		}
	}
	return pubs
}

// minimalBidRequest is a minimal struct for extracting publisher info
type minimalBidRequest struct {
	Site *struct {
		Domain    string `json:"domain"`
		Publisher *struct {
			ID string `json:"id"`
		} `json:"publisher"`
	} `json:"site"`
	App *struct {
		Bundle    string `json:"bundle"`
		Publisher *struct {
			ID string `json:"id"`
		} `json:"publisher"`
	} `json:"app"`
}

// PublisherStore interface for database operations
type PublisherStore interface {
	GetByPublisherID(ctx context.Context, publisherID string) (publisher interface{}, err error)
}

// PublisherAuth provides publisher authentication for auction endpoints
type PublisherAuth struct {
	config         *PublisherAuthConfig
	redisClient    RedisClient
	publisherStore PublisherStore
	mu             sync.RWMutex

	// Rate limiting per publisher
	rateLimits   map[string]*rateLimitEntry
	rateLimitsMu sync.RWMutex

	// IVT detection
	ivtDetector *IVTDetector
}

type rateLimitEntry struct {
	tokens    float64
	lastCheck time.Time
}

// Redis key for registered publishers
const RedisPublishersHash = "tne_catalyst:publishers" // hash: publisher_id -> allowed_domains

// maxRequestBodySize limits request body reads to prevent OOM attacks (1MB)
const maxRequestBodySize = 1024 * 1024

// publisherContextKey is the context key for storing publisher objects
type contextKey string

const publisherContextKey contextKey = "publisher"

// NewPublisherAuth creates a new publisher auth middleware
func NewPublisherAuth(config *PublisherAuthConfig) *PublisherAuth {
	if config == nil {
		config = DefaultPublisherAuthConfig()
	}
	return &PublisherAuth{
		config:      config,
		rateLimits:  make(map[string]*rateLimitEntry),
		ivtDetector: NewIVTDetector(DefaultIVTConfig()),
	}
}

// SetRedisClient sets the Redis client for publisher validation
func (p *PublisherAuth) SetRedisClient(client RedisClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.redisClient = client
}

// SetPublisherStore sets the PostgreSQL publisher store
func (p *PublisherAuth) SetPublisherStore(store PublisherStore) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publisherStore = store
}

// Middleware returns the publisher authentication middleware handler
func (p *PublisherAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.RLock()
		enabled := p.config.Enabled
		p.mu.RUnlock()

		// Skip if disabled
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Only apply to POST requests to auction endpoints
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/openrtb2/auction") {
			next.ServeHTTP(w, r)
			return
		}

		// Read and buffer the body so it can be re-read by the handler
		// Use LimitReader to prevent OOM from oversized requests
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
		r.Body.Close()
		if err != nil {
			http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
			return
		}

		// Parse minimal request to extract publisher info
		var minReq minimalBidRequest
		if err := json.Unmarshal(body, &minReq); err != nil {
			// Let the main handler deal with invalid JSON
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}

		// Extract publisher ID
		publisherID, domain := p.extractPublisherInfo(&minReq)

		// Validate publisher
		if err := p.validatePublisher(r.Context(), publisherID, domain); err != nil {
			log.Warn().
				Str("publisher_id", publisherID).
				Str("domain", domain).
				Str("error", err.Error()).
				Msg("Publisher validation failed")
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusForbidden)
			return
		}

		// IVT detection (Invalid Traffic)
		if p.ivtDetector != nil {
			ivtResult := p.ivtDetector.Validate(r.Context(), r, publisherID, domain)

			// Log IVT detection
			if !ivtResult.IsValid {
				log.Warn().
					Str("publisher_id", publisherID).
					Str("domain", domain).
					Str("ip", ivtResult.IPAddress).
					Str("ua", ivtResult.UserAgent).
					Int("ivt_score", ivtResult.Score).
					Int("signal_count", len(ivtResult.Signals)).
					Bool("blocked", ivtResult.ShouldBlock).
					Msg("IVT detected")
			}

			// Block if IVT score is high and blocking is enabled
			if ivtResult.ShouldBlock {
				log.Warn().
					Str("publisher_id", publisherID).
					Str("reason", ivtResult.BlockReason).
					Int("score", ivtResult.Score).
					Msg("Request blocked - IVT detected")
				http.Error(w, `{"error":"invalid traffic detected"}`, http.StatusForbidden)
				return
			}

			// Add IVT score to headers for monitoring (even if not blocking)
			r.Header.Set("X-IVT-Score", strconv.Itoa(ivtResult.Score))
			if len(ivtResult.Signals) > 0 {
				r.Header.Set("X-IVT-Signals", strconv.Itoa(len(ivtResult.Signals)))
			}
		}

		// Apply rate limiting per publisher
		if publisherID != "" && !p.checkRateLimit(publisherID) {
			log.Warn().
				Str("publisher_id", publisherID).
				Msg("Publisher rate limit exceeded")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		// Add publisher ID to request context via header
		r.Header.Set("X-Publisher-ID", publisherID)

		// Retrieve and store full publisher object in context for downstream use
		if publisherID != "" && p.publisherStore != nil {
			pub, err := p.publisherStore.GetByPublisherID(r.Context(), publisherID)
			if err == nil && pub != nil {
				// Store publisher in context for exchange to access bid_multiplier
				ctx := context.WithValue(r.Context(), publisherContextKey, pub)
				r = r.WithContext(ctx)
			}
		}

		// Restore body for handler
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// extractPublisherInfo extracts publisher ID and domain from request
func (p *PublisherAuth) extractPublisherInfo(req *minimalBidRequest) (publisherID, domain string) {
	if req.Site != nil {
		domain = req.Site.Domain
		if req.Site.Publisher != nil {
			publisherID = req.Site.Publisher.ID
		}
	} else if req.App != nil {
		domain = req.App.Bundle
		if req.App.Publisher != nil {
			publisherID = req.App.Publisher.ID
		}
	}
	return
}

// validatePublisher validates the publisher ID and domain
// Checks in order: PostgreSQL database, Redis, in-memory RegisteredPubs
func (p *PublisherAuth) validatePublisher(ctx context.Context, publisherID, domain string) error {
	p.mu.RLock()
	allowUnregistered := p.config.AllowUnregistered
	validateDomain := p.config.ValidateDomain
	publisherStore := p.publisherStore
	useRedis := p.config.UseRedis
	redisClient := p.redisClient
	registeredPubs := p.config.RegisteredPubs
	p.mu.RUnlock()

	// No publisher ID
	if publisherID == "" {
		if allowUnregistered {
			return nil
		}
		return &PublisherAuthError{Code: "missing_publisher", Message: "publisher ID required"}
	}

	// 1. Check PostgreSQL database (primary source of truth if configured)
	if publisherStore != nil {
		pub, err := publisherStore.GetByPublisherID(ctx, publisherID)
		if err != nil {
			return &PublisherAuthError{
				Code:    "database_error",
				Message: "failed to query publisher",
				Cause:   err,
			}
		}

		// Publisher not found in database
		if pub == nil {
			if allowUnregistered {
				return nil
			}
			return &PublisherAuthError{Code: "unknown_publisher", Message: "publisher not registered"}
		}

		// Extract allowed domains from publisher record
		// The pub interface{} is expected to have an AllowedDomains string field
		type domainProvider interface {
			GetAllowedDomains() string
		}

		var allowedDomains string
		if dp, ok := pub.(domainProvider); ok {
			allowedDomains = dp.GetAllowedDomains()
		} else {
			// Try type assertion to map for flexibility
			if pubMap, ok := pub.(map[string]interface{}); ok {
				if ad, ok := pubMap["allowed_domains"].(string); ok {
					allowedDomains = ad
				}
			}
		}

		// Validate domain if required
		if validateDomain && allowedDomains != "" && allowedDomains != "*" {
			if !p.domainMatches(domain, allowedDomains) {
				return &PublisherAuthError{Code: "domain_mismatch", Message: "domain not allowed for publisher"}
			}
		}

		return nil
	}

	// 2. Check Redis (secondary source if configured)
	if useRedis && redisClient != nil {
		allowedDomains, err := redisClient.HGet(ctx, RedisPublishersHash, publisherID)
		if err != nil {
			// Redis error - log but continue to fallback
			log.Warn().Err(err).Str("publisher_id", publisherID).Msg("Redis lookup failed, falling back")
		} else if allowedDomains != "" {
			// Publisher found in Redis
			// Validate domain if required
			if validateDomain && allowedDomains != "*" {
				if !p.domainMatches(domain, allowedDomains) {
					return &PublisherAuthError{Code: "domain_mismatch", Message: "domain not allowed for publisher"}
				}
			}
			return nil
		}
		// Publisher not found in Redis, continue to fallback
	}

	// 3. Check in-memory RegisteredPubs (fallback for simple deployments and testing)
	if len(registeredPubs) > 0 {
		allowedDomains, exists := registeredPubs[publisherID]
		if exists {
			// Publisher found in memory
			// Validate domain if required
			if validateDomain && allowedDomains != "" && allowedDomains != "*" {
				if !p.domainMatches(domain, allowedDomains) {
					return &PublisherAuthError{Code: "domain_mismatch", Message: "domain not allowed for publisher"}
				}
			}
			return nil
		}
		// Publisher not in RegisteredPubs - reject if we have publishers defined
		if !allowUnregistered {
			return &PublisherAuthError{Code: "unknown_publisher", Message: "publisher not registered"}
		}
	}

	// No validation source configured
	if allowUnregistered {
		return nil
	}

	return &PublisherAuthError{
		Code:    "system_error",
		Message: "publisher database not configured",
	}
}

// domainMatches checks if domain matches allowed domains (comma-separated)
func (p *PublisherAuth) domainMatches(domain, allowedDomains string) bool {
	if domain == "" {
		return false
	}

	for _, allowed := range strings.Split(allowedDomains, "|") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		// Support wildcard subdomains
		if strings.HasPrefix(allowed, "*.") {
			suffix := allowed[1:] // ".example.com"
			if strings.HasSuffix(domain, suffix) || domain == allowed[2:] {
				return true
			}
		} else if domain == allowed {
			return true
		}
	}
	return false
}

// checkRateLimit implements token bucket rate limiting per publisher
func (p *PublisherAuth) checkRateLimit(publisherID string) bool {
	p.mu.RLock()
	rateLimit := p.config.RateLimitPerPub
	p.mu.RUnlock()

	if rateLimit <= 0 {
		return true // Unlimited
	}

	p.rateLimitsMu.Lock()
	defer p.rateLimitsMu.Unlock()

	entry, exists := p.rateLimits[publisherID]
	now := time.Now()

	if !exists {
		p.rateLimits[publisherID] = &rateLimitEntry{
			tokens:    float64(rateLimit) - 1,
			lastCheck: now,
		}
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(entry.lastCheck).Seconds()
	entry.tokens += elapsed * float64(rateLimit)
	if entry.tokens > float64(rateLimit) {
		entry.tokens = float64(rateLimit)
	}
	entry.lastCheck = now

	// Try to consume a token
	if entry.tokens >= 1 {
		entry.tokens--
		return true
	}

	return false
}

// RegisterPublisher adds a publisher at runtime
func (p *PublisherAuth) RegisterPublisher(publisherID, allowedDomains string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.config.RegisteredPubs == nil {
		p.config.RegisteredPubs = make(map[string]string)
	}
	p.config.RegisteredPubs[publisherID] = allowedDomains
}

// UnregisterPublisher removes a publisher at runtime
func (p *PublisherAuth) UnregisterPublisher(publisherID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.config.RegisteredPubs, publisherID)
}

// SetEnabled enables or disables publisher authentication
func (p *PublisherAuth) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.Enabled = enabled
}

// SetIVTConfig updates IVT detection configuration at runtime
func (p *PublisherAuth) SetIVTConfig(config *IVTConfig) {
	if p.ivtDetector != nil {
		p.ivtDetector.SetConfig(config)
	}
}

// GetIVTConfig returns current IVT configuration
func (p *PublisherAuth) GetIVTConfig() *IVTConfig {
	if p.ivtDetector != nil {
		return p.ivtDetector.GetConfig()
	}
	return nil
}

// GetIVTMetrics returns current IVT detection metrics
func (p *PublisherAuth) GetIVTMetrics() IVTMetrics {
	if p.ivtDetector != nil {
		return p.ivtDetector.GetMetrics()
	}
	return IVTMetrics{}
}

// EnableIVTMonitoring enables/disables IVT monitoring (detection, logging, metrics)
func (p *PublisherAuth) EnableIVTMonitoring(enabled bool) {
	if p.ivtDetector != nil {
		config := p.ivtDetector.GetConfig()
		config.MonitoringEnabled = enabled
		p.ivtDetector.SetConfig(config)
	}
}

// EnableIVTBlocking enables/disables IVT blocking (requires monitoring to be enabled)
func (p *PublisherAuth) EnableIVTBlocking(enabled bool) {
	if p.ivtDetector != nil {
		config := p.ivtDetector.GetConfig()
		config.BlockingEnabled = enabled
		// If blocking is enabled but monitoring is not, enable monitoring automatically
		if enabled && !config.MonitoringEnabled {
			config.MonitoringEnabled = true
			log.Warn().Msg("IVT blocking requires monitoring - enabling monitoring automatically")
		}
		p.ivtDetector.SetConfig(config)
	}
}

// EnableIVT enables IVT monitoring (backward compatibility - use EnableIVTMonitoring instead)
// Deprecated: Use EnableIVTMonitoring for clarity
func (p *PublisherAuth) EnableIVT(enabled bool) {
	p.EnableIVTMonitoring(enabled)
}

// SetIVTBlockMode sets IVT blocking mode (backward compatibility - use EnableIVTBlocking instead)
// Deprecated: Use EnableIVTBlocking for clarity
func (p *PublisherAuth) SetIVTBlockMode(block bool) {
	p.EnableIVTBlocking(block)
}

// PublisherAuthError represents a publisher auth error
type PublisherAuthError struct {
	Code    string
	Message string
	Cause   error // Optional underlying cause
}

// Error returns a formatted error message including the error code
func (e *PublisherAuthError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// Unwrap returns the underlying cause for error chain support
func (e *PublisherAuthError) Unwrap() error {
	return e.Cause
}

// PublisherFromContext retrieves the publisher object from the request context
// Returns nil if no publisher was set (e.g., unregistered publisher allowed)
func PublisherFromContext(ctx context.Context) interface{} {
	if pub := ctx.Value(publisherContextKey); pub != nil {
		return pub
	}
	return nil
}

// NewContextWithPublisher creates a new context with the publisher set (for testing)
func NewContextWithPublisher(ctx context.Context, publisher interface{}) context.Context {
	return context.WithValue(ctx, publisherContextKey, publisher)
}
