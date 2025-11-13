package cache

import (
	"context"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
)

// CacheLibraryConfig provides minimal configuration for cache library usage
type CacheLibraryConfig struct {
	// Memory cache configuration
	L1MaxItems int
	L1TTL      time.Duration

	// Redis cache configuration
	L2Enabled   bool
	L2TTL       time.Duration
	RedisConfig config.RedisConfig

	// General configuration
	EnableStats bool
}

// CacheLibraryOption represents a configuration option for cache library usage
type CacheLibraryOption func(*CacheLibraryConfig)

// WithMemoryCache configures memory cache settings
func WithMemoryCache(maxItems int, ttl time.Duration) CacheLibraryOption {
	return func(c *CacheLibraryConfig) {
		c.L1MaxItems = maxItems
		c.L1TTL = ttl
	}
}

// WithRedisCache configures Redis cache settings
func WithRedisCache(redisConfig config.RedisConfig, ttl time.Duration) CacheLibraryOption {
	return func(c *CacheLibraryConfig) {
		c.L2Enabled = true
		c.RedisConfig = redisConfig
		c.L2TTL = ttl
	}
}

// WithStats enables cache statistics
func WithStats(enabled bool) CacheLibraryOption {
	return func(c *CacheLibraryConfig) {
		c.EnableStats = enabled
	}
}

// defaultCacheLibraryConfig returns default configuration for cache library usage
func defaultCacheLibraryConfig() *CacheLibraryConfig {
	return &CacheLibraryConfig{
		L1MaxItems:  1000,
		L1TTL:       5 * time.Minute,
		L2Enabled:   false,
		L2TTL:       30 * time.Minute,
		EnableStats: false,
	}
}

// NewMemoryCacheLibrary creates a new memory cache for library usage
func NewMemoryCacheLibrary(opts ...CacheLibraryOption) *MemoryCache {
	config := defaultCacheLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	return NewMemoryCache(config.L1MaxItems)
}

// NewRedisCacheLibrary creates a new Redis cache for library usage
func NewRedisCacheLibrary(redisConfig config.RedisConfig, opts ...CacheLibraryOption) (*RedisCache, error) {
	config := defaultCacheLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	return NewRedisCache(redisConfig)
}

// NewCacheAggregatorLibrary creates a new cache aggregator for library usage
// This is a simplified version that doesn't require the full application config
func NewCacheAggregatorLibrary(opts ...CacheLibraryOption) *CacheAggregator {
	config := defaultCacheLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	var l1 *MemoryCache
	var l2 *RedisCache

	// Create L1 cache (memory)
	if config.L1MaxItems > 0 {
		l1 = NewMemoryCache(config.L1MaxItems)
	}

	// Create L2 cache (Redis)
	if config.L2Enabled {
		l2, _ = NewRedisCache(config.RedisConfig)
	}

	// Create aggregator with a no-op fetcher (will be provided by caller)
	return NewCacheAggregator(l1, l2, nil)
}

// SimpleCache provides a simplified interface for common cache operations
type SimpleCache struct {
	aggregator *CacheAggregator
	defaultTTL time.Duration
}

// NewSimpleCache creates a simple cache with sensible defaults
func NewSimpleCache(opts ...CacheLibraryOption) *SimpleCache {
	config := defaultCacheLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	aggregator := NewCacheAggregatorLibrary(opts...)

	defaultTTL := config.L1TTL
	if config.L2Enabled {
		defaultTTL = config.L2TTL
	}

	return &SimpleCache{
		aggregator: aggregator,
		defaultTTL: defaultTTL,
	}
}

// Get retrieves a value from cache
func (sc *SimpleCache) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := sc.aggregator.Get(ctx, key, sc.defaultTTL)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Set stores a value in cache
func (sc *SimpleCache) Set(ctx context.Context, key string, value []byte) error {
	return sc.aggregator.Set(ctx, key, value, sc.defaultTTL)
}

// SetWithTTL stores a value in cache with specific TTL
func (sc *SimpleCache) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return sc.aggregator.Set(ctx, key, value, ttl)
}

// Delete removes a value from cache
func (sc *SimpleCache) Delete(ctx context.Context, key string) error {
	return sc.aggregator.Invalidate(ctx, key)
}

// Clear removes all values from cache
func (sc *SimpleCache) Clear(ctx context.Context) error {
	return sc.aggregator.InvalidateByPattern(ctx, "*")
}

// GetStats returns cache statistics
func (sc *SimpleCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return sc.aggregator.GetStats(ctx)
}

// GetOrFetch gets a value from cache or fetches it using the provided function
func (sc *SimpleCache) GetOrFetch(ctx context.Context, key string, fetcher func(ctx context.Context) ([]byte, error)) ([]byte, error) {
	// Try to get from cache first
	result, err := sc.Get(ctx, key)
	if err == nil {
		return result, nil
	}

	// If cache miss, fetch data
	data, err := fetcher(ctx)
	if err != nil {
		return nil, err
	}

	// Store in cache
	if err := sc.Set(ctx, key, data); err != nil {
		// Log error but don't fail the operation
		// In a real library, you might want to use proper logging
	}

	return data, nil
}
