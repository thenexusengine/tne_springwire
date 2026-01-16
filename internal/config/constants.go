// Package config provides shared configuration constants for PBS
package config

import "time"

// Server timeout defaults
const (
	// ServerReadTimeout is the maximum duration for reading the entire request
	ServerReadTimeout = 5 * time.Second

	// ServerWriteTimeout is the maximum duration before timing out writes of the response
	ServerWriteTimeout = 10 * time.Second

	// ServerIdleTimeout is the maximum time to wait for the next request when keep-alives are enabled
	ServerIdleTimeout = 120 * time.Second

	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout = 30 * time.Second
)

// CORS defaults
const (
	// CORSMaxAge is the preflight cache duration in seconds (24 hours)
	CORSMaxAge = 86400
)

// Auth cache defaults
const (
	// AuthCacheTimeout is how long to cache valid API keys
	AuthCacheTimeout = 60 * time.Second

	// AuthNegativeCacheTimeout is how long to cache invalid API key results
	AuthNegativeCacheTimeout = 10 * time.Second
)

// Rate limiting defaults
const (
	// DefaultRPS is the default requests per second limit
	DefaultRPS = 1000

	// DefaultBurstSize is the default burst size for rate limiting
	DefaultBurstSize = 100

	// DefaultPublisherRPS is the default RPS per publisher
	DefaultPublisherRPS = 100
)

// Size limiting defaults
const (
	// DefaultMaxBodySize is the default maximum request body size (1MB)
	DefaultMaxBodySize = 1024 * 1024

	// DefaultMaxURLLength is the default maximum URL length (8KB)
	DefaultMaxURLLength = 8192
)

// Gzip compression defaults
const (
	// GzipMinLength is the minimum response size to compress (256 bytes)
	GzipMinLength = 256
)

// IDR client defaults
const (
	// IDRDefaultTimeout is the default timeout for IDR requests
	IDRDefaultTimeout = 150 * time.Millisecond

	// IDRMaxResponseSize is the maximum response size from IDR (1MB)
	IDRMaxResponseSize = 1024 * 1024

	// IDRMaxConnsPerHost is the maximum connections per host for IDR
	IDRMaxConnsPerHost = 100

	// IDRIdleConnTimeout is how long to keep idle connections
	IDRIdleConnTimeout = 120 * time.Second
)

// Redis defaults
const (
	// RedisPoolSize is the default connection pool size
	RedisPoolSize = 100
)

// Exchange defaults
const (
	// DefaultAuctionTimeout is the default timeout for auctions
	DefaultAuctionTimeout = 1000 * time.Millisecond

	// DefaultMaxBidders is the maximum number of bidders per request
	DefaultMaxBidders = 50

	// DefaultMaxConcurrentBidders is the default concurrent bidder limit
	DefaultMaxConcurrentBidders = 10

	// DefaultEventBufferSize is the default event buffer size
	DefaultEventBufferSize = 100

)

// Cookie sync defaults
const (
	// MaxCookieSize is the maximum cookie size allowed (4KB browser limit)
	MaxCookieSize = 4000
)

// HSTS defaults
const (
	// HSTSMaxAgeSeconds is the max-age for HSTS header (1 year)
	HSTSMaxAgeSeconds = 31536000
)

// P2-7: NBR codes consolidated in openrtb/response.go
// Use openrtb.NoBidXxx constants for all no-bid reasons
