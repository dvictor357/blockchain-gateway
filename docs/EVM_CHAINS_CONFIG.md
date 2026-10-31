# Chain configuration

The code configures **eight EVM chains plus Bitcoin**, not just Ethereum, Bitcoin, and Polygon. Seven EVM chains and Bitcoin are enabled by default (eight clients total). Fantom is disabled by default; enabling it produces nine clients. `GET /api/v1/chains` returns enabled client names, not upstream health.

## Built-in configuration

| URL name | Chain ID | Token | RPC override | Default RPC | Enabled switch / default |
| --- | --- | --- | --- | --- | --- |
| `ethereum` | 1 | ETH | `ETH_RPC_URL` | `https://ethereum.publicnode.com` | Always enabled |
| `polygon` | 137 | POL | `POLYGON_RPC_URL` | `https://polygon-bor-rpc.publicnode.com` | Always enabled |
| `bsc` | 56 | BNB | `BSC_RPC_URL` | `https://bsc-dataseed.binance.org` | `BSC_ENABLED=true` |
| `arbitrum` | 42161 | ETH | `ARBITRUM_RPC_URL` | `https://arb1.arbitrum.io/rpc` | `ARBITRUM_ENABLED=true` |
| `optimism` | 10 | ETH | `OPTIMISM_RPC_URL` | `https://mainnet.optimism.io` | `OPTIMISM_ENABLED=true` |
| `base` | 8453 | ETH | `BASE_RPC_URL` | `https://mainnet.base.org` | `BASE_ENABLED=true` |
| `avalanche` | 43114 | AVAX | `AVALANCHE_RPC_URL` | `https://api.avax.network/ext/bc/C/rpc` | `AVALANCHE_ENABLED=true` |
| `fantom` | 250 | FTM | `FANTOM_RPC_URL` | `https://rpc.ftm.tools` | `FANTOM_ENABLED=false` |
| `bitcoin` | — | BTC | `BTC_RPC_URL` | `https://btc.getblock.io/mainnet` | Always enabled |

These are configured adapters, not a guarantee that public providers are available or free. Fantom is an opt-in legacy network; verify a suitable endpoint before enabling it. The Bitcoin example endpoint may require credentials. Use your own provider or node where necessary; keep credentials in runtime environment variables, not committed files.

```sh
BSC_ENABLED=false FANTOM_ENABLED=true go run ./cmd/server
```

With no overrides, `/api/v1/chains` returns:

```json
{
  "chains": ["ethereum", "polygon", "bsc", "arbitrum", "optimism", "base", "avalanche", "bitcoin"],
  "count": 8
}
```

## Reads

All enabled EVM chains use the same generic EVM client and expose balance, latest block, transaction, gas-price, and nonce REST reads. Raw RPC calls use the same allowlist across chains:

```sh
curl -sS http://localhost:8080/api/v1/chains/base/query \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
```

Add `X-API-Key` when `API_KEY` is configured. A configured `RPC_METHOD_ALLOWLIST` replaces the read-only defaults and also restricts methods used by REST helpers. See [README safety and access](../README.md#safety-and-access).

Bitcoin supports raw node reads, latest block, and transaction lookup (subject to node indexing). Without an address index, its balance endpoint returns HTTP 501. Gas price and account nonce do not apply to Bitcoin. Bitcoin transaction hashes omit `0x`; EVM hashes include it.

## Adding an EVM chain

Add a `ChainConfig` entry in `LoadChainsConfig` in `pkg/config/config.go`, including its name, chain ID, native token, RPC URL environment override, and enabled switch. The registry and client manager load enabled entries; the validator resolves address/hash rules from the registry. Update this table, `.env.development`, API annotations, and verification coverage together. Regenerate Swagger as described in the README.

## Troubleshooting

- Missing chain: check its enabled switch and `/api/v1/chains`.
- HTTP 403: the RPC method is not in the effective allowlist; do not enable privileged methods just to hide the error.
- Upstream errors: check provider authentication, quotas, network, and whether the node supports the method. The gateway does not retry or fail over.
- Slow queries: keep `REQUEST_TIMEOUT` within 1–120 seconds and constrain expensive reads at your provider or proxy.
