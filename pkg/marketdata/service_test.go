package marketdata

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/coingecko"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRepo is a MarketRepository that records upserts; it serves no reads.
type recordingRepo struct {
	upserts atomic.Int64
}

func (r *recordingRepo) UpsertCoinMarkets(context.Context, []models.CoinMarket) error {
	r.upserts.Add(1)
	return nil
}

func (r *recordingRepo) GetCoinMarkets(context.Context, int, int, string, string) ([]models.CoinMarket, int, time.Time, error) {
	return nil, 0, time.Time{}, nil
}

func (r *recordingRepo) GetLatestDataFetchedTimestamp(context.Context) (time.Time, error) {
	return time.Time{}, nil
}

func newTestService(t *testing.T, handler http.HandlerFunc, fetchTimeout time.Duration) (*Service, *recordingRepo) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	repo := &recordingRepo{}
	service := NewService(log.New(io.Discard, "", 0), coingecko.NewClient(nil, server.URL), repo,
		ServiceConfig{CoinGeckoVsCurrency: "usd", CoinGeckoPerPage: 1, CoinGeckoPage: 1, FetchTimeout: fetchTimeout})
	return service, repo
}

func TestRunFetchJob_HasADeadline(t *testing.T) {
	release := make(chan struct{})
	sawCancel := make(chan struct{})
	service, repo := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			close(sawCancel)
		case <-release:
		}
	}, 50*time.Millisecond)
	t.Cleanup(func() { close(release) })

	// The upstream never answers; the job must give up on its own deadline.
	service.runFetchJob(context.Background())

	select {
	case <-sawCancel:
	case <-time.After(10 * time.Second):
		t.Fatal("the in-flight CoinGecko request was not cancelled when the job deadline expired")
	}
	assert.Zero(t, repo.upserts.Load())
}

func TestRunFetchJob_StoresFetchedMarkets(t *testing.T) {
	service, repo := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"bitcoin","symbol":"btc","name":"Bitcoin"}]`)
	}, time.Minute)

	service.runFetchJob(context.Background())

	assert.EqualValues(t, 1, repo.upserts.Load())
}

func TestRunFetchJob_SkipsWorkOnceTheServiceIsStopping(t *testing.T) {
	var requests atomic.Int64
	service, repo := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `[]`)
	}, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service.runFetchJob(ctx)

	assert.Zero(t, requests.Load())
	assert.Zero(t, repo.upserts.Load())
}

func TestStartFetchingLoop_ReturnsOnceCancelled(t *testing.T) {
	service, _ := newTestService(t, func(w http.ResponseWriter, r *http.Request) {}, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.StartFetchingLoop(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		require.Fail(t, "StartFetchingLoop did not return after cancellation")
	}
}
