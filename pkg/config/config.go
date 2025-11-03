package config

import (
	"os"
	"strconv"
)

// Constants for default values
const (
	defaultPort           = "8080"
	defaultHost           = "0.0.0.0"
	defaultGinMode        = "release"
	defaultLogLevel       = "info"
	defaultRequestTimeout = 30

	defaultDBHost     = "localhost"
	defaultDBPort     = "5432"
	defaultDBUser     = "postgres"
	defaultDBPassword = ""
	defaultDBName     = "blockchain_gateway"
	defaultDBSSLMode  = "disable"

	defaultRedisHost = "localhost"
	defaultRedisPort = "6379"
	defaultRedisDB   = 0

	defaultCoinGeckoBaseURL    = "https://api.coingecko.com/api/v3"
	defaultCoinGeckoPerPage    = 100
	defaultCoinGeckoOrder      = "market_cap_desc"
	defaultCoinGeckoVsCurrency = "usd"

	migrationsDir = "migrations"
)

// ServerConfig holds server-specific configurations
type ServerConfig struct {
	Port      string
	Host      string
	GinMode   string
	RateLimit int
	// RequestTimeout time.Duration // For http.Server, if needed directly here
}

// DatabaseConfig holds database connection parameters
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// CoinGeckoConfig holds CoinGecko API client configurations
type CoinGeckoConfig struct {
	BaseURL    string
	PerPage    int
	Order      string
	VsCurrency string
	// Add API Key if needed in future
}

// RedisConfig holds Redis connection parameters
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
	Enabled  bool
}

// ChainConfig holds configuration for individual blockchain networks
type ChainConfig struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	NativeToken string `json:"native_token"`
	Decimals    int    `json:"decimals"`
	ChainID     int64  `json:"chain_id"`
	Type        string `json:"type"` // "evm", "bitcoin", "other"
	RPCURL      string `json:"rpc_url"`
	Explorer    string `json:"explorer"`
	Enabled     bool   `json:"enabled"`
}

// ChainsConfig holds configuration for all supported blockchain networks
type ChainsConfig struct {
	EVMChains []ChainConfig `json:"evm_chains"`
}

// AppConfig holds all application configurations
type AppConfig struct {
	Server        ServerConfig
	Database      DatabaseConfig
	CoinGecko     CoinGeckoConfig
	Redis         RedisConfig
	Chains        ChainsConfig
	LogLevel      string
	MigrationsDir string
}

// LoadConfig loads application configurations from environment variables with defaults
func LoadConfig() *AppConfig {
	cfg := &AppConfig{
		Server: ServerConfig{
			Port:      GetStringEnv("PORT", defaultPort),
			Host:      defaultHost,
			GinMode:   GetStringEnv("GIN_MODE", defaultGinMode),
			RateLimit: GetIntEnv("RATE_LIMIT", 120),
		},
		Database: DatabaseConfig{
			Host:     GetStringEnv("DB_HOST", defaultDBHost),
			Port:     GetStringEnv("DB_PORT", defaultDBPort),
			User:     GetStringEnv("DB_USER", defaultDBUser),
			Password: GetStringEnv("DB_PASSWORD", defaultDBPassword),
			DBName:   GetStringEnv("DB_NAME", defaultDBName),
			SSLMode:  GetStringEnv("DB_SSLMODE", defaultDBSSLMode),
		},
		CoinGecko: CoinGeckoConfig{
			BaseURL:    GetStringEnv("COINGECKO_BASE_URL", defaultCoinGeckoBaseURL),
			PerPage:    GetIntEnv("COINGECKO_PER_PAGE", defaultCoinGeckoPerPage),
			Order:      GetStringEnv("COINGECKO_ORDER", defaultCoinGeckoOrder),
			VsCurrency: GetStringEnv("COINGECKO_VS_CURRENCY", defaultCoinGeckoVsCurrency),
		},
		Redis: RedisConfig{
			Host:     GetStringEnv("REDIS_HOST", defaultRedisHost),
			Port:     GetStringEnv("REDIS_PORT", defaultRedisPort),
			Password: GetStringEnv("REDIS_PASSWORD", ""),
			DB:       GetIntEnv("REDIS_DB", defaultRedisDB),
			Enabled:  GetBoolEnv("REDIS_ENABLED", false),
		},
		Chains:        LoadChainsConfig(),
		LogLevel:      GetStringEnv("LOG_LEVEL", defaultLogLevel),
		MigrationsDir: migrationsDir, // Static for now
	}
	return cfg
}

// LoadChainsConfig loads blockchain network configurations
func LoadChainsConfig() ChainsConfig {
	return ChainsConfig{
		EVMChains: []ChainConfig{
			// Ethereum - Original EVM chain
			{
				Name:        "ethereum",
				DisplayName: "Ethereum",
				NativeToken: "ETH",
				Decimals:    18,
				ChainID:     1,
				Type:        "evm",
				RPCURL:      GetStringEnv("ETH_RPC_URL", "https://ethereum.publicnode.com"),
				Explorer:    "https://etherscan.io",
				Enabled:     true,
			},
			// Polygon - Layer 2 scaling solution
			{
				Name:        "polygon",
				DisplayName: "Polygon",
				NativeToken: "POL",
				Decimals:    18,
				ChainID:     137,
				Type:        "evm",
				RPCURL:      GetStringEnv("POLYGON_RPC_URL", "https://polygon-bor-rpc.publicnode.com"),
				Explorer:    "https://polygonscan.com",
				Enabled:     true,
			},
			// BNB Smart Chain - High performance, low fees
			{
				Name:        "bsc",
				DisplayName: "BNB Smart Chain",
				NativeToken: "BNB",
				Decimals:    18,
				ChainID:     56,
				Type:        "evm",
				RPCURL:      GetStringEnv("BSC_RPC_URL", "https://bsc-dataseed.binance.org"),
				Explorer:    "https://bscscan.com",
				Enabled:     GetBoolEnv("BSC_ENABLED", true),
			},
			// Arbitrum - Leading Layer 2 optimistic rollup
			{
				Name:        "arbitrum",
				DisplayName: "Arbitrum One",
				NativeToken: "ETH",
				Decimals:    18,
				ChainID:     42161,
				Type:        "evm",
				RPCURL:      GetStringEnv("ARBITRUM_RPC_URL", "https://arb1.arbitrum.io/rpc"),
				Explorer:    "https://arbiscan.io",
				Enabled:     GetBoolEnv("ARBITRUM_ENABLED", true),
			},
			// Optimism - Optimistic rollup Layer 2
			{
				Name:        "optimism",
				DisplayName: "Optimism",
				NativeToken: "ETH",
				Decimals:    18,
				ChainID:     10,
				Type:        "evm",
				RPCURL:      GetStringEnv("OPTIMISM_RPC_URL", "https://mainnet.optimism.io"),
				Explorer:    "https://optimistic.etherscan.io",
				Enabled:     GetBoolEnv("OPTIMISM_ENABLED", true),
			},
			// Base - Coinbase's Layer 2
			{
				Name:        "base",
				DisplayName: "Base",
				NativeToken: "ETH",
				Decimals:    18,
				ChainID:     8453,
				Type:        "evm",
				RPCURL:      GetStringEnv("BASE_RPC_URL", "https://mainnet.base.org"),
				Explorer:    "https://basescan.org",
				Enabled:     GetBoolEnv("BASE_ENABLED", true),
			},
			// Avalanche C-Chain - High throughput blockchain
			{
				Name:        "avalanche",
				DisplayName: "Avalanche C-Chain",
				NativeToken: "AVAX",
				Decimals:    18,
				ChainID:     43114,
				Type:        "evm",
				RPCURL:      GetStringEnv("AVALANCHE_RPC_URL", "https://api.avax.network/ext/bc/C/rpc"),
				Explorer:    "https://snowtrace.io",
				Enabled:     GetBoolEnv("AVALANCHE_ENABLED", true),
			},
			// Fantom - Fast finality blockchain
			{
				Name:        "fantom",
				DisplayName: "Fantom",
				NativeToken: "FTM",
				Decimals:    18,
				ChainID:     250,
				Type:        "evm",
				RPCURL:      GetStringEnv("FANTOM_RPC_URL", "https://rpc.ftm.tools"),
				Explorer:    "https://ftmscan.com",
				Enabled:     GetBoolEnv("FANTOM_ENABLED", false), // Disabled by default
			},
		},
	}
}

// Helper function to get string environment variable or default
func GetStringEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// Helper function to get integer environment variable or default
func GetIntEnv(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		// Log or handle error appropriately, for now, fallback to default
		// log.Printf("Warning: Invalid value for %s: %s. Using default: %d", key, valueStr, defaultValue)
		return defaultValue
	}
	return value
}

// Helper function to get boolean environment variable or default
func GetBoolEnv(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
