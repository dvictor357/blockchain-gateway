# Blockchain Gateway Library

This directory contains library-friendly components that can be used independently in other Go projects. The library provides three main modules:

## 📦 Available Modules

### 1. Blockchain Client Library (`pkg/blockchain/`)
Simplified blockchain client management for multiple chains.

### 2. Cache Library (`pkg/cache/`)
Multi-layer caching system with memory and Redis support.

### 3. Resilience Library (`pkg/resilience/`)
Circuit breakers, rate limiters, and retry mechanisms.

## 🚀 Quick Start

### Basic Blockchain Client Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/dvictor357/blockchain-gateway/pkg/blockchain"
)

func main() {
    // Create a simple client manager with common chains
    manager, err := blockchain.NewSimpleClientManager(
        blockchain.WithTimeout(10*time.Second),
    )
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Get Ethereum balance
    balance, err := manager.QuickBalance(ctx, "ethereum", "0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Balance: %s %s\n", balance.Balance.String(), balance.Symbol)

    // Get latest block
    block, err := manager.QuickLatestBlock(ctx, "ethereum")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Latest block: %d\n", block.Number)
}
```

### Advanced Blockchain Client Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "time"

    "github.com/dvictor357/blockchain-gateway/pkg/blockchain"
)

func main() {
    // Create custom HTTP client
    httpClient := &http.Client{
        Timeout: 15 * time.Second,
    }

    // Create client manager with custom configuration
    manager, err := blockchain.NewClientManagerLibrary(
        blockchain.WithHTTPClient(httpClient),
        blockchain.WithTimeout(15*time.Second),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Add custom EVM chain
    customClient, err := blockchain.NewEVMClientLibrary(
        "custom-chain",
        "https://custom-rpc.example.com",
        blockchain.WithTimeout(20*time.Second),
    )
    if err != nil {
        log.Fatal(err)
    }

    err = manager.RegisterClient(customClient)
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Execute custom RPC call
    result, err := manager.QuickQuery(ctx, "custom-chain", "eth_blockNumber", []interface{}{})
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Block number: %s\n", string(result))
}
```

### Cache Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/dvictor357/blockchain-gateway/pkg/cache"
    "github.com/dvictor357/blockchain-gateway/pkg/config"
)

func main() {
    // Memory-only cache
    memCache := cache.NewSimpleCache(
        cache.WithMemoryCache(1000, 5*time.Minute),
        cache.WithStats(true),
    )

    ctx := context.Background()

    // Store value
    err := memCache.Set(ctx, "key1", []byte("value1"))
    if err != nil {
        log.Fatal(err)
    }

    // Retrieve value
    value, err := memCache.Get(ctx, "key1")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Value: %s\n", string(value))

    // Get or fetch pattern
    result, err := memCache.GetOrFetch(ctx, "expensive_key", func(ctx context.Context) ([]byte, error) {
        // Simulate expensive operation
        time.Sleep(100 * time.Millisecond)
        return []byte("expensive_result"), nil
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Result: %s\n", string(result))
}
```

### Redis Cache Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/dvictor357/blockchain-gateway/pkg/cache"
    "github.com/dvictor357/blockchain-gateway/pkg/config"
    "github.com/redis/go-redis/v9"
)

func main() {
    // Redis client
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // Redis cache configuration
    redisConfig := config.RedisConfig{
        Host:    "localhost",
        Port:    "6379",
        Enabled: true,
    }

    // Create cache with Redis
    redisCache := cache.NewSimpleCache(
        cache.WithRedisCache(redisConfig, 30*time.Minute),
        cache.WithMemoryCache(500, 5*time.Minute), // L1 cache
        cache.WithStats(true),
    )

    ctx := context.Background()

    // Store value in both L1 and L2 cache
    err := redisCache.Set(ctx, "user:123", []byte(`{"name":"John","age":30}`))
    if err != nil {
        log.Fatal(err)
    }

    // Retrieve value (will try L1 first, then L2)
    value, err := redisCache.Get(ctx, "user:123")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("User data: %s\n", string(value))

    // Get cache statistics
    stats, err := redisCache.GetStats(ctx)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Cache stats: %+v\n", stats)
}
```

### Resilience Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/dvictor357/blockchain-gateway/pkg/resilience"
    "github.com/redis/go-redis/v9"
)

func main() {
    // Redis client for rate limiting
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // Create resilience manager
    resilienceManager := resilience.NewSimpleResilienceManager(
        redisClient,
        resilience.WithCircuitBreakerSettings(5, 60*time.Second, 30*time.Second),
        resilience.WithRateLimit(100, time.Minute),
        resilience.WithRetry(3, 100*time.Millisecond, 5*time.Second, 2.0),
    )

    ctx := context.Background()

    // Check rate limit
    allowed, err := resilienceManager.CheckRateLimit(ctx, "api:user123")
    if err != nil {
        log.Fatal(err)
    }

    if !allowed {
        fmt.Println("Rate limit exceeded")
        return
    }

    // Execute operation with protection
    result, err := resilienceManager.ExecuteWithProtection(ctx, "external_api", func() (interface{}, error) {
        // Simulate external API call
        time.Sleep(50 * time.Millisecond)
        return "API response", nil
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Result: %v\n", result)

    // Get circuit breaker stats
    stats := resilienceManager.GetCircuitBreakerStats()
    fmt.Printf("Circuit breaker stats: %+v\n", stats)
}
```

## 📚 API Reference

### Blockchain Library

#### Constructors
- `NewSimpleClientManager(opts ...LibraryOption)` - Pre-configured manager with common chains
- `NewClientManagerLibrary(opts ...LibraryOption)` - Empty manager for custom configuration
- `NewEVMClientLibrary(name, rpcURL string, opts ...LibraryOption)` - EVM client
- `NewBitcoinClientLibrary(rpcURL string, opts ...LibraryOption)` - Bitcoin client

#### Options
- `WithTimeout(timeout time.Duration)` - Set HTTP timeout
- `WithHTTPClient(client *http.Client)` - Use custom HTTP client
- `WithCache(ttl time.Duration)` - Enable caching

#### Methods
- `QuickQuery(ctx, chain, method, params)` - Execute RPC call
- `QuickBalance(ctx, chain, address)` - Get account balance
- `QuickLatestBlock(ctx, chain)` - Get latest block
- `QuickTransaction(ctx, chain, hash)` - Get transaction details
- `QuickGasPrice(ctx, chain)` - Get gas price
- `QuickTransactionCount(ctx, chain, address)` - Get transaction count

### Cache Library

#### Constructors
- `NewSimpleCache(opts ...CacheLibraryOption)` - Simple cache interface
- `NewMemoryCacheLibrary(opts ...CacheLibraryOption)` - Memory-only cache
- `NewRedisCacheLibrary(config RedisConfig, opts ...CacheLibraryOption)` - Redis cache
- `NewCacheAggregatorLibrary(opts ...CacheLibraryOption)` - Multi-layer cache

#### Options
- `WithMemoryCache(maxItems int, ttl time.Duration)` - Configure memory cache
- `WithRedisCache(config RedisConfig, ttl time.Duration)` - Configure Redis cache
- `WithStats(enabled bool)` - Enable statistics

#### Methods
- `Get(ctx, key)` - Retrieve value
- `Set(ctx, key, value)` - Store value
- `SetWithTTL(ctx, key, value, ttl)` - Store with custom TTL
- `Delete(ctx, key)` - Remove value
- `Clear(ctx)` - Clear all values
- `GetOrFetch(ctx, key, fetcher)` - Get or fetch pattern
- `GetStats(ctx)` - Get cache statistics

### Resilience Library

#### Constructors
- `NewSimpleResilienceManager(redisClient, opts ...ResilienceLibraryOption)` - All-in-one manager
- `NewCircuitBreakerLibrary(opts ...ResilienceLibraryOption)` - Circuit breaker
- `NewCircuitBreakerPoolLibrary(opts ...ResilienceLibraryOption)` - Circuit breaker pool
- `NewRedisRateLimiterLibrary(client, opts ...ResilienceLibraryOption)` - Rate limiter

#### Options
- `WithCircuitBreakerSettings(failureThreshold, recoveryTimeout, timeout)` - Circuit breaker config
- `WithRateLimit(requests, window)` - Rate limiting config
- `WithRetry(maxRetries, initialDelay, maxDelay, backoffFactor)` - Retry config

#### Methods
- `ExecuteWithProtection(ctx, key, operation)` - Execute with all protections
- `CheckRateLimit(ctx, key)` - Check rate limit
- `GetRemainingRateLimit(ctx, key)` - Get remaining requests
- `GetCircuitBreakerStats()` - Get circuit breaker statistics
- `ResetCircuitBreaker(key)` - Reset specific circuit breaker

## 🔧 Configuration

### Environment Variables

The library can be configured using environment variables when used with the full gateway:

```bash
# Blockchain clients
ETH_RPC_URL=https://ethereum.publicnode.com
POLYGON_RPC_URL=https://polygon-bor-rpc.publicnode.com
BTC_RPC_URL=https://btc.getblock.io/mainnet

# Cache
REDIS_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379

# Timeouts
REQUEST_TIMEOUT=30
```

## 🧪 Testing

Run tests for individual modules:

```bash
# Test blockchain library
go test ./pkg/blockchain/...

# Test cache library
go test ./pkg/cache/...

# Test resilience library
go test ./pkg/resilience/...
```

## 📝 Examples

See the `examples/` directory for complete, runnable examples:

- `examples/blockchain/` - Blockchain client examples
- `examples/cache/` - Cache usage examples
- `examples/resilience/` - Resilience pattern examples
- `examples/integration/` - Combined usage examples

## 🤝 Contributing

When contributing to the library:

1. Ensure all library constructors are properly documented
2. Add examples for new features
3. Maintain backward compatibility
4. Follow Go conventions and best practices
5. Add tests for new functionality

## 📄 License

This library is licensed under the MIT License - see the LICENSE file for details.