package marketdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/models"
	_ "github.com/lib/pq"
)

const (
	// upsertParamsPerRow is the number of bound parameters per market row (data_fetched_at uses NOW()).
	upsertParamsPerRow = 26
	// upsertRoiParam is the zero-based position of the roi parameter, which is cast to jsonb.
	upsertRoiParam = 24
	// upsertRowsPerStatement keeps each statement far below PostgreSQL's 65535-parameter limit.
	upsertRowsPerStatement = 500
)

// marketColumns lists the coin_markets columns in the order used for reads and scans.
var marketColumns = []string{
	"id", "symbol", "name", "image", "current_price", "market_cap", "market_cap_rank",
	"fully_diluted_valuation", "total_volume", "high_24h", "low_24h",
	"price_change_24h", "price_change_percentage_24h",
	"market_cap_change_24h", "market_cap_change_percentage_24h",
	"circulating_supply", "total_supply", "max_supply", "ath", "ath_change_percentage", "ath_date",
	"atl", "atl_change_percentage", "atl_date", "roi", "last_updated", "data_fetched_at",
}

// upsertColumns are the columns written by an upsert: the parameter columns in order, then data_fetched_at.
var upsertColumns = strings.Join(marketColumns, ", ")

// pageSelectColumns is the page-qualified column list of the paginated read, built once at startup.
var pageSelectColumns = "page." + strings.Join(marketColumns, ", page.")

const upsertConflictClause = `
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
		data_fetched_at = NOW()`

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

// upsertStatement builds a multi-row upsert for the given number of rows. Only $n placeholders
// are generated; all values are bound parameters.
func upsertStatement(rows int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO coin_markets (" + upsertColumns + ") VALUES ")
	for row := 0; row < rows; row++ {
		if row > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for param := 0; param < upsertParamsPerRow; param++ {
			if param > 0 {
				b.WriteString(", ")
			}
			b.WriteString("$" + strconv.Itoa(row*upsertParamsPerRow+param+1))
			if param == upsertRoiParam {
				b.WriteString("::jsonb")
			}
		}
		b.WriteString(", NOW())")
	}
	b.WriteString(upsertConflictClause)
	return b.String()
}

// UpsertCoinMarkets inserts or updates multiple coin market data entries in the database.
// Rows are written with bounded multi-row statements inside one transaction, so the whole set
// is applied atomically. If an id appears more than once the last occurrence wins.
func (r *PostgresMarketRepository) UpsertCoinMarkets(ctx context.Context, markets []models.CoinMarket) error {
	if len(markets) == 0 {
		return nil
	}

	// A single INSERT ... ON CONFLICT cannot touch the same row twice.
	unique := make([]models.CoinMarket, 0, len(markets))
	position := make(map[string]int, len(markets))
	for _, market := range markets {
		if i, seen := position[market.ID]; seen {
			unique[i] = market
			continue
		}
		position[market.ID] = len(unique)
		unique = append(unique, market)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	for start := 0; start < len(unique); start += upsertRowsPerStatement {
		end := min(start+upsertRowsPerStatement, len(unique))
		chunk := unique[start:end]

		args := make([]any, 0, len(chunk)*upsertParamsPerRow)
		for _, market := range chunk {
			var roi any
			if market.Roi != nil {
				roiJSON, err := json.Marshal(market.Roi)
				if err != nil {
					return fmt.Errorf("failed to marshal ROI for market %s: %w", market.ID, err)
				}
				roi = string(roiJSON)
			}

			args = append(args,
				market.ID, market.Symbol, market.Name, market.Image, market.CurrentPrice, market.MarketCap, market.MarketCapRank,
				market.FullyDilutedValuation, market.TotalVolume, market.High24h, market.Low24h,
				market.PriceChange24h, market.PriceChangePercentage24h,
				market.MarketCapChange24h, market.MarketCapChangePercentage24h,
				market.CirculatingSupply, market.TotalSupply, market.MaxSupply, market.Ath, market.AthChangePercentage, market.AthDate,
				market.Atl, market.AtlChangePercentage, market.AtlDate, roi, market.LastUpdated,
			)
		}

		if _, err := tx.ExecContext(ctx, upsertStatement(len(chunk)), args...); err != nil {
			return fmt.Errorf("failed to upsert markets %d-%d: %w", start, end-1, err)
		}
	}

	return tx.Commit()
}

// orderableColumns are the columns a caller may sort by.
var orderableColumns = map[string]bool{
	"market_cap_rank": true, "id": true, "name": true, "current_price": true, "last_updated": true, "data_fetched_at": true,
}

// orderClause builds a deterministic ORDER BY body from a whitelisted column and direction.
// prefix qualifies the columns (for example "page."). The id tiebreaker keeps pages stable.
func orderClause(orderBy, sortDirection, prefix string) string {
	orderBy = strings.ToLower(orderBy)
	if !orderableColumns[orderBy] {
		orderBy = "market_cap_rank"
	}

	direction := "ASC"
	if strings.EqualFold(sortDirection, "desc") {
		direction = "DESC"
	}
	if orderBy == "market_cap_rank" {
		// Unranked coins go last when ascending and first when descending.
		if direction == "ASC" {
			direction = "ASC NULLS LAST"
		} else {
			direction = "DESC NULLS FIRST"
		}
	}

	return fmt.Sprintf("%s%s %s, %sid ASC", prefix, orderBy, direction, prefix)
}

// GetCoinMarkets retrieves a paginated list of coin market data from the database.
// It also returns the total count of records and the timestamp of the latest data fetch.
// Everything comes from one statement, so the page, total and timestamp are mutually consistent
// even while a refresh runs, and an empty table or an offset past the end still reports them.
func (r *PostgresMarketRepository) GetCoinMarkets(ctx context.Context, limit int, offset int, orderBy string, sortDirection string) ([]models.CoinMarket, int, time.Time, error) {
	// stats always yields exactly one row; LEFT JOIN keeps it when the page is empty.
	// orderBy and sortDirection are normalized against a whitelist, never interpolated raw.
	query := fmt.Sprintf(`
		WITH stats AS (
			SELECT COUNT(*) AS total, MAX(data_fetched_at) AS latest_fetched_at
			FROM coin_markets
		), page AS (
			SELECT %[1]s
			FROM coin_markets
			ORDER BY %[2]s
			LIMIT $1 OFFSET $2
		)
		SELECT stats.total, stats.latest_fetched_at, %[3]s
		FROM stats
		LEFT JOIN page ON TRUE
		ORDER BY %[4]s`,
		upsertColumns,
		orderClause(orderBy, sortDirection, ""),
		pageSelectColumns,
		orderClause(orderBy, sortDirection, "page."),
	)

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("failed to query coin markets: %w", err)
	}
	defer rows.Close()

	var (
		markets         []models.CoinMarket
		total           int
		latestFetchTime time.Time
	)
	for rows.Next() {
		var (
			latest                  sql.NullTime
			id, symbol, name, image sql.NullString
			market                  models.CoinMarket
			roiScannable            models.RoiData
		)
		if err := rows.Scan(
			&total, &latest,
			&id, &symbol, &name, &image, &market.CurrentPrice, &market.MarketCap, &market.MarketCapRank,
			&market.FullyDilutedValuation, &market.TotalVolume, &market.High24h, &market.Low24h,
			&market.PriceChange24h, &market.PriceChangePercentage24h,
			&market.MarketCapChange24h, &market.MarketCapChangePercentage24h,
			&market.CirculatingSupply, &market.TotalSupply, &market.MaxSupply, &market.Ath, &market.AthChangePercentage, &market.AthDate,
			&market.Atl, &market.AtlChangePercentage, &market.AtlDate, &roiScannable, &market.LastUpdated, &market.DataFetchedAt,
		); err != nil {
			return nil, 0, time.Time{}, fmt.Errorf("failed to scan coin market row: %w", err)
		}
		if latest.Valid {
			latestFetchTime = latest.Time
		}

		if !id.Valid {
			continue // the LEFT JOIN row for an empty page carries only the statistics
		}
		market.ID, market.Symbol, market.Name, market.Image = id.String, symbol.String, name.String, image.String

		// A NULL roi column leaves roiScannable at its zero value.
		if roiScannable.Times != 0 || roiScannable.Percentage != 0 || roiScannable.Currency != "" {
			market.Roi = &roiScannable
		}
		markets = append(markets, market)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("error iterating coin market rows: %w", err)
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
