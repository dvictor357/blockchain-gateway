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
# Chain RPC URL
{CHAIN}_RPC_URL=https://rpc-endpoint.com

# Enable/Disable chain (optional, defaults to true for most chains)
{CHAIN}_ENABLED=true
```

### Example Configuration

```bash
# BNB Smart Chain
BSC_RPC_URL=https://bsc-dataseed.binance.org
BSC_ENABLED=true

# Arbitrum
ARBITRUM_RPC_URL=https://arb1.arbitrum.io/rpc
ARBITRUM_ENABLED=true

# Base
BASE_RPC_URL=https://mainnet.base.org
BASE_ENABLED=true
```

### Default Configuration

The system comes with sensible defaults for all chains:

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
    RPCURL:      GetStringEnv("NEWCHAIN_RPC_URL", "https://rpc.newchain.com"),
    Explorer:    "https://explorer.newchain.com",
    Enabled:     GetBoolEnv("NEWCHAIN_ENABLED", true),
},
```

### Step 2: Add Environment Variables

```bash
NEWCHAIN_RPC_URL=https://rpc.newchain.com
NEWCHAIN_ENABLED=true
```

### Step 3: Test

The new chain will automatically be available through all API endpoints!

## RPC Provider Options

### Free Public RPCs

- **Ethereum**: `https://ethereum.publicnode.com`
- **BSC**: `https://bsc-dataseed.binance.org`
- **Polygon**: `https://polygon-bor-rpc.publicnode.com`
- **Arbitrum**: `https://arb1.arbitrum.io/rpc`

### Premium Providers

- **Alchemy**: `https://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY`
- **Infura**: `https://mainnet.infura.io/v3/YOUR_PROJECT_ID`
- **QuickNode**: `https://your-endpoint.quiknode.pro/YOUR_API_KEY`
- **Ankr**: `https://rpc.ankr.com/eth`

## Best Practices

### Development

- Use free public RPCs for development
- Enable debug logging: `LOG_LEVEL=debug`
- Test with multiple chains to ensure compatibility

### Production

- Use dedicated RPC providers (Alchemy, Infura, QuickNode)
- Implement RPC endpoint monitoring
- Set up fallback RPC endpoints
- Monitor rate limits and costs
- Use load balancers for high availability

### Security

- Keep RPC API keys secure
- Use environment variables for sensitive data
- Implement proper rate limiting
- Monitor for unusual activity

## Troubleshooting

### Chain Not Available

1. Check if the chain is enabled: `{CHAIN}_ENABLED=true`
2. Verify RPC URL is accessible
3. Check logs for connection errors

### RPC Errors

1. Verify API key is correct
2. Check rate limits
3. Test RPC endpoint directly
4. Switch to alternative RPC provider

### Performance Issues

1. Use dedicated RPC providers
2. Implement caching
3. Monitor response times
4. Consider geographic proximity to RPC servers

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

- Automatic RPC endpoint health checking
- Dynamic RPC endpoint switching
- Chain-specific configuration options
- Testnet support for all chains
- Custom gas price strategies per chain
- Chain-specific rate limiting

## Support

For issues or questions about EVM chain configuration:

1. Check the logs for error messages
2. Verify environment variables are set correctly
3. Test RPC endpoints independently
4. Consult the API documentation
5. Open an issue on GitHub
