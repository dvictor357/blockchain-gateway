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
	fmt.Println("=== Cache Library Example ===")

	ctx := context.Background()

	// Example 1: Memory-only cache
	fmt.Println("\n1. Memory-only Cache:")
	memCache := cache.NewSimpleCache(
		cache.WithMemoryCache(100, 5*time.Minute),
		cache.WithStats(true),
	)

	// Store and retrieve values
	err := memCache.Set(ctx, "user:123", []byte(`{"name":"Alice","age":30}`))
	if err != nil {
		log.Fatal(err)
	}

	value, err := memCache.Get(ctx, "user:123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("User data: %s\n", string(value))

	// Example 2: Get or Fetch pattern
	fmt.Println("\n2. Get or Fetch Pattern:")
	result, err := memCache.GetOrFetch(ctx, "expensive_computation", func(ctx context.Context) ([]byte, error) {
		fmt.Println("  -> Performing expensive computation...")
		time.Sleep(100 * time.Millisecond) // Simulate work
		return []byte("computation_result"), nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Result: %s\n", string(result))

	// Second call should hit cache
	result, err = memCache.GetOrFetch(ctx, "expensive_computation", func(ctx context.Context) ([]byte, error) {
		fmt.Println("  -> This should not be called (cache hit)...")
		return []byte("should_not_happen"), nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cached result: %s\n", string(result))

	// Example 3: Cache statistics
	fmt.Println("\n3. Cache Statistics:")
	stats, err := memCache.GetStats(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Memory cache stats: %+v\n", stats)

	// Example 4: Redis cache (if Redis is available)
	fmt.Println("\n4. Redis Cache (will skip if Redis not available):")

	// Try to create Redis cache (will fail if Redis is not running)
	redisConfig := config.RedisConfig{
		Host:    "localhost",
		Port:    "6379",
		Enabled: true,
	}

	redisCache := cache.NewSimpleCache(
		cache.WithRedisCache(redisConfig, 30*time.Minute),
		cache.WithMemoryCache(50, 5*time.Minute), // L1 cache
		cache.WithStats(true),
	)

	// Try to use Redis cache (will work even if Redis is not available, just without L2)
	fmt.Println("Testing multi-layer cache (will work with memory cache even if Redis unavailable)")

	// Store in multi-layer cache
	err = redisCache.Set(ctx, "product:456", []byte(`{"name":"Widget","price":19.99}`))
	if err != nil {
		log.Printf("Cache set error: %v\n", err)
	} else {
		fmt.Println("Stored in multi-layer cache successfully")

		// Retrieve from multi-layer cache
		productValue, err := redisCache.Get(ctx, "product:456")
		if err != nil {
			log.Printf("Cache get error: %v\n", err)
		} else {
			fmt.Printf("Product data: %s\n", string(productValue))
		}

		// Get multi-layer cache stats
		redisStats, err := redisCache.GetStats(ctx)
		if err != nil {
			log.Printf("Cache stats error: %v\n", err)
		} else {
			fmt.Printf("Multi-layer cache stats: %+v\n", redisStats)
		}
	}

	// Example 5: Cache operations
	fmt.Println("\n5. Cache Operations:")

	// Set with custom TTL
	err = memCache.SetWithTTL(ctx, "temp_data", []byte("expires_soon"), 2*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Set temporary data with 2-second TTL")

	// Retrieve immediately
	tempValue, err := memCache.Get(ctx, "temp_data")
	if err != nil {
		fmt.Printf("Error getting temp data: %v\n", err)
	} else {
		fmt.Printf("Temp data: %s\n", string(tempValue))
	}

	// Wait for expiration
	fmt.Println("Waiting 3 seconds for data to expire...")
	time.Sleep(3 * time.Second)

	// Try to retrieve expired data
	_, err = memCache.Get(ctx, "temp_data")
	if err != nil {
		fmt.Printf("Temp data expired as expected: %v\n", err)
	}

	// Example 6: Cache clearing
	fmt.Println("\n6. Cache Clearing:")

	// Add some data
	memCache.Set(ctx, "key1", []byte("value1"))
	memCache.Set(ctx, "key2", []byte("value2"))
	memCache.Set(ctx, "key3", []byte("value3"))

	// Delete specific key
	err = memCache.Delete(ctx, "key2")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Deleted key2")

	// Clear all cache
	err = memCache.Clear(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Cleared all cache")

	// Verify cache is empty
	_, err = memCache.Get(ctx, "key1")
	if err != nil {
		fmt.Printf("Cache cleared successfully (key1 not found): %v\n", err)
	}

	fmt.Println("\n=== Cache Example completed ===")
}
