# EVM Chains Configuration Guide

This document explains how to configure and use multiple EVM-compatible blockchains in the Blockchain Gateway.

## Overview

The Blockchain Gateway now supports **8 EVM-compatible blockchains** out of the box, with a configuration-driven approach that makes it easy to add more chains or customize existing ones.

## Supported EVM Chains

### Layer 1 Blockchains

1. **Ethereum** - The original EVM blockchain
2. **BNB Smart Chain (BSC)** - High performance, low fees
3. **Avalanche C-Chain** - High throughput blockchain
4. **Fantom** - Fast finality blockchain
5. **Polygon** - Ethereum scaling solution

### Layer 2 Blockchains

1. **Arbitrum One** - Leading optimistic rollup
2. **Optimism** - Optimistic rollup Layer 2
3. **Base** - Coinbase's Layer 2

## Configuration

### Environment Variables

Each EVM chain can be configured using environment variables:

```bash
# Chain RPC URL (Supports http, https, ws, wss schemes)
{CHAIN}_RPC_URL=https://rpc-endpoint.com

# Enable/Disable chain (optional, defaults to true for most chains)
{CHAIN}_ENABLED=true
```

### RPC URL Schemes

The `RPCURL` for each chain supports standard HTTP/HTTPS endpoints as well as WebSocket (WS/WSS) endpoints.
- `http://...`
- `https://...`
- `ws://...`
- `wss://...`

The system will automatically detect the scheme from the URL. If `ws` or `wss` is detected, the gateway will establish a WebSocket connection for communication with the RPC node. Otherwise, it will use a standard HTTP/S client. This allows for potentially lower latency and persistent connections when using WebSocket-enabled RPC providers.

### Example Configuration

```bash
# BNB Smart Chain (using HTTPS)
BSC_RPC_URL=https://bsc-dataseed.binance.org
BSC_ENABLED=true

# Arbitrum (using HTTPS)
ARBITRUM_RPC_URL=https://arb1.arbitrum.io/rpc
ARBITRUM_ENABLED=true

# Base (using WSS - WebSocket Secure)
BASE_RPC_URL=wss://base-mainnet.g.alchemy.com/v2/YOUR_ALCHEMY_KEY
BASE_ENABLED=true

# Ethereum (example with a WebSocket URL if your provider supports it)
ETH_RPC_URL=wss://mainnet.infura.io/ws/v3/YOUR_INFURA_PROJECT_ID
# ETH_RPC_URL=https://ethereum.publicnode.com (default HTTPS)
```

### Default Configuration

The system comes with sensible defaults for all chains (typically using HTTPS):

| Chain     | Default RPC                              | Default Status |
| --------- | ---------------------------------------- | -------------- |
| Ethereum  | `https://ethereum.publicnode.com`        | Always enabled |
| Polygon   | `https://polygon-bor-rpc.publicnode.com` | Always enabled |
| BSC       | `https://bsc-dataseed.binance.org`       | Enabled        |
| Arbitrum  | `https://arb1.arbitrum.io/rpc`           | Enabled        |
| Optimism  | `https://mainnet.optimism.io`            | Enabled        |
| Base      | `https://mainnet.base.org`               | Enabled        |
| Avalanche | `https://api.avax.network/ext/bc/C/rpc`  | Enabled        |
| Fantom    | `https://rpc.ftm.tools`                  | Disabled       |

## API Usage

### List Available Chains

```bash
GET /api/v1/chains
```

Response:

```json
{
  "chains": [
    "ethereum",
    "polygon",
    "bsc",
    "arbitrum",
    "optimism",
    "base",
    "avalanche"
  ],
  "count": 7
}
```

### Query Any EVM Chain

All EVM chains support the same API endpoints:

```bash
# Get balance
GET /api/v1/chains/{chain}/address/{address}/balance

# Get latest block
GET /api/v1/chains/{chain}/block/latest

# Get transaction
GET /api/v1/chains/{chain}/tx/{hash}

# Get gas price
GET /api/v1/chains/{chain}/gas-price

# Get nonce
GET /api/v1/chains/{chain}/address/{address}/nonce

# Raw RPC query
POST /api/v1/chains/{chain}/query
```

### Batch Queries Across Chains

```bash
POST /api/v1/batch
```

Request:

```json
{
  "ethereum": [
    { "jsonrpc": "2.0", "method": "eth_blockNumber", "params": [], "id": 1 }
  ],
  "bsc": [
    { "jsonrpc": "2.0", "method": "eth_blockNumber", "params": [], "id": 1 }
  ],
  "arbitrum": [
    { "jsonrpc": "2.0", "method": "eth_gasPrice", "params": [], "id": 1 }
  ]
}
```

## Adding New EVM Chains

### Step 1: Update Configuration

Add the new chain to `pkg/config/config.go` in the `LoadChainsConfig()` function:

```go
// New EVM Chain
{
    Name:        "newchain",
    DisplayName: "New Chain",
    NativeToken: "NEW",
    Decimals:    18,
    ChainID:     12345,
    Type:        "evm",
    RPCURL:      GetStringEnv("NEWCHAIN_RPC_URL", "wss://rpc.newchain.com/ws"), // Example with WSS
    Explorer:    "https://explorer.newchain.com",
    Enabled:     GetBoolEnv("NEWCHAIN_ENABLED", true),
},
```

### Step 2: Add Environment Variables

```bash
NEWCHAIN_RPC_URL=wss://rpc.newchain.com/ws
NEWCHAIN_ENABLED=true
```

### Step 3: Test

The new chain will automatically be available through all API endpoints!

## RPC Provider Options

This section lists some public and premium RPC providers. Many providers offer both HTTPS and WSS endpoints. Check your provider's documentation for available WSS URLs.

### Free Public RPCs

- **Ethereum**: `https://ethereum.publicnode.com` (HTTPS)
- **BSC**: `https://bsc-dataseed.binance.org` (HTTPS)
- **Polygon**: `https://polygon-bor-rpc.publicnode.com` (HTTPS)
- **Arbitrum**: `https://arb1.arbitrum.io/rpc` (HTTPS)

### Premium Providers

Many premium providers offer WSS endpoints for lower latency connections.

- **Alchemy**: `https://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY` (HTTPS), `wss://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY` (WSS)
- **Infura**: `https://mainnet.infura.io/v3/YOUR_PROJECT_ID` (HTTPS), `wss://mainnet.infura.io/ws/v3/YOUR_PROJECT_ID` (WSS)
- **QuickNode**: Typically provide specific WSS URLs with your API key.
- **Ankr**: `https://rpc.ankr.com/eth` (HTTPS), often provide WSS alternatives.

## Best Practices

### Development

- Use free public RPCs for development (HTTPS or WSS if available).
- Enable debug logging: `LOG_LEVEL=debug`
- Test with multiple chains to ensure compatibility.

### Production

- Use dedicated RPC providers (Alchemy, Infura, QuickNode), preferably WSS for performance if supported and stable.
- Implement RPC endpoint monitoring.
- Set up fallback RPC endpoints (could be HTTPS if WSS fails).
- Monitor rate limits and costs.
- Use load balancers for high availability.

### Security

- Keep RPC API keys secure.
- Use environment variables for sensitive data.
- Implement proper rate limiting.
- Monitor for unusual activity.

## Troubleshooting

### Chain Not Available

1. Check if the chain is enabled: `{CHAIN}_ENABLED=true`
2. Verify RPC URL is accessible (try with `curl` for HTTPS or a WebSocket client for WSS).
3. Check logs for connection errors.

### RPC Errors

1. Verify API key is correct.
2. Check rate limits with your provider.
3. Test RPC endpoint directly (HTTPS or WSS).
4. Switch to alternative RPC provider or scheme (e.g., HTTPS if WSS is problematic).

### Performance Issues

1. Use dedicated RPC providers. WSS may offer better performance for frequent requests.
2. Implement caching.
3. Monitor response times.
4. Consider geographic proximity to RPC servers.

## Chain-Specific Notes

### BNB Smart Chain (BSC)

- Very fast block times (~3 seconds)
- Low transaction fees
- High throughput
- Compatible with Ethereum tooling

### Arbitrum

- Optimistic rollup technology
- Lower fees than Ethereum
- Fast finality
- Full EVM compatibility

### Optimism

- Optimistic rollup
- Ethereum-equivalent
- Growing ecosystem
- OP token governance

### Base

- Built by Coinbase
- OP Stack based
- Growing rapidly
- Strong institutional backing

### Avalanche C-Chain

- Subnet architecture
- Fast finality
- High throughput
- AVAX native token

## Future Enhancements

- Automatic RPC endpoint health checking (for both HTTPS and WSS).
- Dynamic RPC endpoint switching.
- Chain-specific configuration options.
- Testnet support for all chains.
- Custom gas price strategies per chain.
- Chain-specific rate limiting.

## Support

For issues or questions about EVM chain configuration:

1. Check the logs for error messages.
2. Verify environment variables are set correctly.
3. Test RPC endpoints independently (HTTPS or WSS).
4. Consult the API documentation.
5. Open an issue on GitHub.
