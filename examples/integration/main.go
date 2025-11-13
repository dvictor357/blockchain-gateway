package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/cache"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/resilience"
	"github.com/redis/go-redis/v9"
)

// BlockchainService demonstrates integration of all library components
type BlockchainService struct {
	blockchainManager *blockchain.SimpleClientManager
	cache             *cache.SimpleCache
	resilience        *resilience.SimpleResilienceManager
}

func NewBlockchainService(redisClient *redis.Client) (*BlockchainService, error) {
	// Create blockchain client manager
	bcManager, err := blockchain.NewSimpleClientManager(
		blockchain.WithTimeout(10*time.Second),
		blockchain.WithCache(5*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain manager: %w", err)
	}

	// Create multi-layer cache
	cacheConfig := config.RedisConfig{
		Host:    "localhost",
		Port:    "6379",
		Enabled: true,
	}

	simpleCache := cache.NewSimpleCache(
		cache.WithMemoryCache(1000, 5*time.Minute),        // L1 cache
		cache.WithRedisCache(cacheConfig, 30*time.Minute), // L2 cache
		cache.WithStats(true),
	)

	// Create resilience manager
	resilienceManager := resilience.NewSimpleResilienceManager(
		redisClient,
		resilience.WithCircuitBreakerSettings(5, 60*time.Second, 30*time.Second),
		resilience.WithRateLimit(100, time.Minute),
		resilience.WithRetry(3, 200*time.Millisecond, 5*time.Second, 2.0),
	)

	return &BlockchainService{
		blockchainManager: bcManager,
		cache:             simpleCache,
		resilience:        resilienceManager,
	}, nil
}

// GetBalanceWithCaching gets balance with caching and resilience protection
func (bs *BlockchainService) GetBalanceWithCaching(ctx context.Context, chain, address string) (interface{}, error) {
	cacheKey := fmt.Sprintf("balance:%s:%s", chain, address)

	// Try cache first
	cachedBalance, err := bs.cache.Get(ctx, cacheKey)
	if err == nil {
		fmt.Printf("Cache HIT for %s balance\n", address)
		return string(cachedBalance), nil
	}

	// Check rate limit
	allowed, err := bs.resilience.CheckRateLimit(ctx, fmt.Sprintf("balance:%s", chain))
	if err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("rate limit exceeded for %s", chain)
	}

	// Execute with circuit breaker and retry protection
	result, err := bs.resilience.ExecuteWithProtection(ctx, fmt.Sprintf("blockchain_%s", chain), func() (interface{}, error) {
		balance, err := bs.blockchainManager.QuickBalance(ctx, chain, address)
		if err != nil {
			return nil, fmt.Errorf("blockchain call failed: %w", err)
		}

		// Format balance for caching
		balanceStr := fmt.Sprintf(`{"address":"%s","balance":"%s","symbol":"%s","chain":"%s"}`,
			balance.Address, balance.Balance.String(), balance.Symbol, balance.Chain)

		// Cache the result
		if err := bs.cache.Set(ctx, cacheKey, []byte(balanceStr)); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: failed to cache balance: %v\n", err)
		}

		return balance, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get balance with protection: %w", err)
	}

	return result, nil
}

// GetLatestBlockWithCaching gets latest block with caching and resilience protection
func (bs *BlockchainService) GetLatestBlockWithCaching(ctx context.Context, chain string) (interface{}, error) {
	cacheKey := fmt.Sprintf("latest_block:%s", chain)

	// Try cache first
	cachedBlock, err := bs.cache.Get(ctx, cacheKey)
	if err == nil {
		fmt.Printf("Cache HIT for %s latest block\n", chain)
		return string(cachedBlock), nil
	}

	// Execute with protection
	result, err := bs.resilience.ExecuteWithProtection(ctx, fmt.Sprintf("blockchain_%s", chain), func() (interface{}, error) {
		block, err := bs.blockchainManager.QuickLatestBlock(ctx, chain)
		if err != nil {
			return nil, fmt.Errorf("blockchain call failed: %w", err)
		}

		// Format block for caching (shorter TTL for blocks)
		blockStr := fmt.Sprintf(`{"number":%d,"hash":"%s","chain":"%s","timestamp":%d}`,
			block.Number, block.Hash, block.Chain, block.Timestamp)

		// Cache with shorter TTL (blocks change frequently)
		if err := bs.cache.SetWithTTL(ctx, cacheKey, []byte(blockStr), 30*time.Second); err != nil {
			fmt.Printf("Warning: failed to cache block: %v\n", err)
		}

		return block, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get latest block with protection: %w", err)
	}

	return result, nil
}

// GetServiceStats returns statistics from all components
func (bs *BlockchainService) GetServiceStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Cache stats
	cacheStats, err := bs.cache.GetStats(ctx)
	if err == nil {
		stats["cache"] = cacheStats
	}

	// Circuit breaker stats
	cbStats := bs.resilience.GetCircuitBreakerStats()
	stats["circuit_breakers"] = cbStats

	// Supported chains
	chains := bs.blockchainManager.ListChains()
	stats["supported_chains"] = chains

	return stats, nil
}

func main() {
	fmt.Println("=== Integration Example: Blockchain Service ===")

	ctx := context.Background()

	// Create Redis client (optional - will work without Redis too)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Create integrated service
	service, err := NewBlockchainService(redisClient)
	if err != nil {
		log.Fatal(err)
	}

	// Example 1: Get balance with caching and resilience
	fmt.Println("\n1. Getting balance with caching and resilience:")

	address := "0x742d35Cc6634C0532925a3b844Bc454e4438f44e"
	balance, err := service.GetBalanceWithCaching(ctx, "ethereum", address)
	if err != nil {
		fmt.Printf("Balance error (expected if offline): %v\n", err)
	} else {
		fmt.Printf("Balance result: %+v\n", balance)
	}

	// Second call should hit cache
	fmt.Println("\n2. Second call (should hit cache):")
	balance2, err := service.GetBalanceWithCaching(ctx, "ethereum", address)
	if err != nil {
		fmt.Printf("Balance error: %v\n", err)
	} else {
		fmt.Printf("Cached balance result: %+v\n", balance2)
	}

	// Example 3: Get latest block with caching
	fmt.Println("\n3. Getting latest block with caching:")
	block, err := service.GetLatestBlockWithCaching(ctx, "ethereum")
	if err != nil {
		fmt.Printf("Block error (expected if offline): %v\n", err)
	} else {
		fmt.Printf("Block result: %+v\n", block)
	}

	// Example 4: Concurrent requests with resilience
	fmt.Println("\n4. Concurrent requests with resilience:")

	var wg sync.WaitGroup
	chains := []string{"ethereum", "polygon", "arbitrum"}

	for i, chain := range chains {
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func(chainIndex int, chainName string, requestNum int) {
				defer wg.Done()

				// Simulate different addresses
				testAddr := fmt.Sprintf("0x1234567890abcdef%d%d", chainIndex, requestNum)

				start := time.Now()
				result, err := service.GetBalanceWithCaching(ctx, chainName, testAddr)
				duration := time.Since(start)

				if err != nil {
					fmt.Printf("Concurrent request %s-%d failed in %v: %v\n", chainName, requestNum, duration, err)
				} else {
					fmt.Printf("Concurrent request %s-%d succeeded in %v\n", chainName, requestNum, duration)
					// Don't print the full result to keep output clean
					if rand.Float32() < 0.3 { // Print occasionally
						fmt.Printf("  Sample result type: %T\n", result)
					}
				}
			}(i, chain, j)
		}
	}

	wg.Wait()

	// Example 5: Service statistics
	fmt.Println("\n5. Service statistics:")
	stats, err := service.GetServiceStats(ctx)
	if err != nil {
		fmt.Printf("Stats error: %v\n", err)
	} else {
		fmt.Printf("Service stats: %+v\n", stats)
	}

	// Example 6: Demonstrate resilience patterns
	fmt.Println("\n6. Demonstrating resilience patterns:")

	// Simulate a failing external service
	for i := 0; i < 10; i++ {
		_, err := service.resilience.ExecuteWithProtection(ctx, "failing_service", func() (interface{}, error) {
			// 60% failure rate
			if rand.Float32() < 0.6 {
				return nil, fmt.Errorf("simulated service failure")
			}
			return fmt.Sprintf("success_%d", i), nil
		})

		if err != nil {
			fmt.Printf("Resilience test %d failed: %v\n", i, err)
		} else {
			fmt.Printf("Resilience test %d succeeded\n", i)
		}

		time.Sleep(200 * time.Millisecond)
	}

	// Final statistics
	fmt.Println("\n7. Final service statistics:")
	finalStats, err := service.GetServiceStats(ctx)
	if err != nil {
		fmt.Printf("Final stats error: %v\n", err)
	} else {
		fmt.Printf("Final service stats: %+v\n", finalStats)
	}

	fmt.Println("\n=== Integration Example completed ===")
	fmt.Println("\nKey takeaways:")
	fmt.Println("✅ All three libraries work together seamlessly")
	fmt.Println("✅ Cache provides fast responses for repeated requests")
	fmt.Println("✅ Resilience patterns protect against failures")
	fmt.Println("✅ Circuit breakers prevent cascading failures")
	fmt.Println("✅ Rate limiting prevents abuse")
	fmt.Println("✅ Retry logic handles temporary failures")
	fmt.Println("✅ Everything works even without Redis (graceful degradation)")
}
