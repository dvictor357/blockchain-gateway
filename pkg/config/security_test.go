package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityConfigRejectsMalformedEnvironment(t *testing.T) {
	for _, raw := range []string{" ", ",eth_call", "eth_call,", "eth_call,,eth_getBalance", "*", "eth_*", "eth-call", "eth.call", "1method", "eth_call\neth_getBalance"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("API_KEY", "")
			t.Setenv("RPC_METHOD_ALLOWLIST", raw)
			err := LoadSecurityConfig().Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "RPC_METHOD_ALLOWLIST")
		})
	}
	for _, key := range []string{" ", "\t\n"} {
		t.Run("whitespace key "+key, func(t *testing.T) {
			t.Setenv("API_KEY", key)
			t.Setenv("RPC_METHOD_ALLOWLIST", "")
			err := LoadSecurityConfig().Validate()
			require.Error(t, err)
		})
	}
}
