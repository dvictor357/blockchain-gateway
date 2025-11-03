package middleware

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/resilience"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RateLimiter interface abstracts different rate limiter implementations
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	GetRemaining(ctx context.Context, key string) (int, error)
}

// RedisRateLimitMiddleware creates a rate limiting middleware using Redis
func RedisRateLimitMiddleware(redisClient *redis.Client, rateLimitConfig config.ServerConfig) gin.HandlerFunc {
	limiter := resilience.NewSlidingWindowRateLimiter(
		redisClient,
		resilience.RateLimitConfig{
			Requests: rateLimitConfig.RateLimit,
			Window:   time.Minute,
		},
	)

	return func(c *gin.Context) {
		// Generate unique request ID for tracing
		requestID := uuid.New().String()
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		// Get client IP
		clientIP := c.ClientIP()

		// Check rate limit
		ctx := c.Request.Context()
		allowed, err := limiter.Allow(ctx, clientIP)

		if err != nil {
			log.Printf("Rate limiter error: %v", err)
			// In case of Redis error, allow the request but log it
			c.Next()
			return
		}

		if !allowed {
			// Get remaining count for response headers
			remaining, _ := limiter.GetRemaining(ctx, clientIP)

			c.Header("X-RateLimit-Limit", strconv.Itoa(rateLimitConfig.RateLimit))
			c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.Header("X-RateLimit-Reset", strconv.Itoa(int(time.Now().Add(time.Minute).Unix())))

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"code":    "RATE_LIMIT_EXCEEDED",
				"message": "Too many requests. Please try again later.",
				"limit":   rateLimitConfig.RateLimit,
				"reset":   time.Now().Add(time.Minute).Unix(),
			})
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(rateLimitConfig.RateLimit))
		if remaining, err := limiter.GetRemaining(ctx, clientIP); err == nil {
			c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		}

		c.Next()
	}
}

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request ID already exists (from load balancer)
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		c.Next()
	}
}

// RateLimitConfig holds the rate limiting configuration
type RateLimitConfig struct {
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
	Burst    int           `json:"burst"`
	Enabled  bool          `json:"enabled"`
}

// GlobalRateLimiter variable to hold the rate limiter instance
var GlobalRateLimiter RateLimiter

// SetRateLimiter allows setting a custom rate limiter (useful for testing)
func SetRateLimiter(limiter RateLimiter) {
	GlobalRateLimiter = limiter
}

// RateLimitMiddleware that adapts to Redis configuration
func RateLimitMiddleware(config config.RedisConfig, serverConfig config.ServerConfig, redisClient *redis.Client) gin.HandlerFunc {
	// If Redis is not enabled, skip rate limiting
	if !config.Enabled {
		return func(c *gin.Context) {
			c.Header("X-RateLimit-Limit", strconv.Itoa(serverConfig.RateLimit))
			c.Header("X-RateLimit-Remaining", strconv.Itoa(serverConfig.RateLimit))
			c.Next()
		}
	}

	// Use Redis-based rate limiting
	return RedisRateLimitMiddleware(redisClient, serverConfig)
}

// AdaptiveRateLimitMiddleware implements adaptive rate limiting based on system load
func AdaptiveRateLimitMiddleware(redisClient *redis.Client, baseConfig config.ServerConfig) gin.HandlerFunc {
	limiter := resilience.NewSlidingWindowRateLimiter(
		redisClient,
		resilience.RateLimitConfig{
			Requests: baseConfig.RateLimit,
			Window:   time.Minute,
		},
	)

	return func(c *gin.Context) {
		requestID := uuid.New().String()
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		clientIP := c.ClientIP()
		ctx := c.Request.Context()

		// Check if this is a premium API key (implement later)
		isPremium := false // TODO: Check API key tier

		// Adjust rate limit based on tier
		rateLimit := baseConfig.RateLimit
		if isPremium {
			rateLimit = rateLimit * 2 // Premium users get double the rate limit
		}

		// For now, just use the base rate limiter
		allowed, err := limiter.Allow(ctx, clientIP)

		if err != nil {
			log.Printf("Rate limiter error: %v", err)
			c.Next()
			return
		}

		if !allowed {
			// Calculate retry-after header
			retryAfter := int(time.Minute.Seconds())

			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Limit", strconv.Itoa(rateLimit))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.Itoa(int(time.Now().Add(time.Minute).Unix())))

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"code":    "RATE_LIMIT_EXCEEDED",
				"message": "Too many requests. Please try again later.",
				"limit":   rateLimit,
				"reset":   time.Now().Add(time.Minute).Unix(),
			})
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(rateLimit))
		c.Next()
	}
}
