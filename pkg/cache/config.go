package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
)

// CacheConfig holds configuration for all cache layers
type CacheConfig struct {
	L1MaxItems int
	L2Enabled  bool
	L3Enabled  bool
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		L1MaxItems: 10000, // Store 10k items in memory
		L2Enabled:  true,
		L3Enabled:  true,
	}
}

// CacheBuilder builds the cache aggregator with all layers
type CacheBuilder struct {
	config CacheConfig
}

// NewCacheBuilder creates a new cache builder
func NewCacheBuilder() *CacheBuilder {
	return &CacheBuilder{
		config: DefaultCacheConfig(),
	}
}

// WithMaxItems sets the maximum number of items for L1 cache
func (b *CacheBuilder) WithMaxItems(maxItems int) *CacheBuilder {
	b.config.L1MaxItems = maxItems
	return b
}

// WithL2Enabled enables or disables L2 Redis cache
func (b *CacheBuilder) WithL2Enabled(enabled bool) *CacheBuilder {
	b.config.L2Enabled = enabled
	return b
}

// WithL3Enabled enables or disables L3 Database cache
func (b *CacheBuilder) WithL3Enabled(enabled bool) *CacheBuilder {
	b.config.L3Enabled = enabled
	return b
}

// Build creates the cache aggregator with all configured layers
func (b *CacheBuilder) Build(
	appConfig *config.AppConfig,
	db *sql.DB,
) (*CacheAggregator, error) {
	var l1 *MemoryCache
	var l2 *RedisCache
	var l3 *DBCache
	var err error

	// Build L1 in-memory cache
	l1 = NewMemoryCache(b.config.L1MaxItems)

	// Build L2 Redis cache
	if b.config.L2Enabled {
		l2, err = NewRedisCache(appConfig.Redis)
		if err != nil {
			return nil, fmt.Errorf("failed to create L2 cache: %w", err)
		}
	}

	// Build L3 database cache
	if b.config.L3Enabled {
		l3, err = NewDBCache(db)
		if err != nil {
			return nil, fmt.Errorf("failed to create L3 cache: %w", err)
		}
		// Start background cleanup
		l3.StartCleanup()
	}

	// Create the aggregator
	aggregator := NewCacheAggregator(l1, l2, l3, nil)

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
	keyGen := &CacheKeyGenerator{}
	key, err := keyGen.GenerateRPCKey(chain, method, params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cache key: %w", err)
	}

	// Determine TTL based on method
	ttl := getTTLForMethod(method)

	// Create fetcher if not provided
	if dataFetcher != nil {
		c.fetcher = dataFetcher

		return c.Get(ctx, key, ttl, "transaction")
	}

	return c.Get(ctx, key, ttl, "rpc")
}

// GetBalanceData retrieves cached balance data
func (c *CacheAggregator) GetBalanceData(
	ctx context.Context,
	chain, address string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	keyGen := &CacheKeyGenerator{}
	key := keyGen.GenerateBalanceKey(chain, address)

	// Balance changes frequently, use shorter TTL
	ttl := L1BalanceTTL

	// Create fetcher
	if dataFetcher != nil {
		c.fetcher = dataFetcher

		return c.Get(ctx, key, ttl, "transaction")
	}

	return c.Get(ctx, key, ttl, "balance")
}

// GetBlockData retrieves cached block data
func (c *CacheAggregator) GetBlockData(
	ctx context.Context,
	chain string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	keyGen := &CacheKeyGenerator{}
	key := keyGen.GenerateBlockKey(chain)

	ttl := L1BlockTTL

	if dataFetcher != nil {
		c.fetcher = dataFetcher

		return c.Get(ctx, key, ttl, "transaction")
	}

	return c.Get(ctx, key, ttl, "block")
}

// GetGasPriceData retrieves cached gas price data
func (c *CacheAggregator) GetGasPriceData(
	ctx context.Context,
	chain string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	keyGen := &CacheKeyGenerator{}
	key := keyGen.GenerateGasPriceKey(chain)

	ttl := L1GasPriceTTL

	if dataFetcher != nil {
		c.fetcher = dataFetcher

		return c.Get(ctx, key, ttl, "transaction")
	}

	return c.Get(ctx, key, ttl, "gas_price")
}

// GetNonceData retrieves cached nonce data
func (c *CacheAggregator) GetNonceData(
	ctx context.Context,
	chain, address string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	keyGen := &CacheKeyGenerator{}
	key := keyGen.GenerateNonceKey(chain, address)

	ttl := L1NonceTTL

	if dataFetcher != nil {
		c.fetcher = dataFetcher

		return c.Get(ctx, key, ttl, "transaction")
	}

	return c.Get(ctx, key, ttl, "nonce")
}

// GetTransactionData retrieves cached transaction data
func (c *CacheAggregator) GetTransactionData(
	ctx context.Context,
	chain, hash string,
	dataFetcher DataFetcher,
) (json.RawMessage, error) {
	keyGen := &CacheKeyGenerator{}
	key := keyGen.GenerateTxKey(chain, hash)

	ttl := L1TxTTL

	if dataFetcher != nil {
		c.fetcher = dataFetcher

		return c.Get(ctx, key, ttl, "transaction")
	}

	return c.Get(ctx, key, ttl, "transaction")
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
