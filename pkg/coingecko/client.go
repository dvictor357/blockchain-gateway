package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/user/blockchain-gateway/pkg/models"
)

const (
	defaultTimeout = 30 * time.Second
	defaultBaseURL = "https://api.coingecko.com/api/v3"
)

// Client is a CoinGecko API client
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new CoinGecko API client
func NewClient(httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// FetchCoinMarkets fetches market data for a list of coins from CoinGecko.
func (c *Client) FetchCoinMarkets(ctx context.Context, vsCurrency string, ids []string, order string, perPage int, page int, sparkline bool, priceChangePercentage []string) ([]models.CoinMarket, error) {
	endpoint := "/coins/markets"
	params := url.Values{}

	if vsCurrency == "" {
		vsCurrency = "usd" // Default to USD
	}
	params.Set("vs_currency", vsCurrency)

	if len(ids) > 0 {
		params.Set("ids", strings.Join(ids, ","))
	}
	if order != "" {
		params.Set("order", order)
	}
	if perPage > 0 {
		params.Set("per_page", fmt.Sprintf("%d", perPage))
	}
	if page > 0 {
		params.Set("page", fmt.Sprintf("%d", page))
	}
	params.Set("sparkline", fmt.Sprintf("%t", sparkline))
	if len(priceChangePercentage) > 0 {
		params.Set("price_change_percentage", strings.Join(priceChangePercentage, ","))
	}

	fullURL := fmt.Sprintf("%s%s?%s", c.baseURL, endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create CoinGecko request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute CoinGecko request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// TODO: Parse error body from CoinGecko if available for more details
		return nil, fmt.Errorf("CoinGecko API request failed with status %s", resp.Status)
	}

	var markets []models.CoinMarket
	if err := json.NewDecoder(resp.Body).Decode(&markets); err != nil {
		return nil, fmt.Errorf("failed to decode CoinGecko response: %w", err)
	}

	return markets, nil
}
