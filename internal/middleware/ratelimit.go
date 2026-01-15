package middleware

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool
	RequestsPerSecond int           // Max requests per second per client
	BurstSize         int           // Max burst size
	CleanupInterval   time.Duration // How often to clean up old entries
	WindowSize        time.Duration // Time window for rate limiting
	TrustedProxies    []*net.IPNet  // CIDR ranges of trusted proxies
	TrustXFF          bool          // Whether to trust X-Forwarded-For at all
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	rps, err := strconv.Atoi(os.Getenv("RATE_LIMIT_RPS"))
	if err != nil || rps <= 0 {
		rps = 1000 // Default: 1000 requests per second
	}

	burst, err := strconv.Atoi(os.Getenv("RATE_LIMIT_BURST"))
	if err != nil || burst <= 0 {
		burst = rps * 2 // Default burst size
	}

	// Parse trusted proxies from env (comma-separated CIDR ranges)
	// Example: TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1/32
	var trustedProxies []*net.IPNet
	if proxyStr := os.Getenv("TRUSTED_PROXIES"); proxyStr != "" {
		for _, cidr := range strings.Split(proxyStr, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			// Handle single IPs by adding /32 or /128
			if !strings.Contains(cidr, "/") {
				if strings.Contains(cidr, ":") {
					cidr += "/128"
				} else {
					cidr += "/32"
				}
			}
			_, network, err := net.ParseCIDR(cidr)
			if err == nil {
				trustedProxies = append(trustedProxies, network)
			}
		}
	}

	// Only trust XFF header if trusted proxies are configured
	trustXFF := len(trustedProxies) > 0

	// P1-1: Rate limiting ENABLED by default for DoS protection
	// Set RATE_LIMIT_ENABLED=false to disable (development only)
	return &RateLimitConfig{
		Enabled:           os.Getenv("RATE_LIMIT_ENABLED") != "false",
		RequestsPerSecond: rps,
		BurstSize:         burst,
		CleanupInterval:   time.Minute,
		WindowSize:        time.Second,
		TrustedProxies:    trustedProxies,
		TrustXFF:          trustXFF,
	}
}

// clientState tracks rate limit state for a single client
type clientState struct {
	tokens    float64
	lastCheck time.Time
}

// RateLimitMetrics defines the metrics interface for rate limiter
type RateLimitMetrics interface {
	IncRateLimitRejected()
}

// RateLimiter provides rate limiting middleware using token bucket algorithm
type RateLimiter struct {
	config  *RateLimitConfig
	clients map[string]*clientState
	mu      sync.Mutex
	stopCh  chan struct{}
	metrics RateLimitMetrics
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	rl := &RateLimiter{
		config:  config,
		clients: make(map[string]*clientState),
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine only if cleanup interval is positive
	if config.CleanupInterval > 0 {
		go rl.cleanup()
	}

	return rl
}

// cleanup periodically removes stale client entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, state := range rl.clients {
				// Remove entries not seen in the last minute
				if now.Sub(state.lastCheck) > time.Minute {
					delete(rl.clients, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Middleware returns the rate limiting middleware handler
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get client identifier (prefer publisher ID from auth, fallback to IP)
		clientID := r.Header.Get("X-Publisher-ID")
		if clientID == "" {
			clientID = rl.getClientIP(r)
		}

		// Check rate limit
		if !rl.allow(clientID) {
			// Record metric for rate limit rejection
			if rl.metrics != nil {
				rl.metrics.IncRateLimitRejected()
			}
			w.Header().Set("Retry-After", "1")
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.config.RequestsPerSecond))
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		// Add rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.config.RequestsPerSecond))

		next.ServeHTTP(w, r)
	})
}

// allow checks if a request from the given client should be allowed
func (rl *RateLimiter) allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	state, exists := rl.clients[clientID]

	if !exists {
		// New client, start with burst size tokens
		rl.clients[clientID] = &clientState{
			tokens:    float64(rl.config.BurstSize - 1), // -1 for current request
			lastCheck: now,
		}
		return true
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(state.lastCheck).Seconds()
	state.tokens += elapsed * float64(rl.config.RequestsPerSecond)

	// Cap at burst size
	if state.tokens > float64(rl.config.BurstSize) {
		state.tokens = float64(rl.config.BurstSize)
	}

	state.lastCheck = now

	// Check if we have tokens available
	if state.tokens < 1 {
		return false
	}

	state.tokens--
	return true
}

// getClientIP extracts the client IP from the request with secure XFF handling
func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Get the direct connection IP (RemoteAddr)
	remoteIP := extractIP(r.RemoteAddr)

	// Only trust XFF if configured and remote IP is from a trusted proxy
	if rl.config.TrustXFF && rl.isTrustedProxy(remoteIP) {
		// Check X-Forwarded-For header
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			// Parse the XFF chain and find the rightmost untrusted IP
			// XFF format: client, proxy1, proxy2 (leftmost is original client)
			ips := strings.Split(xff, ",")
			// Walk backwards through the chain, stopping at the first untrusted IP
			for i := len(ips) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(ips[i])
				if ip == "" {
					continue
				}
				// If this IP is not a trusted proxy, it's the client
				if !rl.isTrustedProxy(ip) {
					return ip
				}
			}
		}

		// Check X-Real-IP header (set by some proxies like nginx)
		xri := r.Header.Get("X-Real-IP")
		if xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	// Fall back to RemoteAddr
	return remoteIP
}

// isTrustedProxy checks if an IP is in the trusted proxy list
func (rl *RateLimiter) isTrustedProxy(ipStr string) bool {
	if len(rl.config.TrustedProxies) == 0 {
		return false
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, network := range rl.config.TrustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// extractIP extracts the IP from an address that may include a port
func extractIP(addr string) string {
	// Handle IPv6 with port: [::1]:8080
	if strings.HasPrefix(addr, "[") {
		if idx := strings.LastIndex(addr, "]"); idx != -1 {
			return addr[1:idx]
		}
	}

	// Handle IPv4 with port or plain IP
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		// Check if this looks like IPv6 without brackets
		if strings.Count(addr, ":") > 1 {
			// It's IPv6 without port
			return addr
		}
		return addr[:idx]
	}

	return addr
}

// SetEnabled enables or disables rate limiting
func (rl *RateLimiter) SetEnabled(enabled bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.config.Enabled = enabled
}

// SetRPS sets the requests per second limit
func (rl *RateLimiter) SetRPS(rps int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.config.RequestsPerSecond = rps
}

// SetBurstSize sets the burst size
func (rl *RateLimiter) SetBurstSize(burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.config.BurstSize = burst
}

// SetMetrics sets the metrics interface for the rate limiter
func (rl *RateLimiter) SetMetrics(m RateLimitMetrics) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.metrics = m
}
