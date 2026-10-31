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

func TestLoadConfig_OptionalServices(t *testing.T) {
	tests := []struct {
		name            string
		databaseEnabled string
		fetcherEnabled  string
		wantDatabase    bool
		wantFetcher     bool
		wantError       bool
	}{
		{name: "defaults disable optional services"},
		{name: "explicitly disabled", databaseEnabled: "false", fetcherEnabled: "false"},
		{name: "database serves stored data", databaseEnabled: "true", wantDatabase: true},
		{name: "database and fetcher enabled", databaseEnabled: "true", fetcherEnabled: "true", wantDatabase: true, wantFetcher: true},
		{name: "fetcher requires database", databaseEnabled: "false", fetcherEnabled: "true", wantFetcher: true, wantError: true},
		{name: "fetcher requires explicit database opt in", fetcherEnabled: "true", wantFetcher: true, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_ENABLED", tt.databaseEnabled)
			t.Setenv("MARKET_FETCHER_ENABLED", tt.fetcherEnabled)
			t.Setenv("REQUEST_TIMEOUT", "")
			t.Setenv("API_KEY", "")
			t.Setenv("RPC_METHOD_ALLOWLIST", "")

			cfg := LoadConfig()
			assert.Equal(t, tt.wantDatabase, cfg.Database.Enabled)
			assert.Equal(t, tt.wantFetcher, cfg.CoinGecko.FetcherEnabled)
			err := cfg.Validate()
			if tt.wantError {
				assert.ErrorContains(t, err, "MARKET_FETCHER_ENABLED requires DATABASE_ENABLED=true")
			} else {
				assert.NoError(t, err)
			}
		})
	}
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
