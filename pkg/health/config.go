package health

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/redis/go-redis/v9"
)

// Config holds health check configuration
type Config struct {
	// Health check intervals
	Interval time.Duration

	// HTTP client timeouts
	HTTPClientTimeout time.Duration

	// Check timeouts
	CheckTimeout time.Duration

	// Whether to enable detailed logging
	DetailedLogging bool

	// Components to check
	Components ComponentConfig
}

// ComponentConfig holds configuration for different components
type ComponentConfig struct {
	Database DatabaseHealthConfig
	Redis    RedisHealthConfig
	RPC      RPCHealthConfig
	External ExternalHealthConfig
}

// DatabaseHealthConfig holds database health check configuration
type DatabaseHealthConfig struct {
	Enabled         bool
	Component       string
	Priority        int
	CheckMigrations bool
}

// RedisHealthConfig holds Redis health check configuration
type RedisHealthConfig struct {
	Enabled      bool
	Component    string
	Priority     int
	CheckCluster bool
}

// RPCHealthConfig holds RPC health check configuration
type RPCHealthConfig struct {
	Enabled       bool
	Priority      int
	Timeout       time.Duration
	LatencyCheck  bool
	ChainsToCheck []string
}

// ExternalHealthConfig holds external API health check configuration
type ExternalHealthConfig struct {
	Enabled   bool
	Priority  int
	CoinGecko CoinGeckoHealthConfig
}

// CoinGeckoHealthConfig holds CoinGecko health check configuration
type CoinGeckoHealthConfig struct {
	Enabled bool
	APIURL  string
	Timeout time.Duration
}

// DefaultHealthConfig returns default health check configuration
func DefaultHealthConfig() Config {
	return Config{
		Interval:          30 * time.Second,
		HTTPClientTimeout: 10 * time.Second,
		CheckTimeout:      30 * time.Second,
		DetailedLogging:   true,
		Components: ComponentConfig{
			Database: DatabaseHealthConfig{
				Enabled:         true,
				Component:       "database",
				Priority:        1,
				CheckMigrations: true,
			},
			Redis: RedisHealthConfig{
				Enabled:      true,
				Component:    "redis",
				Priority:     1,
				CheckCluster: false,
			},
			RPC: RPCHealthConfig{
				Enabled:      true,
				Priority:     1,
				Timeout:      10 * time.Second,
				LatencyCheck: true,
			},
			External: ExternalHealthConfig{
				Enabled:  true,
				Priority: 5,
				CoinGecko: CoinGeckoHealthConfig{
					Enabled: true,
					APIURL:  "https://api.coingecko.com/api/v3",
					Timeout: 10 * time.Second,
				},
			},
		},
	}
}

// Builder builds health checkers from configuration
type Builder struct {
	config     Config
	httpClient *http.Client
	logger     *log.Logger
}

// NewBuilder creates a new health check builder
func NewBuilder(config Config, logger *log.Logger) *Builder {
	return &Builder{
		config: config,
		httpClient: &http.Client{
			Timeout: config.HTTPClientTimeout,
		},
		logger: logger,
	}
}

// Build builds all health checkers based on configuration
func (b *Builder) Build(appConfig *config.AppConfig, db *sql.DB, redisClient *redis.Client) *HealthChecker {
	hc := NewHealthChecker(b.logger)

	// Database checks
	if b.config.Components.Database.Enabled && db != nil {
		// Database connection checker
		dbChecker := NewDatabaseConnectionChecker(db, b.config.Components.Database.Component)
		hc.AddChecker(dbChecker)

		// Database stats checker
		dbStatsChecker := NewDatabaseChecker(db, "database_stats")
		hc.AddChecker(dbStatsChecker)

		// Migration checker
		if b.config.Components.Database.CheckMigrations {
			migrationChecker := NewDatabaseMigrationChecker(db, appConfig.MigrationsDir)
			hc.AddChecker(migrationChecker)
		}
	}

	// Redis checks
	if b.config.Components.Redis.Enabled && redisClient != nil {
		redisChecker := NewRedisChecker(redisClient, b.config.Components.Redis.Component)
		hc.AddChecker(redisChecker)

		if b.config.Components.Redis.CheckCluster {
			clusterChecker := NewRedisClusterChecker(redisClient)
			hc.AddChecker(clusterChecker)
		}
	}

	// RPC checks
	if b.config.Components.RPC.Enabled {
		rpcCheckers := buildRPCCheckers(appConfig, b.httpClient, b.config.Components.RPC)
		for _, checker := range rpcCheckers {
			hc.AddChecker(checker)
		}

		// Latency checker
		if b.config.Components.RPC.LatencyCheck && len(rpcCheckers) > 0 {
			latencyChecker := NewRPCLatencyChecker(rpcCheckers)
			hc.AddChecker(latencyChecker)
		}
	}

	// External API checks
	if b.config.Components.External.Enabled {
		// CoinGecko
		if b.config.Components.External.CoinGecko.Enabled {
			coingeckoChecker := NewCoinGeckoChecker(
				b.config.Components.External.CoinGecko.APIURL,
				b.httpClient,
			)
			hc.AddChecker(coingeckoChecker)
		}
	}

	return hc
}

// buildRPCCheckers builds RPC health checkers for all configured chains
func buildRPCCheckers(appConfig *config.AppConfig, client *http.Client, config RPCHealthConfig) []*RPCChecker {
	checkers := make([]*RPCChecker, 0)

	// Add EVM chains
	for _, chain := range appConfig.Chains.EVMChains {
		if !chain.Enabled {
			continue
		}

		checker := NewRPCChecker(
			"rpc_"+chain.Name,
			chain.RPCURL,
			chain.Type,
			client,
		)
		checkers = append(checkers, checker)
	}

	return checkers
}
