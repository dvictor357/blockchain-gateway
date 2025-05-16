package marketdata

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/user/blockchain-gateway/pkg/models"
)

// MarketRepository defines the interface for database operations on coin market data.
type MarketRepository interface {
	UpsertCoinMarkets(ctx context.Context, markets []models.CoinMarket) error
	GetCoinMarkets(ctx context.Context, limit int, offset int, orderBy string, sortDirection string) ([]models.CoinMarket, int, time.Time, error)
	GetLatestDataFetchedTimestamp(ctx context.Context) (time.Time, error)
}

// PostgresMarketRepository implements MarketRepository for PostgreSQL.
type PostgresMarketRepository struct {
	db *sql.DB
}

// NewPostgresMarketRepository creates a new PostgresMarketRepository.
func NewPostgresMarketRepository(db *sql.DB) *PostgresMarketRepository {
	return &PostgresMarketRepository{db: db}
}

// UpsertCoinMarkets inserts or updates multiple coin market data entries in the database.
// It uses a transaction to ensure atomicity.
func (r *PostgresMarketRepository) UpsertCoinMarkets(ctx context.Context, markets []models.CoinMarket) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	// Using a large VALUES clause for a single INSERT statement can be more efficient for many DBs than many individual INSERTs.
	// However, PostgreSQL also handles prepared statements and individual upserts well.
	// For clarity and to avoid overly complex string building for the VALUES clause with varying numbers of records,
	// we prepare one statement and execute it multiple times within the transaction.
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO coin_markets (
			id, symbol, name, image, current_price, market_cap, market_cap_rank,
			fully_diluted_valuation, total_volume, high_24h, low_24h,
			price_change_24h, price_change_percentage_24h,
			market_cap_change_24h, market_cap_change_percentage_24h,
			circulating_supply, total_supply, max_supply, ath, ath_change_percentage, ath_date,
			atl, atl_change_percentage, atl_date, roi, last_updated, data_fetched_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23, $24, $25, $26, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			symbol = EXCLUDED.symbol,
			name = EXCLUDED.name,
			image = EXCLUDED.image,
			current_price = EXCLUDED.current_price,
			market_cap = EXCLUDED.market_cap,
			market_cap_rank = EXCLUDED.market_cap_rank,
			fully_diluted_valuation = EXCLUDED.fully_diluted_valuation,
			total_volume = EXCLUDED.total_volume,
			high_24h = EXCLUDED.high_24h,
			low_24h = EXCLUDED.low_24h,
			price_change_24h = EXCLUDED.price_change_24h,
			price_change_percentage_24h = EXCLUDED.price_change_percentage_24h,
			market_cap_change_24h = EXCLUDED.market_cap_change_24h,
			market_cap_change_percentage_24h = EXCLUDED.market_cap_change_percentage_24h,
			circulating_supply = EXCLUDED.circulating_supply,
			total_supply = EXCLUDED.total_supply,
			max_supply = EXCLUDED.max_supply,
			ath = EXCLUDED.ath,
			ath_change_percentage = EXCLUDED.ath_change_percentage,
			ath_date = EXCLUDED.ath_date,
			atl = EXCLUDED.atl,
			atl_change_percentage = EXCLUDED.atl_change_percentage,
			atl_date = EXCLUDED.atl_date,
			roi = EXCLUDED.roi,
			last_updated = EXCLUDED.last_updated,
			data_fetched_at = NOW();
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare upsert statement: %w", err)
	}
	defer stmt.Close()

	for _, market := range markets {
		var roiJSONdriverValue driver.Value
		if market.Roi != nil {
			roiJSONdriverValue, err = market.Roi.Value() // Use the Value() method from RoiData
			if err != nil {
				return fmt.Errorf("failed to marshal ROI for market %s: %w", market.ID, err)
			}
		} else {
			roiJSONdriverValue = nil // Explicitly nil if Roi is nil
		}

		_, err := stmt.ExecContext(ctx,
			market.ID, market.Symbol, market.Name, market.Image, market.CurrentPrice, market.MarketCap, market.MarketCapRank,
			market.FullyDilutedValuation, market.TotalVolume, market.High24h, market.Low24h,
			market.PriceChange24h, market.PriceChangePercentage24h,
			market.MarketCapChange24h, market.MarketCapChangePercentage24h,
			market.CirculatingSupply, market.TotalSupply, market.MaxSupply, market.Ath, market.AthChangePercentage, market.AthDate,
			market.Atl, market.AtlChangePercentage, market.AtlDate, roiJSONdriverValue, market.LastUpdated,
		)
		if err != nil {
			return fmt.Errorf("failed to execute upsert statement for market %s: %w", market.ID, err)
		}
	}

	return tx.Commit()
}

// GetCoinMarkets retrieves a paginated list of coin market data from the database.
// It also returns the total count of records and the timestamp of the latest data fetch.
func (r *PostgresMarketRepository) GetCoinMarkets(ctx context.Context, limit int, offset int, orderBy string, sortDirection string) ([]models.CoinMarket, int, time.Time, error) {
	// Validate orderBy and sortDirection to prevent SQL injection if they were directly from user input
	// For now, we assume they are controlled internally or validated upstream.
	// Default ordering if not specified or invalid
	if orderBy == "" {
		orderBy = "market_cap_rank"
	}
	validOrderBys := map[string]bool{"market_cap_rank": true, "id": true, "name": true, "current_price": true, "last_updated": true, "data_fetched_at": true}
	if !validOrderBys[orderBy] {
		orderBy = "market_cap_rank" // Default to a safe column
	}

	if sortDirection == "" || (strings.ToUpper(sortDirection) != "ASC" && strings.ToUpper(sortDirection) != "DESC") {
		sortDirection = "ASC"
	}
	if orderBy == "market_cap_rank" && strings.ToUpper(sortDirection) == "ASC" {
		sortDirection = "ASC NULLS LAST" // Ensure nulls are at the end for market_cap_rank ASC
	} else if orderBy == "market_cap_rank" && strings.ToUpper(sortDirection) == "DESC" {
		sortDirection = "DESC NULLS FIRST" // Ensure nulls are at the beginning for market_cap_rank DESC
	}

	query := fmt.Sprintf(`
		SELECT
			id, symbol, name, image, current_price, market_cap, market_cap_rank,
			fully_diluted_valuation, total_volume, high_24h, low_24h,
			price_change_24h, price_change_percentage_24h,
			market_cap_change_24h, market_cap_change_percentage_24h,
			circulating_supply, total_supply, max_supply, ath, ath_change_percentage, ath_date,
			atl, atl_change_percentage, atl_date, roi, last_updated, data_fetched_at
		FROM coin_markets
		ORDER BY %s %s
		LIMIT $1 OFFSET $2;
	`, orderBy, sortDirection)

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("failed to query coin markets: %w", err)
	}
	defer rows.Close()

	var markets []models.CoinMarket
	for rows.Next() {
		var market models.CoinMarket
		var roiScannable models.RoiData // Use a temporary variable that implements sql.Scanner for market.Roi
		if err := rows.Scan(
			&market.ID, &market.Symbol, &market.Name, &market.Image, &market.CurrentPrice, &market.MarketCap, &market.MarketCapRank,
			&market.FullyDilutedValuation, &market.TotalVolume, &market.High24h, &market.Low24h,
			&market.PriceChange24h, &market.PriceChangePercentage24h,
			&market.MarketCapChange24h, &market.MarketCapChangePercentage24h,
			&market.CirculatingSupply, &market.TotalSupply, &market.MaxSupply, &market.Ath, &market.AthChangePercentage, &market.AthDate,
			&market.Atl, &market.AtlChangePercentage, &market.AtlDate, &roiScannable, &market.LastUpdated, &market.DataFetchedAt,
		); err != nil {
			return nil, 0, time.Time{}, fmt.Errorf("failed to scan coin market row: %w", err)
		}
		// If roiScannable was successfully scanned (even if it results in an empty RoiData if DB was NULL for JSONB),
		// assign it to market.Roi. If it was NULL in DB, roiScannable will be zero-value RoiData.
		// We might need to explicitly check if the JSONB was NULL if we want market.Roi to be nil instead of an empty struct.
		// For now, if JSONB is NULL, roiScannable.Scan should handle it (e.g., json.Unmarshal on null bytes might be fine or might need adjustment to set market.Roi = nil)
		// Let's assume for now that if JSONB is NULL, roiScannable will be its zero value, and we assign its address if not zero.
		// A better way: make roiScannable a *RoiData and let Scan handle nil.
		// Or check if roiScannable is its zero value. For simplicity with the existing types.go:
		if roiScannable.Times != 0 || roiScannable.Percentage != 0 || roiScannable.Currency != "" {
			market.Roi = &roiScannable
		}
		markets = append(markets, market)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("error iterating coin market rows: %w", err)
	}

	// Get total count for pagination
	var total int
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM coin_markets").Scan(&total)
	if err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("failed to query total coin markets count: %w", err)
	}

	// Get the latest data_fetched_at timestamp
	latestFetchTime, err := r.GetLatestDataFetchedTimestamp(ctx)
	if err != nil {
		// Log error but don't fail the whole request if we have data
		fmt.Printf("Warning: failed to get latest data fetched timestamp: %v\n", err)
	}

	return markets, total, latestFetchTime, nil
}

// GetLatestDataFetchedTimestamp retrieves the most recent data_fetched_at timestamp from the coin_markets table.
func (r *PostgresMarketRepository) GetLatestDataFetchedTimestamp(ctx context.Context) (time.Time, error) {
	var latestFetchTime sql.NullTime // Use sql.NullTime to handle case where table might be empty
	err := r.db.QueryRowContext(ctx, "SELECT MAX(data_fetched_at) FROM coin_markets").Scan(&latestFetchTime)
	if err != nil {
		if err == sql.ErrNoRows { // Table is empty or all data_fetched_at are NULL
			return time.Time{}, nil // Return zero time, no error
		}
		return time.Time{}, fmt.Errorf("failed to query latest data_fetched_at: %w", err)
	}
	if !latestFetchTime.Valid {
		return time.Time{}, nil // No valid timestamp found (e.g., table empty)
	}
	return latestFetchTime.Time, nil
}
