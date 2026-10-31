package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dvictor357/blaze"
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const blockNumberRequest = `{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`

// serveBody serves a request whose declared Content-Length is controlled by the caller;
// -1 simulates a chunked or otherwise unknown-length body.
func serveBody(route, target string, body io.Reader, contentLength int64, handler blaze.HandlerFunc) *httptest.ResponseRecorder {
	router := blaze.New()
	router.Handle(http.MethodPost, route, handler)
	request := httptest.NewRequest(http.MethodPost, target, body)
	request.ContentLength = contentLength
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, request)
	return writer
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var response ErrorResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response.Code
}

func TestHandler_RequestBodyLimit(t *testing.T) {
	const padding = 16
	limit := int64(len(blockNumberRequest) + padding)
	exact := blockNumberRequest + strings.Repeat(" ", padding)
	over := exact + " "

	tests := []struct {
		name          string
		body          string
		contentLength int64
		wantStatus    int
		wantCode      string
		wantExecuted  bool
	}{
		{name: "exactly at the limit", body: exact, contentLength: int64(len(exact)), wantStatus: http.StatusOK, wantExecuted: true},
		{name: "exactly at the limit, unknown length", body: exact, contentLength: -1, wantStatus: http.StatusOK, wantExecuted: true},
		{name: "one byte over, declared length", body: over, contentLength: int64(len(over)), wantStatus: http.StatusRequestEntityTooLarge, wantCode: "REQUEST_TOO_LARGE"},
		{name: "one byte over, unknown length", body: over, contentLength: -1, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "REQUEST_TOO_LARGE"},
		{
			name:          "small valid JSON followed by oversized trailing whitespace",
			body:          blockNumberRequest + strings.Repeat(" ", 4096),
			contentLength: -1,
			wantStatus:    http.StatusRequestEntityTooLarge,
			wantCode:      "REQUEST_TOO_LARGE",
		},
		{
			name:          "small valid JSON followed by trailing garbage",
			body:          blockNumberRequest + " garbage",
			contentLength: -1,
			wantStatus:    http.StatusBadRequest,
			wantCode:      "INVALID_JSON",
		},
		{
			name:          "second JSON value",
			body:          blockNumberRequest + `{}`, // fits within the limit, so only the trailing value is wrong
			contentLength: -1,
			wantStatus:    http.StatusBadRequest,
			wantCode:      "INVALID_JSON",
		},
		{
			name:          "incomplete JSON larger than the limit",
			body:          `{"jsonrpc":"2.0","method":"` + strings.Repeat("a", 4096),
			contentLength: -1,
			wantStatus:    http.StatusRequestEntityTooLarge,
			wantCode:      "REQUEST_TOO_LARGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, manager, _ := setupTestHandler()
			handler.apiConfig.MaxRequestBodyBytes = limit
			if tt.wantExecuted {
				manager.On("Execute", mock.Anything, "ethereum", "eth_blockNumber", []interface{}{}).
					Return(json.RawMessage(`"0x1"`), nil).Once()
			}

			w := serveBody("/chains/{chain}/query", "/chains/ethereum/query",
				strings.NewReader(tt.body), tt.contentLength, handler.QueryChain)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, errorCode(t, w))
			}
			manager.AssertExpectations(t)
		})
	}
}

func TestHandler_RequestBodyLimit_Batch(t *testing.T) {
	handler, manager, _ := setupTestHandler()
	handler.apiConfig.MaxRequestBodyBytes = 64

	body := `{"ethereum":[` + blockNumberRequest + `]}`
	w := serveBody("/batch", "/batch", strings.NewReader(body), -1, handler.BatchQuery)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Equal(t, "REQUEST_TOO_LARGE", errorCode(t, w))
	manager.AssertExpectations(t) // BatchExecute was never reached
}

func batchBody(perChain map[string]int) string {
	chains := make([]string, 0, len(perChain))
	for chain, count := range perChain {
		requests := make([]string, count)
		for i := range requests {
			requests[i] = fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":%d}`, i+1)
		}
		chains = append(chains, fmt.Sprintf("%q:[%s]", chain, strings.Join(requests, ",")))
	}
	return "{" + strings.Join(chains, ",") + "}"
}

func TestHandler_BatchQuery_TotalRequestLimit(t *testing.T) {
	tests := []struct {
		name       string
		perChain   map[string]int
		wantStatus int
		wantCode   string
	}{
		{name: "limit on one chain", perChain: map[string]int{"ethereum": 10}, wantStatus: http.StatusOK},
		{name: "limit across chains", perChain: map[string]int{"ethereum": 4, "polygon": 3, "bitcoin": 3}, wantStatus: http.StatusOK},
		{name: "limit+1 on one chain", perChain: map[string]int{"ethereum": 11}, wantStatus: http.StatusBadRequest, wantCode: "BATCH_TOO_LARGE"},
		{name: "limit+1 across chains", perChain: map[string]int{"ethereum": 6, "polygon": 5}, wantStatus: http.StatusBadRequest, wantCode: "BATCH_TOO_LARGE"},
		{name: "chain with no requests", perChain: map[string]int{"ethereum": 0}, wantStatus: http.StatusBadRequest, wantCode: "EMPTY_BATCH"},
		{name: "all chains empty", perChain: map[string]int{"ethereum": 0, "polygon": 0}, wantStatus: http.StatusBadRequest, wantCode: "EMPTY_BATCH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, manager, _ := setupTestHandler()
			if tt.wantStatus == http.StatusOK {
				// Any BatchExecute call is a success here; rejected batches must not reach it
				// (an unexpected call on the mock fails the test).
				manager.On("BatchExecute", mock.Anything, mock.Anything).
					Return(map[string][]blockchain.RPCResponse{}, nil).Once()
			}

			w := serveHandler(http.MethodPost, "/batch", "/batch", batchBody(tt.perChain), handler.BatchQuery)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, errorCode(t, w))
			}
			manager.AssertExpectations(t)
		})
	}
}

func TestHandler_BatchQuery_NullChainRequests(t *testing.T) {
	handler, manager, _ := setupTestHandler()

	w := serveHandler(http.MethodPost, "/batch", "/batch", `{"ethereum":null}`, handler.BatchQuery)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "EMPTY_BATCH", errorCode(t, w))
	manager.AssertExpectations(t)
}

func TestHandler_GetCoinMarkets_OrderByIsCaseInsensitive(t *testing.T) {
	handler, _, marketService := setupTestHandler()
	marketService.On("GetMarketDataFromDB", mock.Anything, handler.apiConfig.DefaultPageLimit, 0, "current_price", "desc").
		Return([]models.CoinMarket{}, 0, time.Time{}, nil).Once()

	w := serveHandler(http.MethodGet, "/markets", "/markets?orderBy=Current_Price&sortDirection=desc", "", handler.GetCoinMarkets)

	assert.Equal(t, http.StatusOK, w.Code)
	marketService.AssertExpectations(t)
}

func TestHandler_GetCoinMarkets_BoundedByRequestTimeout(t *testing.T) {
	handler, _, marketService := setupTestHandler()
	hasDeadline := mock.MatchedBy(func(ctx context.Context) bool {
		_, ok := ctx.Deadline()
		return ok
	})
	marketService.On("GetMarketDataFromDB", hasDeadline, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, 0, time.Time{}, fmt.Errorf("query: %w", context.DeadlineExceeded)).Once()

	w := serveHandler(http.MethodGet, "/markets", "/markets", "", handler.GetCoinMarkets)

	assert.Equal(t, http.StatusGatewayTimeout, w.Code)
	assert.Equal(t, "REQUEST_TIMEOUT", errorCode(t, w))
	marketService.AssertExpectations(t)
}
