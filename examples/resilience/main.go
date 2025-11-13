package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/resilience"
	"github.com/redis/go-redis/v9"
)

func main() {
	fmt.Println("=== Resilience Library Example ===")

	ctx := context.Background()

	// Example 1: Circuit Breaker
	fmt.Println("\n1. Circuit Breaker:")
	cb := resilience.NewCircuitBreakerLibrary(
		resilience.WithCircuitBreakerSettings(3, 5*time.Second, 2*time.Second),
	)

	// Simulate some operations that might fail
	for i := 0; i < 10; i++ {
		result, err := cb.Execute(func() (interface{}, error) {
			// Simulate operation that fails 70% of time
			if rand.Float32() < 0.7 {
				return nil, fmt.Errorf("operation failed")
			}
			return fmt.Sprintf("success_%d", i), nil
		})

		if err != nil {
			fmt.Printf("Operation %d failed: %v (State: %s)\n", i, err, cb.GetState())
		} else {
			fmt.Printf("Operation %d succeeded: %v (State: %s)\n", i, result, cb.GetState())
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Get circuit breaker stats
	stats := cb.GetStats()
	fmt.Printf("Circuit breaker stats: %+v\n", stats)

	// Example 2: Rate Limiting (requires Redis)
	fmt.Println("\n2. Rate Limiting:")

	// Try to create Redis client (will work even if Redis is not available)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	rateLimiter := resilience.NewRedisRateLimiterLibrary(
		redisClient,
		resilience.WithRateLimit(5, 10*time.Second),
	)

	// Test rate limiting
	for i := 0; i < 8; i++ {
		allowed, err := rateLimiter.Allow(ctx, "test_user")
		if err != nil {
			fmt.Printf("Rate limit check %d error: %v\n", i, err)
		} else if allowed {
			fmt.Printf("Request %d: ALLOWED\n", i)
		} else {
			fmt.Printf("Request %d: RATE LIMITED\n", i)
		}

		remaining, err := rateLimiter.GetRemaining(ctx, "test_user")
		if err == nil {
			fmt.Printf("  Remaining requests: %d\n", remaining)
		}
	}

	// Example 3: Retry with Exponential Backoff
	fmt.Println("\n3. Retry with Exponential Backoff:")

	retryConfig := resilience.DefaultRetryConfig()
	retryConfig.MaxRetries = 3
	retryConfig.InitialDelay = 100 * time.Millisecond
	retryConfig.MaxDelay = 2 * time.Second

	attempt := 0
	result, err := resilience.RetryWithResult(ctx, retryConfig, func() (interface{}, error) {
		attempt++
		fmt.Printf("  Attempt %d\n", attempt)

		// Fail first 2 attempts, succeed on 3rd
		if attempt < 3 {
			return nil, fmt.Errorf("temporary failure")
		}

		return "success after retries", nil
	})

	if err != nil {
		fmt.Printf("Retry failed: %v\n", err)
	} else {
		fmt.Printf("Retry succeeded: %v\n", result)
	}

	// Example 4: Simple Resilience Manager (all-in-one)
	fmt.Println("\n4. Simple Resilience Manager:")

	resilienceManager := resilience.NewSimpleResilienceManager(
		redisClient,
		resilience.WithCircuitBreakerSettings(5, 30*time.Second, 10*time.Second),
		resilience.WithRateLimit(10, time.Minute),
		resilience.WithRetry(2, 200*time.Millisecond, 3*time.Second, 2.0),
	)

	// Test protected operation
	for i := 0; i < 5; i++ {
		// Check rate limit first
		allowed, err := resilienceManager.CheckRateLimit(ctx, "api_user")
		if err != nil {
			fmt.Printf("Rate limit check error: %v\n", err)
			continue
		}

		if !allowed {
			fmt.Printf("Request %d: Rate limited\n", i)
			continue
		}

		// Execute with protection
		result, err := resilienceManager.ExecuteWithProtection(ctx, "external_api", func() (interface{}, error) {
			// Simulate external API call
			if rand.Float32() < 0.3 { // 30% failure rate
				return nil, fmt.Errorf("external API error")
			}

			time.Sleep(50 * time.Millisecond) // Simulate network latency
			return fmt.Sprintf("API response %d", i), nil
		})

		if err != nil {
			fmt.Printf("Request %d failed: %v\n", i, err)
		} else {
			fmt.Printf("Request %d succeeded: %v\n", i, result)
		}
	}

	// Get resilience manager stats
	cbStats := resilienceManager.GetCircuitBreakerStats()
	fmt.Printf("Circuit breaker pool stats: %+v\n", cbStats)

	// Example 5: Circuit Breaker Pool
	fmt.Println("\n5. Circuit Breaker Pool:")

	cbPool := resilience.NewCircuitBreakerPoolLibrary(
		resilience.WithCircuitBreakerSettings(3, 5*time.Second, 2*time.Second),
	)

	// Test multiple services with different circuit breakers
	services := []string{"user_service", "payment_service", "notification_service"}
	var wg sync.WaitGroup

	for _, service := range services {
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(svc string, req int) {
				defer wg.Done()

				result, err := cbPool.Execute(svc, func() (interface{}, error) {
					// Different failure rates for different services
					failureRate := map[string]float32{
						"user_service":         0.2,
						"payment_service":      0.5,
						"notification_service": 0.8,
					}[svc]

					if rand.Float32() < failureRate {
						return nil, fmt.Errorf("%s temporarily unavailable", svc)
					}

					return fmt.Sprintf("%s_response_%d", svc, req), nil
				})

				if err != nil {
					fmt.Printf("%s request %d failed: %v\n", svc, req, err)
				} else {
					fmt.Printf("%s request %d succeeded: %v\n", svc, req, result)
				}
			}(service, i)
		}
	}

	wg.Wait()

	// Get pool stats
	poolStats := cbPool.GetStats()
	fmt.Printf("Circuit breaker pool final stats: %+v\n", poolStats)

	fmt.Println("\n=== Resilience Example completed ===")
}
