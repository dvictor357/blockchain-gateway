package resilience

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	Requests int           // Maximum requests allowed
	Window   time.Duration // Time window for requests
	Burst    int           // Burst allowance (optional)
}

// RedisRateLimiter implements a distributed rate limiter using Redis
type RedisRateLimiter struct {
	client *redis.Client
	config RateLimitConfig
}

// NewRedisRateLimiter creates a new Redis-based rate limiter
func NewRedisRateLimiter(client *redis.Client, config RateLimitConfig) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		config: config,
	}
}

// Allow checks if a request is allowed under the rate limit
// Returns true if the request should be allowed, false otherwise
func (rl *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	// Use Redis sliding window or token bucket algorithm
	// We'll use a simple sliding window approach

	pipe := rl.client.Pipeline()
	now := time.Now().Unix()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, rl.getKey(key), "-inf", fmt.Sprintf("%d", rl.getWindowStart(now)))

	// Count current requests
	pipe.ZCard(ctx, rl.getKey(key))

	// Add current request
	pipe.ZAdd(ctx, rl.getKey(key), redis.Z{
		Score:  float64(now),
		Member: key,
	})

	// Set expiration for the sorted set
	pipe.Expire(ctx, rl.getKey(key), rl.config.Window)

	// Execute pipeline
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// Get the count from the second command
	countCmd := cmds[1]
	currentCount, err := countCmd.(*redis.IntCmd).Result()
	if err != nil {
		return false, err
	}

	// Check if we're within limits
	// Note: We're counting after adding, so check if count <= Requests
	return currentCount <= int64(rl.config.Requests), nil
}

// getKey generates the Redis key for rate limiting
func (rl *RedisRateLimiter) getKey(key string) string {
	return "rate_limit:" + key
}

// getWindowStart calculates the start of the time window
func (rl *RedisRateLimiter) getWindowStart(nowUnix int64) int64 {
	return nowUnix - int64(rl.config.Window.Seconds())
}

// GetRemaining returns the number of remaining requests
func (rl *RedisRateLimiter) GetRemaining(ctx context.Context, key string) (int, error) {
	now := time.Now().Unix()

	// Remove old entries
	rl.client.ZRemRangeByScore(ctx, rl.getKey(key), "-inf", fmt.Sprintf("%d", rl.getWindowStart(now)))

	// Count current requests
	countCmd := rl.client.ZCard(ctx, rl.getKey(key))
	count, err := countCmd.Result()
	if err != nil {
		return 0, err
	}

	remaining := max(rl.config.Requests-int(count), 0)

	return remaining, nil
}

// Reset removes all rate limit entries for a key
func (rl *RedisRateLimiter) Reset(ctx context.Context, key string) error {
	_, err := rl.client.Del(ctx, rl.getKey(key)).Result()
	return err
}

// SlidingWindowRateLimiter implements a true sliding window rate limiter
type SlidingWindowRateLimiter struct {
	client *redis.Client
	config RateLimitConfig
}

// NewSlidingWindowRateLimiter creates a new sliding window rate limiter
func NewSlidingWindowRateLimiter(client *redis.Client, config RateLimitConfig) *SlidingWindowRateLimiter {
	return &SlidingWindowRateLimiter{
		client: client,
		config: config,
	}
}

// Allow implements the sliding window rate limiting algorithm
func (rl *SlidingWindowRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-rl.config.Window)

	pipe := rl.client.Pipeline()

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, rl.getKey(key), "-inf", fmt.Sprintf("%d", windowStart.Unix()))

	// Count entries in the window
	pipe.ZCard(ctx, rl.getKey(key))

	// Add current request if under limit
	pipe.ZAdd(ctx, rl.getKey(key), redis.Z{
		Score:  float64(now.Unix()),
		Member: key,
	})

	// Set expiration
	pipe.Expire(ctx, rl.getKey(key), rl.config.Window*2)

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// Get count from second command
	countCmd := cmds[1]
	currentCount, err := countCmd.(*redis.IntCmd).Result()
	if err != nil {
		return false, err
	}

	return currentCount < int64(rl.config.Requests), nil
}

func (rl *SlidingWindowRateLimiter) getKey(key string) string {
	return "sliding_window:" + key
}

// GetRemaining returns the number of remaining requests
func (rl *SlidingWindowRateLimiter) GetRemaining(ctx context.Context, key string) (int, error) {
	now := time.Now()
	windowStart := now.Add(-rl.config.Window)

	// Remove old entries
	rl.client.ZRemRangeByScore(ctx, rl.getKey(key), "-inf", fmt.Sprintf("%d", windowStart.Unix()))

	// Count current requests
	count, err := rl.client.ZCard(ctx, rl.getKey(key)).Result()
	if err != nil {
		return 0, err
	}

	remaining := max(rl.config.Requests-int(count), 0)

	return remaining, nil
}

// FixedWindowRateLimiter implements a simpler fixed window rate limiter
type FixedWindowRateLimiter struct {
	client *redis.Client
	config RateLimitConfig
}

// NewFixedWindowRateLimiter creates a new fixed window rate limiter
func NewFixedWindowRateLimiter(client *redis.Client, config RateLimitConfig) *FixedWindowRateLimiter {
	return &FixedWindowRateLimiter{
		client: client,
		config: config,
	}
}

// Allow implements fixed window rate limiting
func (rl *FixedWindowRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	window := rl.getCurrentWindow()
	counterKey := rl.getCounterKey(key, window)

	pipe := rl.client.Pipeline()

	// Get current count
	pipe.Get(ctx, counterKey)

	// Increment counter
	pipe.Incr(ctx, counterKey)

	// Set expiration on first increment
	pipe.Expire(ctx, counterKey, rl.config.Window)

	cmds, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return false, err
	}

	var count int64
	if len(cmds) > 0 {
		countCmd := cmds[0]
		val, err := countCmd.(*redis.IntCmd).Result()
		if err == nil {
			count = val
		}
	}

	return count <= int64(rl.config.Requests), nil
}

func (rl *FixedWindowRateLimiter) getCurrentWindow() int64 {
	now := time.Now()
	return now.Unix() / int64(rl.config.Window.Seconds())
}

func (rl *FixedWindowRateLimiter) getCounterKey(key string, window int64) string {
	return "fixed_window:" + key + ":" + string(rune(window))
}

// GetRemaining returns the number of remaining requests
func (rl *FixedWindowRateLimiter) GetRemaining(ctx context.Context, key string) (int, error) {
	window := rl.getCurrentWindow()
	counterKey := rl.getCounterKey(key, window)

	count, err := rl.client.Get(ctx, counterKey).Int64()
	if err == redis.Nil {
		return rl.config.Requests, nil
	}
	if err != nil {
		return 0, err
	}

	remaining := max(rl.config.Requests-int(count), 0)

	return remaining, nil
}
