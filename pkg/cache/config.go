package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
)

// CacheConfig holds configuration for cache layers
type CacheConfig struct {
	L1MaxItems int
	L2Enabled  bool
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		L1MaxItems: 10000, // Store 10k items in memory
		L2Enabled:  true,
	}
}

// NewDefaultCacheAggregator creates a new cache aggregator with default configuration
func NewDefaultCacheAggregator(
	appConfig *config.AppConfig,
) (*CacheAggregator, error) {
	var l1 *MemoryCache
	var l2 *RedisCache
	var err error

	// Build L1 in-memory cache
	l1 = NewMemoryCache(10000)

	// Build L2 Redis cache
	if appConfig.Redis.Enabled {
		l2, err = NewRedisCache(appConfig.Redis)
		if err != nil {
			return nil, fmt.Errorf("failed to create L2 cache: %w", err)
		}
	}

	// Create the aggregator
	aggregator := NewCacheAggregator(l1, l2, nil)

	return aggregator, nil
}

// Helper functions for common cache operations

// GetRPCData retrieves cached RPC data with automatic fallback
func (c *CacheAggregator) GetRPCData(
	ctx context.Context,
	chain, method string,
	params any,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	key := fmt.Sprintf("rpc:%s:%s", chain, method)
	if params != nil {
		paramsBytes, _ := json.Marshal(params)
		key = fmt.Sprintf("rpc:%s:%s:%x", chain, method, paramsBytes)
	}

	// Determine TTL based on method
	ttl := getTTLForMethod(method)

	// Create fetcher if not provided
	c.fetcher = dataFetcher

	return c.Get(ctx, key, ttl)
}

// GetBalanceData retrieves cached balance data
func (c *CacheAggregator) GetBalanceData(
	ctx context.Context,
	chain, address string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	key := fmt.Sprintf("balance:%s:%s", chain, address)
	// Balance changes frequently, use shorter TTL
	ttl := 30 * time.Second

	c.fetcher = dataFetcher

	return c.Get(ctx, key, ttl)
}

// GetBlockData retrieves cached block data
func (c *CacheAggregator) GetBlockData(
	ctx context.Context,
	chain string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	key := fmt.Sprintf("block:latest:%s", chain)
	ttl := 30 * time.Second

	c.fetcher = dataFetcher

	return c.Get(ctx, key, ttl)
}

// GetGasPriceData retrieves cached gas price data
func (c *CacheAggregator) GetGasPriceData(
	ctx context.Context,
	chain string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	key := fmt.Sprintf("gas_price:%s", chain)
	ttl := 30 * time.Second

	c.fetcher = dataFetcher

	return c.Get(ctx, key, ttl)
}

// GetNonceData retrieves cached nonce data
func (c *CacheAggregator) GetNonceData(
	ctx context.Context,
	chain, address string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	key := fmt.Sprintf("nonce:%s:%s", chain, address)
	ttl := 30 * time.Second

	c.fetcher = dataFetcher

	return c.Get(ctx, key, ttl)
}

// GetTransactionData retrieves cached transaction data
func (c *CacheAggregator) GetTransactionData(
	ctx context.Context,
	chain, hash string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	key := fmt.Sprintf("tx:%s:%s", chain, hash)
	ttl := 5 * time.Minute

	c.fetcher = dataFetcher

	return c.Get(ctx, key, ttl)
}

// getTTLForMethod determines the appropriate TTL based on the RPC method
func getTTLForMethod(method string) time.Duration {
	// Different methods have different optimal TTLs based on how often they change
	switch method {
	case "eth_blockNumber", "eth_chainId":
		// Block numbers and chain IDs change frequently
		return 2 * time.Second
	case "eth_getBalance", "eth_getTransactionCount":
		// Balances and nonces change frequently
		return 10 * time.Second
	case "eth_getTransactionByHash", "eth_getTransactionReceipt":
		// Transactions don't change once confirmed
		return 5 * time.Minute
	case "eth_getBlockByNumber", "eth_getBlockByHash":
		// Blocks don't change once confirmed
		return 30 * time.Second
	default:
		// Default TTL for other methods
		return 30 * time.Second
	}
}
