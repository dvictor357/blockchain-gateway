package api

import (
	"io"
	"log"
	"net/http"
	"testing"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestHandler_GetCoinMarkets_Disabled(t *testing.T) {
	handler := NewHandler(nil, log.New(io.Discard, "", 0), nil, config.NewAPIConfig())

	for _, query := range []string{"", "?limit=invalid", "?offset=-1"} {
		t.Run(query, func(t *testing.T) {
			response := serveHandler(http.MethodGet, "/markets", "/markets"+query, "", handler.GetCoinMarkets)

			assert.Equal(t, http.StatusServiceUnavailable, response.Code)
			assert.Equal(t, "MARKET_DATA_DISABLED", errorCode(t, response))
		})
	}
}
