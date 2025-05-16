package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// CoinMarket defines the structure for coin market data.
// This structure is used across CoinGecko client, database repository, and API responses.
type CoinMarket struct {
	ID                           string     `json:"id"`
	Symbol                       string     `json:"symbol"`
	Name                         string     `json:"name"`
	Image                        string     `json:"image"`
	CurrentPrice                 *float64   `json:"current_price,omitempty"`
	MarketCap                    *int64     `json:"market_cap,omitempty"`
	MarketCapRank                *int       `json:"market_cap_rank,omitempty"`
	FullyDilutedValuation        *int64     `json:"fully_diluted_valuation,omitempty"`
	TotalVolume                  *float64   `json:"total_volume,omitempty"`
	High24h                      *float64   `json:"high_24h,omitempty"`
	Low24h                       *float64   `json:"low_24h,omitempty"`
	PriceChange24h               *float64   `json:"price_change_24h,omitempty"`
	PriceChangePercentage24h     *float64   `json:"price_change_percentage_24h,omitempty"`
	MarketCapChange24h           *float64   `json:"market_cap_change_24h,omitempty"`
	MarketCapChangePercentage24h *float64   `json:"market_cap_change_percentage_24h,omitempty"`
	CirculatingSupply            *float64   `json:"circulating_supply,omitempty"`
	TotalSupply                  *float64   `json:"total_supply,omitempty"`
	MaxSupply                    *float64   `json:"max_supply,omitempty"`
	Ath                          *float64   `json:"ath,omitempty"`
	AthChangePercentage          *float64   `json:"ath_change_percentage,omitempty"`
	AthDate                      *time.Time `json:"ath_date,omitempty"`
	Atl                          *float64   `json:"atl,omitempty"`
	AtlChangePercentage          *float64   `json:"atl_change_percentage,omitempty"`
	AtlDate                      *time.Time `json:"atl_date,omitempty"`
	Roi                          *RoiData   `json:"roi,omitempty"`
	LastUpdated                  *time.Time `json:"last_updated,omitempty"`
	DataFetchedAt                *time.Time `json:"data_fetched_at,omitempty"`
}

// RoiData defines the structure for ROI data.
type RoiData struct {
	Times      float64 `json:"times"`
	Currency   string  `json:"currency"`
	Percentage float64 `json:"percentage"`
}

// Value implements the driver.Valuer interface for RoiData.
func (r RoiData) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// Scan implements the sql.Scanner interface for RoiData.
func (r *RoiData) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed for RoiData scan")
	}
	return json.Unmarshal(b, &r)
}

// NullableFloat64 helper.
func NullableFloat64(val float64) *float64 {
	return &val
}

// NullableInt64 helper.
func NullableInt64(val int64) *int64 {
	return &val
}

// NullableInt helper.
func NullableInt(val int) *int {
	return &val
}

// NullableTime helper.
func NullableTime(val time.Time) *time.Time {
	return &val
}
