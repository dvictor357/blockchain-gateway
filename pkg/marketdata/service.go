package marketdata

import (
	"context"
	"log"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/coingecko"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/robfig/cron/v3"
)

// defaultFetchTimeout bounds one scheduled fetch; it is shorter than the one-minute schedule.
const defaultFetchTimeout = 50 * time.Second

// ServiceConfig holds configuration for the MarketDataService
type ServiceConfig struct {
	CoinGeckoVsCurrency      string
	CoinGeckoOrder           string
	CoinGeckoPerPage         int
	CoinGeckoPage            int
	CoinGeckoSparkline       bool
	CoinGeckoPriceChangePerc []string // e.g., ["1h", "24h", "7d"]
	FetchTimeout             time.Duration
}

// Service orchestrates fetching and storing of market data.
// Only one Service should run StartFetchingLoop against a given database.
type Service struct {
	logger   *log.Logger
	cgClient *coingecko.Client
	repo     MarketRepository
	config   ServiceConfig
	cron     *cron.Cron
}

// NewService creates a new MarketDataService.
func NewService(logger *log.Logger, cgClient *coingecko.Client, repo MarketRepository, config ServiceConfig) *Service {
	if config.FetchTimeout <= 0 {
		config.FetchTimeout = defaultFetchTimeout
	}

	// SkipIfStillRunning prevents a slow fetch from overlapping the next scheduled one.
	c := cron.New(cron.WithSeconds(), cron.WithChain(cron.SkipIfStillRunning(cron.PrintfLogger(logger))))

	return &Service{
		logger:   logger,
		cgClient: cgClient,
		repo:     repo,
		config:   config,
		cron:     c,
	}
}

// StartFetchingLoop begins the periodic fetching of market data using a cron schedule and
// blocks until ctx is cancelled. On cancellation it stops scheduling and waits for a running
// fetch (which also observes ctx) to finish before returning.
func (s *Service) StartFetchingLoop(ctx context.Context) {
	s.logger.Println("Initializing market data fetching loop with cron scheduler (runs at start of each minute).")

	// The cron expression "0 * * * * *" means "at second 0 of every minute".
	entryID, err := s.cron.AddFunc("0 * * * * *", func() { s.runFetchJob(ctx) })
	if err != nil {
		s.logger.Printf("Error scheduling cron job: %v", err)
		return
	}
	s.logger.Printf("Market data fetch job scheduled with EntryID %d, running at the start of each minute.", entryID)

	s.cron.Start()
	s.logger.Println("Cron scheduler started.")

	<-ctx.Done()
	s.logger.Println("Stopping cron scheduler...")
	<-s.cron.Stop().Done()
	s.logger.Println("Cron scheduler stopped gracefully.")
}

// runFetchJob performs one scheduled fetch under its own deadline, derived from the service context.
func (s *Service) runFetchJob(ctx context.Context) {
	if ctx.Err() != nil {
		s.logger.Println("Cron job: context cancelled, skipping fetch.")
		return
	}

	jobCtx, cancel := context.WithTimeout(ctx, s.config.FetchTimeout)
	defer cancel()

	s.logger.Printf("Cron job triggered: fetching market data at %s", time.Now().Format(time.RFC3339))
	s.fetchAndStoreData(jobCtx)
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
	// The database write shares the job's deadline.
	err = s.repo.UpsertCoinMarkets(ctx, markets)
	if err != nil {
		if ctx.Err() == context.Canceled {
			s.logger.Printf("Context cancelled during UpsertCoinMarkets: %v", ctx.Err())
		} else if ctx.Err() == context.DeadlineExceeded {
			s.logger.Printf("Context deadline exceeded during UpsertCoinMarkets: %v", ctx.Err())
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
