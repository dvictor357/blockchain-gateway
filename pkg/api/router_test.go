package api

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestRouterLimitsUnknownRoutes(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	router := NewRouter(NewHandler(nil, logger, nil, config.NewAPIConfig()), logger, config.ServerConfig{RateLimit: 1})

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	assert.Equal(t, http.StatusNotFound, missing.Code)

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusTooManyRequests, health.Code)
	assert.JSONEq(t, `{"error":"Rate limit exceeded. Please try again later."}`, health.Body.String())
}
