package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/dvictor357/blockchain-gateway/pkg/cache"
)

// CachedClientManager wraps ClientManager with multi-layer caching
type CachedClientManager struct {
	manager *ClientManager
	cache   *cache.CacheAggregator
	enabled bool
}

// NewCachedClientManager creates a new cached client manager
func NewCachedClientManager(manager *ClientManager, cache *cache.CacheAggregator) *CachedClientManager {
	return &CachedClientManager{
		manager: manager,
		cache:   cache,
		enabled: true,
	}
}

// Execute executes an RPC method with caching support
func (cm *CachedClientManager) Execute(ctx context.Context, chain, method string, params interface{}) (json.RawMessage, error) {
	// Check if caching is enabled
	if !cm.enabled {
		return cm.manager.Execute(ctx, chain, method, params)
	}

	// Try to get from cache
	result, err := cm.cache.GetRPCData(ctx, chain, method, params, func(fetchCtx context.Context) (json.RawMessage, error) {
		return cm.manager.Execute(fetchCtx, chain, method, params)
	})

	if err != nil {
		return nil, fmt.Errorf("cache get error: %w", err)
	}

	return result, nil
}

// BatchExecute executes multiple RPC requests with caching support
func (cm *CachedClientManager) BatchExecute(ctx context.Context, requests map[string][]RPCRequest) (map[string][]RPCResponse, error) {
	// For batch requests, we don't use caching for now
	// Could be implemented later with batch-specific caching logic
	return cm.manager.BatchExecute(ctx, requests)
}

// GetClient returns a client for the specified blockchain (uncached)
func (cm *CachedClientManager) GetClient(chain string) (Client, error) {
	return cm.manager.GetClient(chain)
}

// ListChains returns a list of all supported blockchain names
func (cm *CachedClientManager) ListChains() []string {
	return cm.manager.ListChains()
}

// GetBalance retrieves balance with caching
func (cm *CachedClientManager) GetBalance(ctx context.Context, chain, address string) (*Balance, error) {
	if !cm.enabled {
		return cm.manager.GetBalance(ctx, chain, address)
	}

	// Use cached data fetcher and convert to typed result
	result, err := cm.cache.GetBalanceData(ctx, chain, address, func(fetchCtx context.Context) (json.RawMessage, error) {
		typedResult, err := cm.manager.GetBalance(fetchCtx, chain, address)
		if err != nil {
			return nil, err
		}
		// Marshal typed result to JSON
		data, err := json.Marshal(typedResult)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(data), nil
	})

	if err != nil {
		return nil, fmt.Errorf("cache get balance error: %w", err)
	}

	// Unmarshal to typed structure
	var balance Balance
	if err := json.Unmarshal(result, &balance); err != nil {
		return nil, fmt.Errorf("failed to unmarshal balance: %w", err)
	}

	return &balance, nil
}

// GetLatestBlock retrieves latest block with caching
func (cm *CachedClientManager) GetLatestBlock(ctx context.Context, chain string) (*BlockInfo, error) {
	if !cm.enabled {
		return cm.manager.GetLatestBlock(ctx, chain)
	}

	result, err := cm.cache.GetBlockData(ctx, chain, func(fetchCtx context.Context) (json.RawMessage, error) {
		typedResult, err := cm.manager.GetLatestBlock(fetchCtx, chain)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(typedResult)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(data), nil
	})

	if err != nil {
		return nil, fmt.Errorf("cache get block error: %w", err)
	}

	var blockInfo BlockInfo
	if err := json.Unmarshal(result, &blockInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &blockInfo, nil
}

// GetTransaction retrieves transaction with caching
func (cm *CachedClientManager) GetTransaction(ctx context.Context, chain, hash string) (*TransactionInfo, error) {
	if !cm.enabled {
		return cm.manager.GetTransaction(ctx, chain, hash)
	}

	result, err := cm.cache.GetTransactionData(ctx, chain, hash, func(fetchCtx context.Context) (json.RawMessage, error) {
		typedResult, err := cm.manager.GetTransaction(fetchCtx, chain, hash)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(typedResult)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(data), nil
	})

	if err != nil {
		return nil, fmt.Errorf("cache get transaction error: %w", err)
	}

	var txInfo TransactionInfo
	if err := json.Unmarshal(result, &txInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}

	return &txInfo, nil
}

// GetGasPrice retrieves gas price with caching
func (cm *CachedClientManager) GetGasPrice(ctx context.Context, chain string) (*big.Int, error) {
	if !cm.enabled {
		return cm.manager.GetGasPrice(ctx, chain)
	}

	result, err := cm.cache.GetGasPriceData(ctx, chain, func(fetchCtx context.Context) (json.RawMessage, error) {
		typedResult, err := cm.manager.GetGasPrice(fetchCtx, chain)
		if err != nil {
			return nil, err
		}
		// Marshal big.Int as string
		data, err := json.Marshal(typedResult.String())
		if err != nil {
			return nil, err
		}
		return json.RawMessage(data), nil
	})

	if err != nil {
		return nil, fmt.Errorf("cache get gas price error: %w", err)
	}

	var gasPriceStr string
	if err := json.Unmarshal(result, &gasPriceStr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gas price: %w", err)
	}

	gasPrice := new(big.Int)
	gasPrice.SetString(gasPriceStr, 10)

	return gasPrice, nil
}

// GetTransactionCount retrieves transaction count with caching
func (cm *CachedClientManager) GetTransactionCount(ctx context.Context, chain, address string) (uint64, error) {
	if !cm.enabled {
		return cm.manager.GetTransactionCount(ctx, chain, address)
	}

	result, err := cm.cache.GetNonceData(ctx, chain, address, func(fetchCtx context.Context) (json.RawMessage, error) {
		typedResult, err := cm.manager.GetTransactionCount(fetchCtx, chain, address)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(typedResult)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(data), nil
	})

	if err != nil {
		return 0, fmt.Errorf("cache get transaction count error: %w", err)
	}

	var count uint64
	if err := json.Unmarshal(result, &count); err != nil {
		return 0, fmt.Errorf("failed to unmarshal transaction count: %w", err)
	}

	return count, nil
}

// DisableCaching disables the cache for this client manager
func (cm *CachedClientManager) DisableCaching() {
	cm.enabled = false
}

// EnableCaching enables the cache for this client manager
func (cm *CachedClientManager) EnableCaching() {
	cm.enabled = true
}

// IsCachingEnabled returns whether caching is enabled
func (cm *CachedClientManager) IsCachingEnabled() bool {
	return cm.enabled
}

// InvalidateCache invalidates cache entries matching a pattern
func (cm *CachedClientManager) InvalidateCache(ctx context.Context, pattern string) error {
	return cm.cache.InvalidateByPattern(ctx, pattern)
}

// GetCacheStats returns cache statistics
func (cm *CachedClientManager) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	return cm.cache.GetStats(ctx)
}
