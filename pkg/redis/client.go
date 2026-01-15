// Package redis provides a Redis client for PBS with connection pooling
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// Client wraps a Redis connection pool
type Client struct {
	client *redis.Client
}

// ClientConfig holds configuration for the Redis client
type ClientConfig struct {
	// Connection pool size (default: 10 * runtime.GOMAXPROCS)
	PoolSize int
	// Minimum idle connections to maintain
	MinIdleConns int
	// Maximum connection age before recycling
	MaxConnAge time.Duration
	// Timeout for establishing new connections
	DialTimeout time.Duration
	// Timeout for socket reads
	ReadTimeout time.Duration
	// Timeout for socket writes
	WriteTimeout time.Duration
	// Timeout for getting connection from pool
	PoolTimeout time.Duration
}

// DefaultClientConfig returns production-ready configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		PoolSize:     100,              // Handle high concurrency
		MinIdleConns: 10,               // Keep warm connections ready
		MaxConnAge:   30 * time.Minute, // Recycle connections periodically
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	}
}

// New creates a new Redis client from a URL with default configuration
func New(redisURL string) (*Client, error) {
	return NewWithConfig(redisURL, DefaultClientConfig())
}

// NewWithConfig creates a new Redis client with custom configuration
func NewWithConfig(redisURL string, cfg *ClientConfig) (*Client, error) {
	if redisURL == "" {
		return nil, fmt.Errorf("redis URL is empty")
	}

	if cfg == nil {
		cfg = DefaultClientConfig()
	}

	// Parse Redis URL
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	// Apply our configuration
	opts.PoolSize = cfg.PoolSize
	opts.MinIdleConns = cfg.MinIdleConns
	opts.ConnMaxLifetime = cfg.MaxConnAge
	opts.DialTimeout = cfg.DialTimeout
	opts.ReadTimeout = cfg.ReadTimeout
	opts.WriteTimeout = cfg.WriteTimeout
	opts.PoolTimeout = cfg.PoolTimeout

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Warn().Err(err).Str("address", opts.Addr).Msg("Redis connection test failed")
		// Don't fail - we'll retry on each request
	} else {
		log.Info().
			Str("address", opts.Addr).
			Int("pool_size", cfg.PoolSize).
			Int("min_idle", cfg.MinIdleConns).
			Msg("Redis connected with connection pooling")
	}

	return &Client{client: client}, nil
}

// HGet gets a hash field value
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	result, err := c.client.HGet(ctx, key, field).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return result, err
}

// HGetAll gets all fields and values from a hash
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// HSet sets a hash field value
func (c *Client) HSet(ctx context.Context, key, field string, value interface{}) error {
	return c.client.HSet(ctx, key, field, value).Err()
}

// HDel deletes hash fields
func (c *Client) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, key, fields...).Err()
}

// SMembers gets all members of a set
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// Ping tests the connection
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close closes the connection pool
func (c *Client) Close() error {
	return c.client.Close()
}

// PoolStats returns connection pool statistics for monitoring
func (c *Client) PoolStats() *redis.PoolStats {
	return c.client.PoolStats()
}

// Do executes a generic Redis command (for compatibility)
func (c *Client) Do(ctx context.Context, args ...interface{}) *redis.Cmd {
	return c.client.Do(ctx, args...)
}
