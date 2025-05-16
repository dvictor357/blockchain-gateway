-- Migrations for coin_markets table

CREATE TABLE IF NOT EXISTS coin_markets (
    id VARCHAR(255) PRIMARY KEY,
    symbol VARCHAR(50),
    name VARCHAR(255),
    image TEXT,
    current_price NUMERIC,
    market_cap BIGINT,
    market_cap_rank INT,
    fully_diluted_valuation BIGINT,
    total_volume NUMERIC,
    high_24h NUMERIC,
    low_24h NUMERIC,
    price_change_24h NUMERIC,
    price_change_percentage_24h NUMERIC,
    market_cap_change_24h NUMERIC,
    market_cap_change_percentage_24h NUMERIC,
    circulating_supply NUMERIC,
    total_supply NUMERIC,
    max_supply NUMERIC,
    ath NUMERIC,
    ath_change_percentage NUMERIC,
    ath_date TIMESTAMPTZ,
    atl NUMERIC,
    atl_change_percentage NUMERIC,
    atl_date TIMESTAMPTZ,
    roi JSONB,
    last_updated TIMESTAMPTZ, -- Timestamp from CoinGecko payload
    data_fetched_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP -- Timestamp of when our service inserted/updated the record
);

-- Indexes to improve query performance
CREATE INDEX IF NOT EXISTS idx_coin_markets_market_cap_rank ON coin_markets(market_cap_rank ASC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_coin_markets_last_updated ON coin_markets(last_updated DESC);
CREATE INDEX IF NOT EXISTS idx_coin_markets_data_fetched_at ON coin_markets(data_fetched_at DESC);

COMMENT ON COLUMN coin_markets.last_updated IS 'Timestamp from CoinGecko payload';
COMMENT ON COLUMN coin_markets.data_fetched_at IS 'Timestamp of when our service inserted/updated the record'; 