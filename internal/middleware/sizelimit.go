package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
)

// SizeLimitConfig holds request size limit configuration
type SizeLimitConfig struct {
	Enabled      bool
	MaxBodySize  int64 // Max request body size in bytes
	MaxURLLength int   // Max URL length
}

// DefaultSizeLimitConfig returns default size limit configuration
func DefaultSizeLimitConfig() *SizeLimitConfig {
	maxBody, err := strconv.ParseInt(os.Getenv("MAX_REQUEST_SIZE"), 10, 64)
	if err != nil || maxBody <= 0 {
		maxBody = 1024 * 1024 // Default: 1MB
	}

	maxURL, err := strconv.Atoi(os.Getenv("MAX_URL_LENGTH"))
	if err != nil || maxURL <= 0 {
		maxURL = 8192 // Default: 8KB
	}

	return &SizeLimitConfig{
		Enabled:      true, // Enabled by default for security
		MaxBodySize:  maxBody,
		MaxURLLength: maxURL,
	}
}

// SizeLimiter provides request size limiting middleware
type SizeLimiter struct {
	config *SizeLimitConfig
	mu     sync.RWMutex
}

// NewSizeLimiter creates a new size limiter
func NewSizeLimiter(config *SizeLimitConfig) *SizeLimiter {
	if config == nil {
		config = DefaultSizeLimitConfig()
	}
	return &SizeLimiter{config: config}
}

// Middleware returns the size limiting middleware handler
func (sl *SizeLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Copy config fields while holding the lock to prevent data race
		sl.mu.RLock()
		enabled := sl.config.Enabled
		maxURLLength := sl.config.MaxURLLength
		maxBodySize := sl.config.MaxBodySize
		sl.mu.RUnlock()

		if !enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Check URL length
		if len(r.URL.String()) > maxURLLength {
			http.Error(w, `{"error":"URL too long"}`, http.StatusRequestURITooLong)
			return
		}

		// Check Content-Length header if present
		if r.ContentLength > maxBodySize {
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
			return
		}

		// Wrap body with size limit reader
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		}

		next.ServeHTTP(w, r)
	})
}

// SetMaxBodySize sets the max body size
func (sl *SizeLimiter) SetMaxBodySize(size int64) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.config.MaxBodySize = size
}

// SetMaxURLLength sets the max URL length
func (sl *SizeLimiter) SetMaxURLLength(length int) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.config.MaxURLLength = length
}

// SetEnabled enables or disables size limiting
func (sl *SizeLimiter) SetEnabled(enabled bool) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.config.Enabled = enabled
}

// GetConfig returns a copy of the current configuration
func (sl *SizeLimiter) GetConfig() SizeLimitConfig {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return *sl.config
}
