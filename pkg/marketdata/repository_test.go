package marketdata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvictor357/blockchain-gateway/internal/pgtest"
	"github.com/dvictor357/blockchain-gateway/pkg/database"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRepository(t *testing.T) (*PostgresMarketRepository, *sql.DB) {
	t.Helper()
	db := pgtest.Open(t)
	require.NoError(t, database.RunMigrations(context.Background(), db, "../../migrations"))
	return NewPostgresMarketRepository(db), db
}

func testMarket(id string, rank *int) models.CoinMarket {
	return models.CoinMarket{
		ID:            id,
		Symbol:        id,
		Name:          "Coin " + id,
		Image:         "https://example.invalid/" + id + ".png",
		CurrentPrice:  models.NullableFloat64(1.5),
		MarketCap:     models.NullableInt64(1_000_000),
		MarketCapRank: rank,
	}
}

func ids(markets []models.CoinMarket) []string {
	result := make([]string, len(markets))
	for i, market := range markets {
		result[i] = market.ID
	}
	return result
}

func countRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM coin_markets").Scan(&count))
	return count
}

func TestUpsertCoinMarkets_WritesEveryRowAcrossStatementBoundaries(t *testing.T) {
	repo, db := newTestRepository(t)
	ctx := context.Background()
	total := 2*upsertRowsPerStatement + 100

	markets := make([]models.CoinMarket, total)
	for i := range markets {
		markets[i] = testMarket(fmt.Sprintf("coin-%04d", i), models.NullableInt(i+1))
	}
	require.NoError(t, repo.UpsertCoinMarkets(ctx, markets))
	assert.Equal(t, total, countRows(t, db))

	// Upserting the same ids updates in place instead of duplicating or failing.
	for i := range markets {
		markets[i].Name = "Renamed " + markets[i].ID
	}
	require.NoError(t, repo.UpsertCoinMarkets(ctx, markets))
	assert.Equal(t, total, countRows(t, db))

	page, count, _, err := repo.GetCoinMarkets(ctx, 3, total-3, "market_cap_rank", "asc")
	require.NoError(t, err)
	assert.Equal(t, total, count)
	assert.Equal(t, []string{"coin-1097", "coin-1098", "coin-1099"}, ids(page))
	assert.Equal(t, "Renamed coin-1099", page[2].Name)
}

func TestUpsertCoinMarkets_DuplicateIDsKeepTheLastOccurrence(t *testing.T) {
	repo, _ := newTestRepository(t)
	first, last := testMarket("dup", models.NullableInt(1)), testMarket("dup", models.NullableInt(2))
	first.Name, last.Name = "first", "last"

	require.NoError(t, repo.UpsertCoinMarkets(context.Background(), []models.CoinMarket{first, testMarket("other", nil), last}))

	page, total, _, err := repo.GetCoinMarkets(context.Background(), 10, 0, "id", "asc")
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, page, 2)
	assert.Equal(t, "dup", page[0].ID)
	assert.Equal(t, "last", page[0].Name)
}

func TestUpsertCoinMarkets_FailureRollsBackEveryChunk(t *testing.T) {
	repo, db := newTestRepository(t)
	ctx := context.Background()
	baseline := testMarket("baseline", models.NullableInt(1))
	require.NoError(t, repo.UpsertCoinMarkets(ctx, []models.CoinMarket{baseline}))

	markets := make([]models.CoinMarket, upsertRowsPerStatement+10)
	for i := range markets {
		markets[i] = testMarket(fmt.Sprintf("new-%04d", i), models.NullableInt(i+10))
	}
	markets[len(markets)-1].Symbol = strings.Repeat("x", 51) // symbol is VARCHAR(50): fails in the second statement

	require.Error(t, repo.UpsertCoinMarkets(ctx, markets))

	assert.Equal(t, 1, countRows(t, db), "no row from the failed upsert may remain")
	var name string
	require.NoError(t, db.QueryRow("SELECT name FROM coin_markets WHERE id = 'baseline'").Scan(&name))
	assert.Equal(t, baseline.Name, name)
}

func TestUpsertCoinMarkets_EmptyInputIsANoOp(t *testing.T) {
	repo, db := newTestRepository(t)

	require.NoError(t, repo.UpsertCoinMarkets(context.Background(), nil))

	assert.Zero(t, countRows(t, db))
}

func TestUpsertCoinMarkets_RoundTripsROIAndNulls(t *testing.T) {
	repo, _ := newTestRepository(t)
	withROI := testMarket("with-roi", models.NullableInt(1))
	withROI.Roi = &models.RoiData{Times: 2.5, Currency: "btc", Percentage: 150.25}
	withoutROI := testMarket("without-roi", nil)
	withoutROI.CurrentPrice = nil

	require.NoError(t, repo.UpsertCoinMarkets(context.Background(), []models.CoinMarket{withROI, withoutROI}))

	page, _, _, err := repo.GetCoinMarkets(context.Background(), 10, 0, "market_cap_rank", "asc")
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, withROI.Roi, page[0].Roi)
	assert.Nil(t, page[1].Roi)
	assert.Nil(t, page[1].CurrentPrice)
	assert.Nil(t, page[1].MarketCapRank)
	assert.NotNil(t, page[0].DataFetchedAt)
}

func TestGetCoinMarkets_RowsWithNullTextColumnsAreReadable(t *testing.T) {
	repo, db := newTestRepository(t)
	_, err := db.Exec("INSERT INTO coin_markets (id) VALUES ('sparse')")
	require.NoError(t, err)

	page, total, _, err := repo.GetCoinMarkets(context.Background(), 10, 0, "market_cap_rank", "asc")

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, page, 1)
	assert.Equal(t, "sparse", page[0].ID)
}

func TestGetCoinMarkets_PagesAreStableWithTiedAndMissingSortKeys(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repo.UpsertCoinMarkets(ctx, []models.CoinMarket{
		testMarket("c1", models.NullableInt(1)),
		testMarket("c2", models.NullableInt(2)),
		testMarket("c3", models.NullableInt(2)),
		testMarket("c4", models.NullableInt(2)),
		testMarket("c5", nil),
		testMarket("c6", nil),
		testMarket("c7", models.NullableInt(3)),
	}))

	tests := []struct {
		name      string
		orderBy   string
		direction string
		want      []string
	}{
		{"rank ascending puts unranked last", "market_cap_rank", "asc", []string{"c1", "c2", "c3", "c4", "c7", "c5", "c6"}},
		{"rank descending puts unranked first", "market_cap_rank", "desc", []string{"c5", "c6", "c7", "c2", "c3", "c4", "c1"}},
		// Every row was written by one transaction, so data_fetched_at is tied across all rows.
		{"tied timestamps fall back to id ascending", "data_fetched_at", "desc", []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7"}},
		{"requested column is case-insensitive", "DATA_FETCHED_AT", "ASC", []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var walked []string
			for offset := 0; ; offset += 2 {
				page, total, _, err := repo.GetCoinMarkets(ctx, 2, offset, tt.orderBy, tt.direction)
				require.NoError(t, err)
				assert.Equal(t, 7, total)
				if len(page) == 0 {
					break
				}
				walked = append(walked, ids(page)...)
			}
			assert.Equal(t, tt.want, walked, "paging must visit every row exactly once in a fixed order")
		})
	}
}

func TestGetCoinMarkets_OrdersByRequestedColumn(t *testing.T) {
	repo, _ := newTestRepository(t)
	a, b, c := testMarket("a", models.NullableInt(1)), testMarket("b", models.NullableInt(2)), testMarket("c", models.NullableInt(3))
	a.Name, b.Name, c.Name = "Zed", "Amy", "Bob"
	require.NoError(t, repo.UpsertCoinMarkets(context.Background(), []models.CoinMarket{a, b, c}))

	page, _, _, err := repo.GetCoinMarkets(context.Background(), 10, 0, "NAME", "asc")

	require.NoError(t, err)
	assert.Equal(t, []string{"b", "c", "a"}, ids(page))
}

func TestGetCoinMarkets_EmptyTableStillReportsMetadata(t *testing.T) {
	repo, _ := newTestRepository(t)

	page, total, latest, err := repo.GetCoinMarkets(context.Background(), 20, 0, "market_cap_rank", "asc")

	require.NoError(t, err)
	assert.Empty(t, page)
	assert.Zero(t, total)
	assert.True(t, latest.IsZero())
}

func TestGetCoinMarkets_OffsetPastTheEndStillReportsTotalAndFreshness(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	before := time.Now().Add(-time.Minute)
	require.NoError(t, repo.UpsertCoinMarkets(ctx, []models.CoinMarket{
		testMarket("a", models.NullableInt(1)), testMarket("b", models.NullableInt(2)), testMarket("c", models.NullableInt(3)),
	}))

	page, total, latest, err := repo.GetCoinMarkets(ctx, 20, 500, "market_cap_rank", "asc")

	require.NoError(t, err)
	assert.Empty(t, page)
	assert.Equal(t, 3, total)
	assert.True(t, latest.After(before), "freshness must come from the stored rows, got %v", latest)

	exact, total, _, err := repo.GetCoinMarkets(ctx, 20, 3, "market_cap_rank", "asc")
	require.NoError(t, err)
	assert.Empty(t, exact)
	assert.Equal(t, 3, total)
}
