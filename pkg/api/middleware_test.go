package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dvictor357/blaze"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitDoesNotSerializeDownstreamHandlers(t *testing.T) {
	engine := blaze.New()
	engine.Use(RateLimit(10))
	entered := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 2)
	engine.GET("/", func(c *blaze.Context) error {
		entered <- c.Request.RemoteAddr
		<-release
		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	started := 0
	defer func() {
		close(release)
		for range started {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("request failed to finish after downstream handlers were released")
			}
		}
	}()
	request := func(peer string) {
		started++
		go func() {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = peer
			engine.ServeHTTP(httptest.NewRecorder(), r)
			done <- struct{}{}
		}()
	}

	request("192.0.2.1:1000")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter its handler")
	}
	request("192.0.2.2:2000")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("a different peer was blocked behind a running downstream handler")
	}
}

func TestRateLimitIgnoresForwardedHeaders(t *testing.T) {
	engine := blaze.New()
	engine.Use(RateLimit(1))
	engine.GET("/", func(c *blaze.Context) error {
		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	for i, forwarded := range []string{"198.51.100.1", "198.51.100.2"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "192.0.2.1:1000"
		r.Header.Set("X-Forwarded-For", forwarded)
		r.Header.Set("X-Real-IP", forwarded)
		r.Header.Set("Forwarded", "for="+forwarded)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)

		if i == 0 {
			assert.Equal(t, http.StatusOK, w.Code)
		} else {
			assert.Equal(t, http.StatusTooManyRequests, w.Code)
			assert.JSONEq(t, `{"error":"Rate limit exceeded. Please try again later."}`, w.Body.String())
		}
	}
}

func TestRateLimitKeepsIndependentPeerCounters(t *testing.T) {
	tests := []struct {
		name  string
		peers [4]string
	}{
		{
			name:  "IPv4 peers ignore source ports",
			peers: [4]string{"192.0.2.1:1000", "192.0.2.1:1001", "192.0.2.2:2000", "192.0.2.2:2001"},
		},
		{
			name:  "IPv6 peers ignore source ports",
			peers: [4]string{"[2001:db8::1]:1000", "[2001:db8::1]:1001", "[2001:db8::2]:2000", "[2001:db8::2]:2001"},
		},
		{
			name:  "synthetic peers without ports",
			peers: [4]string{"192.0.2.1", "192.0.2.1", "192.0.2.2", "192.0.2.2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := blaze.New()
			engine.Use(RateLimit(1))
			engine.GET("/", func(c *blaze.Context) error {
				return c.JSON(http.StatusOK, map[string]bool{"ok": true})
			})

			for i, peer := range tt.peers {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.RemoteAddr = peer
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, r)
				expected := http.StatusOK
				if i%2 == 1 {
					expected = http.StatusTooManyRequests
				}
				assert.Equal(t, expected, w.Code, "peer %s", peer)
			}
		})
	}
}

func TestLoggingMiddlewareTracksWrittenStatus(t *testing.T) {
	tests := []struct {
		name    string
		handler blaze.HandlerFunc
		status  int
	}{
		{
			name: "Blaze JSON response",
			handler: func(c *blaze.Context) error {
				return c.JSON(http.StatusAccepted, map[string]bool{"ok": true})
			},
			status: http.StatusAccepted,
		},
		{
			name: "implicit successful body write",
			handler: func(c *blaze.Context) error {
				_, err := c.ResponseWriter.Write([]byte("ok"))
				return err
			},
			status: http.StatusOK,
		},
		{
			name: "first final status wins",
			handler: func(c *blaze.Context) error {
				c.ResponseWriter.WriteHeader(http.StatusCreated)
				c.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
				return nil
			},
			status: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			engine := blaze.New()
			engine.Use(LoggingMiddleware(log.New(&logs, "", 0)))
			engine.GET("/logged", tt.handler)
			r := httptest.NewRequest(http.MethodGet, "/logged", nil)
			r.RemoteAddr = "192.0.2.1:1000"
			r.Header.Set("X-Forwarded-For", "198.51.100.1")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)

			assert.Equal(t, tt.status, w.Code)
			fields := strings.Fields(logs.String())
			if len(fields) != 5 {
				t.Fatalf("expected method, path, peer, status, and latency fields; got %q", logs.String())
			}
			assert.Contains(t, fields[0], http.MethodGet)
			assert.Equal(t, "/logged", fields[1])
			assert.Equal(t, "192.0.2.1", fields[2])
			assert.Equal(t, strconv.Itoa(tt.status), fields[3])
			_, err := time.ParseDuration(fields[4])
			assert.NoError(t, err)
		})
	}
}
