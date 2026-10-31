package blockchain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBTCToSatoshi_ExactConversions(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0"},
		{"0.0", "0"},
		{"0.00000000", "0"},
		{"0.00000001", "1"},
		{"0.00000029", "29"}, // float64(0.00000029)*1e8 truncates to 28
		{"0.00000058", "58"},
		{"0.1", "10000000"},
		{"0.29", "29000000"},
		{"1", "100000000"},
		{"1.00000000", "100000000"},
		{"12.34567891", "1234567891"},
		{"1e-8", "1"},
		{"2.9E-7", "29"},
		{"2.9e-07", "29"},
		{"1E+1", "1000000000"},
		{"100e-2", "100000000"},
		{"0.000000290e0", "29"},
		{"21000000", "2100000000000000"},
		{"21000000.00000000", "2100000000000000"},
	}
	for _, tt := range tests {
		got, err := btcToSatoshi(json.RawMessage(tt.input))
		require.NoError(t, err, tt.input)
		assert.Equal(t, tt.want, got.String(), tt.input)
	}
}

func TestBTCToSatoshi_RejectsUnsafeValues(t *testing.T) {
	tests := map[string]string{
		"fractional satoshi":            "0.000000005",
		"fractional satoshi via exp":    "1e-9",
		"fractional after big mantissa": "1.123456789",
		"negative":                      "-0.1",
		"negative zero":                 "-0",
		"empty":                         "",
		"text":                          "abc",
		"quoted number":                 `"0.1"`,
		"null":                          "null",
		"leading plus":                  "+1",
		"leading dot":                   ".5",
		"trailing dot":                  "1.",
		"two dots":                      "1.2.3",
		"bare exponent":                 "1e",
		"signed bare exponent":          "1e+",
		"hex":                           "0x10",
		"fraction syntax":               "1/3",
		"NaN":                           "NaN",
		"above supply cap":              "21000000.00000001",
		"far above supply cap":          "99999999999",
		"huge exponent":                 "1e999999",
		"huge exponent, tiny mantissa":  "1e999999999999",
		"exponent just above bound":     "1e33",
		"negative huge exponent":        "1e-999999",
		"very long digits":              strings.Repeat("9", 80),
		"very long fraction":            "0." + strings.Repeat("0", 80) + "1",
	}
	for name, input := range tests {
		got, err := btcToSatoshi(json.RawMessage(input))
		assert.ErrorIs(t, err, ErrUpstream, name)
		assert.Nil(t, got, name)
	}
}
