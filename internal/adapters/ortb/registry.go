// Package ortb provides a dynamic bidder registry that loads
// configurations from Redis at runtime.
package ortb

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

const (
	// Redis keys for bidder storage
	redisBiddersHash   = "tne_catalyst:bidders"
	redisBiddersActive = "tne_catalyst:bidders:active"
)

// P3-NEW-1: Metrics tracks operational metrics for the dynamic registry
type Metrics struct {
	mu sync.RWMutex

	// Refresh operations
	RefreshCount       int64         // Total refresh operations
	RefreshErrors      int64         // Failed refresh operations
	LastRefreshTime    time.Time     // Time of last successful refresh
	LastRefreshLatency time.Duration // Duration of last refresh

	// Lookup operations
	GetHits   int64 // Successful adapter lookups
	GetMisses int64 // Failed adapter lookups

	// Adapter stats
	TotalAdapters   int // Total registered adapters
	EnabledAdapters int // Enabled adapters
}

// GetMetrics returns a copy of the current metrics (thread-safe)
func (m *Metrics) GetMetrics() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Metrics{
		RefreshCount:       m.RefreshCount,
		RefreshErrors:      m.RefreshErrors,
		LastRefreshTime:    m.LastRefreshTime,
		LastRefreshLatency: m.LastRefreshLatency,
		GetHits:            m.GetHits,
		GetMisses:          m.GetMisses,
		TotalAdapters:      m.TotalAdapters,
		EnabledAdapters:    m.EnabledAdapters,
	}
}

func (m *Metrics) recordRefreshSuccess(latency time.Duration, total, enabled int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RefreshCount++
	m.LastRefreshTime = time.Now()
	m.LastRefreshLatency = latency
	m.TotalAdapters = total
	m.EnabledAdapters = enabled
}

func (m *Metrics) recordRefreshError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RefreshCount++
	m.RefreshErrors++
}

func (m *Metrics) recordGetHit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetHits++
}

func (m *Metrics) recordGetMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetMisses++
}

// RedisClient interface for Redis operations
type RedisClient interface {
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	SMembers(ctx context.Context, key string) ([]string, error)
	HGet(ctx context.Context, key, field string) (string, error)
}

// DynamicRegistry manages dynamically configured bidders
type DynamicRegistry struct {
	mu            sync.RWMutex
	adapters      map[string]*GenericAdapter
	redis         RedisClient
	refreshPeriod time.Duration
	stopChan      chan struct{}
	onUpdate      func(string, *BidderConfig) // Callback when a bidder is updated
	metrics       *Metrics                    // P3-NEW-1: Operational metrics
}

// NewDynamicRegistry creates a new dynamic registry
func NewDynamicRegistry(redis RedisClient, refreshPeriod time.Duration) *DynamicRegistry {
	return &DynamicRegistry{
		adapters:      make(map[string]*GenericAdapter),
		redis:         redis,
		refreshPeriod: refreshPeriod,
		stopChan:      make(chan struct{}),
		metrics:       &Metrics{}, // P3-NEW-1: Initialize metrics
	}
}

// GetRegistryMetrics returns the current metrics for the dynamic registry
func (r *DynamicRegistry) GetRegistryMetrics() Metrics {
	return r.metrics.GetMetrics()
}

// SetUpdateCallback sets a callback function to be called when a bidder is updated
func (r *DynamicRegistry) SetUpdateCallback(fn func(string, *BidderConfig)) {
	r.onUpdate = fn
}

// Start begins the background refresh goroutine
func (r *DynamicRegistry) Start(ctx context.Context) error {
	// Initial load
	if err := r.Refresh(ctx); err != nil {
		return fmt.Errorf("initial load failed: %w", err)
	}

	// Start background refresh
	go r.refreshLoop(ctx)

	return nil
}

// Stop stops the background refresh
func (r *DynamicRegistry) Stop() {
	close(r.stopChan)
}

// refreshLoop periodically refreshes configurations from Redis
func (r *DynamicRegistry) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(r.refreshPeriod)
	defer ticker.Stop()

	// P1-NEW-3: Timeout for individual refresh operations to prevent blocking
	const refreshTimeout = 5 * time.Second

	for {
		select {
		case <-ticker.C:
			// Create timeout context for each refresh to prevent blocking on slow Redis
			refreshCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
			if err := r.Refresh(refreshCtx); err != nil {
				// Log error but continue
				logger.Log.Warn().Err(err).Msg("Failed to refresh dynamic bidders")
			}
			cancel()
		case <-r.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Refresh loads all bidder configurations from Redis
func (r *DynamicRegistry) Refresh(ctx context.Context) error {
	start := time.Now() // P3-NEW-1: Track refresh latency

	// Get all bidder configs from Redis
	configs, err := r.redis.HGetAll(ctx, redisBiddersHash)
	if err != nil {
		r.metrics.recordRefreshError() // P3-NEW-1: Record error
		return fmt.Errorf("failed to get bidders from Redis: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Track which bidders we've seen
	seen := make(map[string]bool)

	// Update or create adapters
	for bidderCode, jsonStr := range configs {
		seen[bidderCode] = true

		var config BidderConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
			logger.Log.Warn().Err(err).Str("bidder", bidderCode).Msg("Failed to parse bidder config")
			continue
		}

		existing, exists := r.adapters[bidderCode]
		if exists {
			// Update existing adapter
			existing.UpdateConfig(&config)
		} else {
			// Create new adapter
			r.adapters[bidderCode] = New(&config)
		}

		// Call update callback if set
		if r.onUpdate != nil {
			r.onUpdate(bidderCode, &config)
		}
	}

	// Remove adapters that no longer exist in Redis
	for code := range r.adapters {
		if !seen[code] {
			delete(r.adapters, code)
		}
	}

	// P3-NEW-1: Record successful refresh with stats
	enabledCount := 0
	for _, adapter := range r.adapters {
		if adapter.IsEnabled() {
			enabledCount++
		}
	}
	r.metrics.recordRefreshSuccess(time.Since(start), len(r.adapters), enabledCount)

	return nil
}

// Get retrieves an adapter by bidder code
func (r *DynamicRegistry) Get(bidderCode string) (*GenericAdapter, bool) {
	// P1-NEW-5: Release registry lock before acquiring metrics lock to avoid lock ordering issues
	r.mu.RLock()
	adapter, ok := r.adapters[bidderCode]
	r.mu.RUnlock()

	// Record metrics outside the critical section to prevent potential deadlock
	if ok {
		r.metrics.recordGetHit()
	} else {
		r.metrics.recordGetMiss()
	}

	return adapter, ok
}

// GetAll returns all registered adapters
func (r *DynamicRegistry) GetAll() map[string]*GenericAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*GenericAdapter, len(r.adapters))
	for k, v := range r.adapters {
		result[k] = v
	}
	return result
}

// GetEnabled returns all enabled adapters
func (r *DynamicRegistry) GetEnabled() []*GenericAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*GenericAdapter, 0)
	for _, adapter := range r.adapters {
		if adapter.IsEnabled() {
			result = append(result, adapter)
		}
	}
	return result
}

// ListBidderCodes returns all bidder codes
func (r *DynamicRegistry) ListBidderCodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	codes := make([]string, 0, len(r.adapters))
	for code := range r.adapters {
		codes = append(codes, code)
	}
	return codes
}

// ListEnabledBidderCodes returns enabled bidder codes
func (r *DynamicRegistry) ListEnabledBidderCodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	codes := make([]string, 0)
	for code, adapter := range r.adapters {
		if adapter.IsEnabled() {
			codes = append(codes, code)
		}
	}
	return codes
}

// Count returns the number of registered adapters
func (r *DynamicRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.adapters)
}

// GetForPublisher returns adapters available for a specific publisher
func (r *DynamicRegistry) GetForPublisher(publisherID string, country string) []*GenericAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*GenericAdapter, 0)
	for _, adapter := range r.adapters {
		if !adapter.IsEnabled() {
			continue
		}
		if !adapter.CanBidForPublisher(publisherID) {
			continue
		}
		if country != "" && !adapter.CanBidForCountry(country) {
			continue
		}
		result = append(result, adapter)
	}
	return result
}

// RegisterWithStaticRegistry registers all dynamic bidders with the static registry
// This allows dynamic bidders to work with the existing auction flow
func (r *DynamicRegistry) RegisterWithStaticRegistry() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for code, adapter := range r.adapters {
		info := adapter.Info()

		// Try to register (ignore errors if already registered)
		_ = adapters.DefaultRegistry.Register(code, adapter, info) //nolint:errcheck
	}

	return nil
}

// UnregisterFromStaticRegistry removes all dynamic bidders from the static registry
func (r *DynamicRegistry) UnregisterFromStaticRegistry() {
	r.mu.RLock()
	codes := make([]string, 0, len(r.adapters))
	for code := range r.adapters {
		codes = append(codes, code) //nolint:staticcheck
	}
	r.mu.RUnlock()

	// Note: The static registry doesn't have an Unregister method,
	// so we can't actually remove them. This would require extending
	// the static registry.
}

// ToAdapterWithInfoMap converts dynamic adapters to the static registry format
func (r *DynamicRegistry) ToAdapterWithInfoMap() map[string]adapters.AdapterWithInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]adapters.AdapterWithInfo, len(r.adapters))
	for code, adapter := range r.adapters {
		result[code] = adapters.AdapterWithInfo{
			Adapter: adapter,
			Info:    adapter.Info(),
		}
	}
	return result
}

// Global dynamic registry instance
var globalDynamicRegistry *DynamicRegistry
var globalDynamicRegistryOnce sync.Once

// GetGlobalDynamicRegistry returns the global dynamic registry instance
// This should only be called after InitGlobalDynamicRegistry
func GetGlobalDynamicRegistry() *DynamicRegistry {
	return globalDynamicRegistry
}

// InitGlobalDynamicRegistry initializes the global dynamic registry
func InitGlobalDynamicRegistry(redis RedisClient, refreshPeriod time.Duration) *DynamicRegistry {
	globalDynamicRegistryOnce.Do(func() {
		globalDynamicRegistry = NewDynamicRegistry(redis, refreshPeriod)
	})
	return globalDynamicRegistry
}
