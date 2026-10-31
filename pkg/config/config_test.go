package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig_RequestTimeout(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		set       bool
		want      time.Duration
		wantValid bool
	}{
		{name: "unset uses the default", want: 30 * time.Second, wantValid: true},
		{name: "explicit seconds", value: "45", set: true, want: 45 * time.Second, wantValid: true},
		{name: "lower bound", value: "1", set: true, want: time.Second, wantValid: true},
		{name: "upper bound", value: "120", set: true, want: 120 * time.Second, wantValid: true},
		{name: "above upper bound", value: "121", set: true, want: 121 * time.Second},
		{name: "zero", value: "0", set: true},
		{name: "negative", value: "-5", set: true},
		{name: "duration syntax is not seconds", value: "30s", set: true},
		{name: "non-numeric", value: "soon", set: true},
		{name: "absurdly large", value: "99999999999999", set: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("REQUEST_TIMEOUT", tt.value)
			} else {
				t.Setenv("REQUEST_TIMEOUT", "")
			}

			cfg := LoadConfig()

			assert.Equal(t, tt.want, cfg.Server.RequestTimeout)
			err := cfg.Validate()
			if tt.wantValid {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, "REQUEST_TIMEOUT")
			}
		})
	}
}

func TestLoadConfig_BitcoinRPCURL(t *testing.T) {
	t.Setenv("BTC_RPC_URL", "")
	assert.Empty(t, LoadConfig().Chains.BitcoinRPCURL, "unset leaves the built-in default to the client manager")

	t.Setenv("BTC_RPC_URL", "http://user:pass@127.0.0.1:8332")
	assert.Equal(t, "http://user:pass@127.0.0.1:8332", LoadConfig().Chains.BitcoinRPCURL)
}

func TestLoadConfig_MarketFetcherFlag(t *testing.T) {
	t.Setenv("MARKET_FETCHER_ENABLED", "")
	assert.True(t, LoadConfig().CoinGecko.FetcherEnabled)

	t.Setenv("MARKET_FETCHER_ENABLED", "false")
	assert.False(t, LoadConfig().CoinGecko.FetcherEnabled)
}

func TestNewAPIConfigFromServer(t *testing.T) {
	cfg := NewAPIConfigFromServer(ServerConfig{RequestTimeout: 20 * time.Second})

	assert.Equal(t, 20*time.Second, cfg.DefaultTimeout)
	assert.Equal(t, 40*time.Second, cfg.BatchTimeout)
	assert.Equal(t, 40*time.Second, cfg.LongestTimeout())
	assert.Equal(t, NewAPIConfig().MaxRequestBodyBytes, cfg.MaxRequestBodyBytes)

	fallback := NewAPIConfigFromServer(ServerConfig{})
	assert.Equal(t, NewAPIConfig().DefaultTimeout, fallback.DefaultTimeout)
	assert.Equal(t, NewAPIConfig().BatchTimeout, fallback.BatchTimeout)
}
