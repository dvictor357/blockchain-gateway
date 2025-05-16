package marketdata

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/user/blockchain-gateway/pkg/coingecko"
	"github.com/user/blockchain-gateway/pkg/models"
)

// ServiceConfig holds configuration for the MarketDataService
type ServiceConfig struct {
	CoinGeckoVsCurrency      string
	CoinGeckoOrder           string
	CoinGeckoPerPage         int
	CoinGeckoPage            int
	CoinGeckoSparkline       bool
	CoinGeckoPriceChangePerc []string // e.g., ["1h", "24h", "7d"]
}

// Service orchestrates fetching and storing of market data.
type Service struct {
	logger   *log.Logger
	cgClient *coingecko.Client
	repo     MarketRepository
	config   ServiceConfig
	cron     *cron.Cron
	stopChan chan struct{}
}

// NewService creates a new MarketDataService.
func NewService(logger *log.Logger, cgClient *coingecko.Client, repo MarketRepository, config ServiceConfig) *Service {
	c := cron.New(cron.WithSeconds())

	return &Service{
		logger:   logger,
		cgClient: cgClient,
		repo:     repo,
		config:   config,
		cron:     c,
		stopChan: make(chan struct{}),
	}
}

// StartFetchingLoop begins the periodic fetching of market data using a cron schedule.
// This should be run as a goroutine.
func (s *Service) StartFetchingLoop(ctx context.Context) {
	s.logger.Println("Initializing market data fetching loop with cron scheduler (runs at start of each minute).")

	// Schedule the job
	// The cron expression "0 * * * * *" means "at second 0 of every minute".
	entryID, err := s.cron.AddFunc("0 * * * * *", func() {
		// Create a new context for this specific job execution, respecting the parent context for cancellation.
		// This is important because fetchAndStoreData takes a context.
		jobCtx, cancelJobCtx := context.WithCancel(ctx)
		defer cancelJobCtx() // Ensure this inner context is cancelled when the job func returns

		// Check if overall service is stopping before running the job
		select {
		case <-s.stopChan:
			s.logger.Println("Cron job: stopChan signaled, skipping fetch.")
			return
		case <-ctx.Done():
			s.logger.Println("Cron job: main context cancelled, skipping fetch.")
			return
		default:
			// Proceed with fetch
		}
		s.logger.Printf("Cron job triggered: fetching market data at %s", time.Now().Format(time.RFC3339))
		s.fetchAndStoreData(jobCtx)
	})

	if err != nil {
		s.logger.Fatalf("Error scheduling cron job: %v", err)
		return
	}
	s.logger.Printf("Market data fetch job scheduled with EntryID %d, running at the start of each minute.", entryID)

	s.cron.Start()
	s.logger.Println("Cron scheduler started.")

	// Keep the StartFetchingLoop alive until a stop signal is received.
	// This loop now primarily serves to wait for a shutdown signal for the cron scheduler.
	select {
	case <-s.stopChan:
		s.logger.Println("Received stop signal on stopChan, stopping cron scheduler...")
		shutdownCtx := s.cron.Stop()
		<-shutdownCtx.Done()
		s.logger.Println("Cron scheduler stopped gracefully via stopChan.")
		return
	case <-ctx.Done():
		s.logger.Println("Received stop signal on main context, stopping cron scheduler...")
		shutdownCtx := s.cron.Stop()
		<-shutdownCtx.Done()
		s.logger.Println("Cron scheduler stopped gracefully via context cancellation.")
		return
	}
}

// fetchAndStoreData performs a single fetch from CoinGecko and stores the data in the repository.
func (s *Service) fetchAndStoreData(ctx context.Context) {
	s.logger.Printf("Fetching top %d coin markets from CoinGecko (vs %s)...", s.config.CoinGeckoPerPage, s.config.CoinGeckoVsCurrency)

	markets, err := s.cgClient.FetchCoinMarkets(
		ctx,
		s.config.CoinGeckoVsCurrency,
		nil, // ids - fetch by market cap, not specific IDs for now
		s.config.CoinGeckoOrder,
		s.config.CoinGeckoPerPage,
		s.config.CoinGeckoPage,
		s.config.CoinGeckoSparkline,
		s.config.CoinGeckoPriceChangePerc,
	)

	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			s.logger.Printf("Context cancelled during FetchCoinMarkets: %v", ctx.Err())
		} else if ctx.Err() == context.DeadlineExceeded {
			s.logger.Printf("Context deadline exceeded during FetchCoinMarkets: %v", ctx.Err())
		} else {
			s.logger.Printf("Error fetching market data from CoinGecko: %v", err)
		}
		return
	}

	if len(markets) == 0 {
		s.logger.Println("No market data returned from CoinGecko.")
		return
	}

	s.logger.Printf("Successfully fetched %d coin markets from CoinGecko. Storing in database...", len(markets))
	// Create a new context for the database operation, possibly with a shorter timeout
	// For now, reuse the job's context.
	dbCtx := ctx
	err = s.repo.UpsertCoinMarkets(dbCtx, markets)
	if err != nil {
		if dbCtx.Err() == context.Canceled {
			s.logger.Printf("Context cancelled during UpsertCoinMarkets: %v", dbCtx.Err())
		} else if dbCtx.Err() == context.DeadlineExceeded {
			s.logger.Printf("Context deadline exceeded during UpsertCoinMarkets: %v", dbCtx.Err())
		} else {
			s.logger.Printf("Error storing market data in database: %v", err)
		}
		return
	}

	s.logger.Printf("Successfully stored %d coin markets in the database.", len(markets))
}

// GetMarketDataFromDB retrieves paginated market data from the database.
func (s *Service) GetMarketDataFromDB(
	ctx context.Context,
	limit int,
	offset int,
	orderBy string,
	sortDirection string,
) ([]models.CoinMarket, int, time.Time, error) {
	return s.repo.GetCoinMarkets(ctx, limit, offset, orderBy, sortDirection)
}
