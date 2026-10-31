package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dvictor357/blaze"
)

// APIKeyAuth protects every HTTP path except the exact liveness endpoint.
// Keys are accepted only in one X-API-Key header, never in the URL.
func APIKeyAuth(apiKey string) blaze.MiddlewareFunc {
	expected := sha256.Sum256([]byte(apiKey))
	return func(next blaze.HandlerFunc) blaze.HandlerFunc {
		return func(c *blaze.Context) error {
			if apiKey == "" || c.Request.URL.Path == "/health" {
				return next(c)
			}
			values := c.Request.Header.Values("X-API-Key")
			if len(values) == 1 {
				actual := sha256.Sum256([]byte(values[0]))
				if subtle.ConstantTimeCompare(actual[:], expected[:]) == 1 {
					return next(c)
				}
			}
			return c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: "A valid X-API-Key header is required",
				Code:  "UNAUTHORIZED",
			})
		}
	}
}

func RateLimit(rateLimit int) blaze.MiddlewareFunc {
	const perMinute = time.Minute

	type rateLimitEntry struct {
		count     int
		lastReset time.Time
	}

	var (
		rateLimitMap = make(map[string]rateLimitEntry)
		mutex        sync.Mutex
		lastSweep    time.Time
	)

	return func(next blaze.HandlerFunc) blaze.HandlerFunc {
		return func(c *blaze.Context) error {
			ip := peerIP(c.Request)

			mutex.Lock()
			currentTime := time.Now()
			if currentTime.Sub(lastSweep) >= perMinute {
				for peer, entry := range rateLimitMap {
					if currentTime.Sub(entry.lastReset) > perMinute {
						delete(rateLimitMap, peer)
					}
				}
				lastSweep = currentTime
			}

			entry, exists := rateLimitMap[ip]
			if !exists || currentTime.Sub(entry.lastReset) > perMinute {
				rateLimitMap[ip] = rateLimitEntry{count: 1, lastReset: currentTime}
				mutex.Unlock()
				return next(c)
			}

			if entry.count >= rateLimit {
				mutex.Unlock()
				return c.JSON(http.StatusTooManyRequests, struct {
					Error string `json:"error"`
				}{Error: "Rate limit exceeded. Please try again later."})
			}

			entry.count++
			rateLimitMap[ip] = entry
			mutex.Unlock()
			return next(c)
		}
	}
}

func LoggingMiddleware(logger *log.Logger) blaze.MiddlewareFunc {
	return func(next blaze.HandlerFunc) blaze.HandlerFunc {
		return func(c *blaze.Context) error {
			start := time.Now()
			writer := &statusResponseWriter{ResponseWriter: c.ResponseWriter}
			c.ResponseWriter = writer

			err := next(c)

			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.Printf(
				"[%s] %s %s %d %s",
				c.Request.Method,
				c.Request.URL.Path,
				peerIP(c.Request),
				status,
				time.Since(start),
			)
			return err
		}
	}
}

func peerIP(request *http.Request) string {
	ip, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return ip
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.ResponseWriter.WriteHeader(status)
	if status >= 200 {
		w.status = status
	}
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
