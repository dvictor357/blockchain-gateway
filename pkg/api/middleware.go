package api

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func RateLimit(rateLimit int) gin.HandlerFunc {

	// Implement rate limiting logic here
	// For now, we'll use a simple in-memory map to track requests per IP
	// In production, consider using a distributed rate limiter like redis

	perMinute := 60 * time.Second

	// Use a mutex-protected map to store request counts and timestamps
	type rateLimitEntry struct {
		count     int
		lastReset time.Time
	}

	var (
		rateLimitMap = make(map[string]*rateLimitEntry)
		mutex        sync.Mutex
	)

	mutex.Lock()
	defer mutex.Unlock()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		// Get or initialize the entry for this IP
		entry, exists := rateLimitMap[ip]
		currentTime := time.Now()

		if !exists {
			rateLimitMap[ip] = &rateLimitEntry{
				count:     1,
				lastReset: currentTime,
			}
			c.Next()
			return
		}

		// Reset count if more than a minute has passed
		if currentTime.Sub(entry.lastReset) > perMinute {
			entry.count = 1
			entry.lastReset = currentTime
			c.Next()
			return
		}

		// Check if rate limit is exceeded
		if entry.count >= rateLimit {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			return
		}

		// Increment count and allow request
		entry.count++
		c.Next()
	}
}

func LoggingMiddleware(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Log request after completion
		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Printf(
			"[%s] %s %s %d %s",
			c.Request.Method,
			c.Request.URL.Path,
			c.ClientIP(),
			status,
			latency,
		)
	}
}
