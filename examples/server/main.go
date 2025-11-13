package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/cache"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/resilience"
	"github.com/redis/go-redis/v9"
)

// ServerExample demonstrates how the main server could use library components
type ServerExample struct {
	blockchainManager *blockchain.SimpleClientManager
	cache             *cache.SimpleCache
	resilience        *resilience.SimpleResilienceManager
}

func NewServerExample(appConfig *config.AppConfig) (*ServerExample, error) {
	// Create Redis client if enabled
	var redisClient *redis.Client
	if appConfig.Redis.Enabled {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%s", appConfig.Redis.Host, appConfig.Redis.Port),
			Password: appConfig.Redis.Password,
			DB:       appConfig.Redis.DB,
		})
	}

	// Create blockchain client manager using library constructors
	bcManager, err := blockchain.NewSimpleClientManager(
		blockchain.WithTimeout(30*time.Second), // Use default timeout
		blockchain.WithCache(5*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain manager: %w", err)
	}

	// Create cache using library constructors
	var simpleCache *cache.SimpleCache
	if appConfig.Redis.Enabled {
		redisConfig := config.RedisConfig{
			Host:     appConfig.Redis.Host,
			Port:     appConfig.Redis.Port,
			Password: appConfig.Redis.Password,
			DB:       appConfig.Redis.DB,
			Enabled:  true,
		}

		simpleCache = cache.NewSimpleCache(
			cache.WithMemoryCache(1000, 5*time.Minute),        // L1 cache
			cache.WithRedisCache(redisConfig, 30*time.Minute), // L2 cache
			cache.WithStats(true),
		)
	} else {
		// Memory-only cache if Redis is disabled
		simpleCache = cache.NewSimpleCache(
			cache.WithMemoryCache(1000, 5*time.Minute),
			cache.WithStats(true),
		)
	}

	// Create resilience manager using library constructors
	var resilienceManager *resilience.SimpleResilienceManager
	if redisClient != nil {
		resilienceManager = resilience.NewSimpleResilienceManager(
			redisClient,
			resilience.WithCircuitBreakerSettings(5, 60*time.Second, 30*time.Second),
			resilience.WithRateLimit(appConfig.Server.RateLimit, time.Minute),
			resilience.WithRetry(3, 200*time.Millisecond, 5*time.Second, 2.0),
		)
	}

	return &ServerExample{
		blockchainManager: bcManager,
		cache:             simpleCache,
		resilience:        resilienceManager,
	}, nil
}

// GetBalance demonstrates a server endpoint using library components
func (s *ServerExample) GetBalance(ctx context.Context, chain, address string) (interface{}, error) {
	cacheKey := fmt.Sprintf("balance:%s:%s", chain, address)

	// Try cache first
	if cached, err := s.cache.Get(ctx, cacheKey); err == nil {
		log.Printf("Cache HIT for balance %s on %s", address, chain)
		return string(cached), nil
	}

	// Check rate limit if resilience manager is available
	if s.resilience != nil {
		allowed, err := s.resilience.CheckRateLimit(ctx, fmt.Sprintf("balance:%s", chain))
		if err != nil {
			return nil, fmt.Errorf("rate limit check failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("rate limit exceeded for %s", chain)
		}
	}

	// Get balance from blockchain
	balance, err := s.blockchainManager.QuickBalance(ctx, chain, address)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Format and cache the result
	result := fmt.Sprintf(`{
		"address": "%s",
		"balance": "%s",
		"symbol": "%s",
		"chain": "%s",
		"decimals": %d
	}`, balance.Address, balance.Balance.String(), balance.Symbol, balance.Chain, balance.Decimals)

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, []byte(result)); err != nil {
		log.Printf("Warning: failed to cache balance: %v", err)
	}

	return balance, nil
}

// GetLatestBlock demonstrates another server endpoint
func (s *ServerExample) GetLatestBlock(ctx context.Context, chain string) (interface{}, error) {
	cacheKey := fmt.Sprintf("latest_block:%s", chain)

	// Try cache first
	if cached, err := s.cache.Get(ctx, cacheKey); err == nil {
		log.Printf("Cache HIT for latest block on %s", chain)
		return string(cached), nil
	}

	// Get latest block from blockchain
	block, err := s.blockchainManager.QuickLatestBlock(ctx, chain)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %w", err)
	}

	// Format and cache the result (shorter TTL for blocks)
	result := fmt.Sprintf(`{
		"number": %d,
		"hash": "%s",
		"parent_hash": "%s",
		"timestamp": %d,
		"transaction_count": %d,
		"chain": "%s"
	}`, block.Number, block.Hash, block.ParentHash, block.Timestamp, block.TransactionCount, block.Chain)

	// Cache with shorter TTL (blocks change frequently)
	if err := s.cache.SetWithTTL(ctx, cacheKey, []byte(result), 30*time.Second); err != nil {
		log.Printf("Warning: failed to cache block: %v", err)
	}

	return block, nil
}

// GetHealth demonstrates a health check endpoint
func (s *ServerExample) GetHealth(ctx context.Context) (map[string]interface{}, error) {
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"version":   "2.0.0-library",
	}

	// Add cache stats
	if cacheStats, err := s.cache.GetStats(ctx); err == nil {
		health["cache"] = cacheStats
	}

	// Add circuit breaker stats
	if s.resilience != nil {
		health["circuit_breakers"] = s.resilience.GetCircuitBreakerStats()
	}

	// Add supported chains
	health["supported_chains"] = s.blockchainManager.ListChains()

	return health, nil
}

// GetStats demonstrates a statistics endpoint
func (s *ServerExample) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Cache statistics
	if cacheStats, err := s.cache.GetStats(ctx); err == nil {
		stats["cache"] = cacheStats
	}

	// Circuit breaker statistics
	if s.resilience != nil {
		stats["circuit_breakers"] = s.resilience.GetCircuitBreakerStats()
	}

	// Blockchain statistics
	stats["blockchain"] = map[string]interface{}{
		"supported_chains": s.blockchainManager.ListChains(),
		"total_chains":     len(s.blockchainManager.ListChains()),
	}

	stats["server"] = map[string]interface{}{
		"uptime":       time.Since(time.Now()), // This would be tracked properly in real server
		"library_mode": true,
	}

	return stats, nil
}

func main() {
	fmt.Println("=== Server Example with Library Components ===")

	// Load configuration (same as original server)
	appConfig := config.LoadConfig()

	// Create server using library components
	server, err := NewServerExample(appConfig)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Demonstrate server endpoints
	fmt.Println("\n1. Health Check:")
	health, err := server.GetHealth(ctx)
	if err != nil {
		log.Printf("Health check error: %v", err)
	} else {
		fmt.Printf("Health: %+v\n", health)
	}

	// Example balance requests
	fmt.Println("\n2. Balance Requests:")
	addresses := []string{
		"0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
		"0x1234567890abcdef1234567890abcdef12345678",
	}

	for i, addr := range addresses {
		fmt.Printf("\nRequest %d - Balance for %s:\n", i+1, addr)

		balance, err := server.GetBalance(ctx, "ethereum", addr)
		if err != nil {
			fmt.Printf("Error (expected if offline): %v\n", err)
		} else {
			fmt.Printf("Balance result type: %T\n", balance)
		}

		// Second request should hit cache
		fmt.Printf("Request %d (cached) - Balance for %s:\n", i+1, addr)
		balance2, err := server.GetBalance(ctx, "ethereum", addr)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Cached balance result type: %T\n", balance2)
		}
	}

	// Example latest block requests
	fmt.Println("\n3. Latest Block Requests:")
	chains := []string{"ethereum", "polygon", "arbitrum"}

	for i, chain := range chains {
		fmt.Printf("\nRequest %d - Latest block for %s:\n", i+1, chain)

		block, err := server.GetLatestBlock(ctx, chain)
		if err != nil {
			fmt.Printf("Error (expected if offline): %v\n", err)
		} else {
			fmt.Printf("Block result type: %T\n", block)
		}
	}

	// Statistics
	fmt.Println("\n4. Server Statistics:")
	stats, err := server.GetStats(ctx)
	if err != nil {
		fmt.Printf("Stats error: %v\n", err)
	} else {
		fmt.Printf("Server stats: %+v\n", stats)
	}

	fmt.Println("\n=== Server Example completed ===")
	fmt.Println("\nKey improvements with library components:")
	fmt.Println("✅ Simplified server initialization")
	fmt.Println("✅ Consistent configuration patterns")
	fmt.Println("✅ Better separation of concerns")
	fmt.Println("✅ Reusable components")
	fmt.Println("✅ Easier testing and maintenance")
	fmt.Println("✅ Graceful degradation when Redis unavailable")
	fmt.Println("✅ Built-in resilience patterns")
}
