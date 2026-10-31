# Blockchain Gateway

[![CI](https://github.com/dvictor357/blockchain-gateway/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/dvictor357/blockchain-gateway/actions/workflows/ci.yml)

A tiny, read-only, agent-friendly chain API. Not a replacement for eRPC.

A Go REST/JSON-RPC gateway built with [Blaze](https://github.com/dvictor357/blaze). Small enough to read, self-host, and use from a script or an agent: query balances, blocks, transactions, gas prices, or batch reads across chains. This is a portfolio project, not a production RPC routing platform.

Need this set up or customized? https://x.com/dvictor3579

## Quickstart

Go 1.25.5 or newer:

```sh
git clone https://github.com/dvictor357/blockchain-gateway.git
cd blockchain-gateway
go run ./cmd/server
```

Or Docker, with no database or configuration file required:

```sh
docker build -t blockchain-gateway .
docker run --rm -p 8080:8080 blockchain-gateway
```

In another terminal:

```sh
curl -sS http://localhost:8080/health
curl -sS http://localhost:8080/api/v1/chains

# Raw read; result is the node's JSON-RPC value.
curl -sS http://localhost:8080/api/v1/chains/ethereum/query \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# REST read; wei is returned as a decimal string.
curl -sS http://localhost:8080/api/v1/chains/ethereum/address/0x0000000000000000000000000000000000000000/balance

# Reads across chains; maximum 10 RPC requests total by default.
curl -sS http://localhost:8080/api/v1/batch \
  -H 'Content-Type: application/json' \
  -d '{"ethereum":[{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}],"polygon":[{"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":2}]}'
```

`/health` is process liveness, not upstream or database readiness. Chain reads require working RPC endpoints; public providers may throttle, require credentials, or stop serving. Supply your own endpoints for dependable use. The built-in Bitcoin endpoint may require provider credentials.

## Chains and endpoints

The implementation has **eight EVM adapters plus Bitcoin**. Defaults expose eight chains: seven EVM chains and Bitcoin. Fantom is opt-in, so enabling it gives nine total. See [chain configuration](docs/EVM_CHAINS_CONFIG.md) for IDs, endpoint defaults, and switches.

| Chain IDs used in URLs | Default |
| --- | --- |
| `ethereum`, `polygon`, `bsc`, `arbitrum`, `optimism`, `base`, `avalanche`, `bitcoin` | Enabled |
| `fantom` | Disabled |

`GET /api/v1/chains` reports the configured clients, not a live availability check. All EVM adapters share the same read operations; this is not a claim that every public node is currently healthy.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/health` | Liveness |
| GET | `/swagger/index.html` | Interactive API reference |
| GET | `/api/v1/chains` | Enabled chains |
| POST | `/api/v1/chains/{chain}/query` | Allowlisted JSON-RPC call |
| POST | `/api/v1/batch` | Allowlisted calls grouped by chain |
| GET | `/api/v1/chains/{chain}/address/{address}/balance` | Native balance (EVM only) |
| GET | `/api/v1/chains/{chain}/block/latest` | Latest block |
| GET | `/api/v1/chains/{chain}/tx/{hash}` | Transaction details |
| GET | `/api/v1/chains/{chain}/gas-price` | Gas price (EVM only) |
| GET | `/api/v1/chains/{chain}/address/{address}/nonce` | Nonce (EVM only) |
| GET | `/api/v1/markets` | Stored CoinGecko market data, when database enabled |

Bitcoin has no configured address index: balance returns HTTP 501 (`OPERATION_NOT_SUPPORTED`); gas price and nonce are not applicable. Raw Bitcoin reads depend on node indexing and permissions. REST EVM hashes include `0x`; Bitcoin hashes do not.

## Safety and access

By default, raw RPC calls use a finite read-only allowlist ([exact defaults](pkg/config/security.go)). Unknown methods, `admin_*`, `personal_*`, `debug_*`, signing, and transaction-send methods are denied unless explicitly named in your configured allowlist. Mixed batches are rejected before any upstream work when any method is denied. Read-only is a method policy, not a cost limit: `eth_call` and log queries can still be expensive.

Set `API_KEY` to require the `X-API-Key` header. Leading or trailing whitespace in a configured key is rejected at startup, not silently trimmed. Never put a real key in a checked-in file, command history, or URL. For example, after securely exporting `API_KEY` in your shell:

```sh
# The server inherits API_KEY.
go run ./cmd/server
# In a shell with the same securely supplied API_KEY:
curl -sS -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/chains
```

Set `RPC_METHOD_ALLOWLIST` to a comma-separated list of **exact, case-sensitive names** to replace the defaults, not append to them. Unset or empty uses the defaults; malformed lists fail startup:

```sh
RPC_METHOD_ALLOWLIST=eth_chainId,eth_blockNumber,eth_getBalance go run ./cmd/server
```

There are no wildcards. Explicitly enabling a send, wallet, admin, or debug method removes the read-only guarantee for that method; use an isolated node and access controls. Restrictive policies also apply to the RPC methods used by REST helpers. With `API_KEY` set, every path except exact `/health` requires the header, including Swagger, debug routes, and unknown paths. Missing or invalid keys receive HTTP 401; failed authentication attempts also count toward the rate limit. Denied RPC methods receive HTTP 403 (`RPC_METHOD_NOT_ALLOWED`).

When authentication is enabled, ordinary browser navigation cannot load Swagger UI: the page, specification, and assets all require the header. Use a trusted header-injecting client/proxy or read the checked-in [JSON](docs/swagger.json) / [YAML](docs/swagger.yaml) reference. Swagger's Authorize button does not authenticate the initial page or asset requests.

Use TLS at a reverse proxy for remote access. The key is a shared secret, not user management. Socket-peer rate limiting is in-memory and per process; forwarded IP headers are not trusted. Do not expose a privileged node or provider key without quotas and network controls. The gateway does not retry, fail over, load-balance, cache chain reads, or implement WebSocket subscriptions. Upstream errors are returned rather than hidden.

## Configuration

Environment variables take precedence over an optional `.env`. `.env.development` is an example, not automatically loaded. It contains no real credentials. Docker images exclude `.env*`; inject configuration at runtime.

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | Listen port, on `0.0.0.0` |
| `APP_MODE` | `release` | `debug` also enables `/debug/routes` |
| `LOG_LEVEL` | `info` | Startup log-level label |
| `RATE_LIMIT` | `120` | Requests/minute per socket-peer IP |
| `REQUEST_TIMEOUT` | `30` | Whole seconds, 1–120; invalid values stop startup; batches get twice this budget |
| `API_KEY` | unset | Optional shared key, sent as `X-API-Key` |
| `RPC_METHOD_ALLOWLIST` | built-in read-only list | Exact method names; replaces defaults |
| `DATABASE_ENABLED` | `false` | Connect to Postgres, migrate, and serve stored markets |
| `MARKET_FETCHER_ENABLED` | `false` | Fetch CoinGecko data; requires `DATABASE_ENABLED=true` |
| `DB_HOST`, `DB_PORT` | `localhost`, `5432` | Used only when database enabled |
| `DB_USER`, `DB_PASSWORD` | `postgres`, empty | Supply credentials securely |
| `DB_NAME`, `DB_SSLMODE` | `blockchain_gateway`, `disable` | Use appropriate TLS settings outside local development |
| `COINGECKO_BASE_URL` | `https://api.coingecko.com/api/v3` | Used only by enabled fetcher |
| `COINGECKO_PER_PAGE` | `100` | Coins per fetch |
| `COINGECKO_ORDER` | `market_cap_desc` | Fetch ordering |
| `COINGECKO_VS_CURRENCY` | `usd` | Quote currency |

RPC URL overrides and chain switches are listed in [the chain guide](docs/EVM_CHAINS_CONFIG.md). RPC URLs may contain credentials; do not commit them. Request bodies are limited to 1 MiB; batches to 10 calls total. Chains run concurrently in a batch, calls within each chain sequentially. The HTTP write timeout is the longest handler budget plus ten seconds.

### Optional Postgres and market data

With both switches off, no database connection, migrations, or CoinGecko requests occur. `/api/v1/markets` returns HTTP 503 (`MARKET_DATA_DISABLED`). Database-only mode serves previously stored data without contacting CoinGecko:

```sh
# Supply DB_HOST, DB_USER, DB_PASSWORD, DB_NAME, DB_SSLMODE securely first.
DATABASE_ENABLED=true go run ./cmd/server
# Fetch and persist market data as well:
DATABASE_ENABLED=true MARKET_FETCHER_ENABLED=true go run ./cmd/server
```

Enabled startup applies migrations from `./migrations`. Run from the repository root when using Go; the Docker image includes migrations. Exactly one instance per database should run the fetcher; additional instances should use `MARKET_FETCHER_ENABLED=false`. Shutdown stops and awaits the worker before closing the database.

The Compose file includes Postgres with a health check and persistent volume, plus development (`:8080`) and production-like (`:8081`) gateway profiles. Compose intentionally enables database mode; market fetching remains opt-in:

```sh
docker compose --profile prod up --build
# Or hot reload:
docker compose --profile dev up --build
# Opt into CoinGecko on one service only:
MARKET_FETCHER_ENABLED=true docker compose --profile prod up --build
```

Compose's database credentials are local-development examples only; replace them for shared deployments. PostgreSQL initializes credentials only when its volume is first created.

## Development and verification

```sh
make install-air  # Air v1.63.0
make dev

go build ./...
go vet ./...
go test -race ./...
```

Postgres integration tests skip unless `GATEWAY_TEST_DATABASE_URL` points at a disposable PostgreSQL server. CI supplies PostgreSQL 16 and runs those tests as well as build, vet, and race checks. For local integration checks, set the URL securely, then run the same test command. Tests create isolated schemas and remove them afterwards; the database user needs schema-creation privileges. Do not use a production database.

Swagger source annotations live in `cmd/server` and `pkg/api`; regenerate the checked-in artifacts after changing API contracts:

```sh
go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g cmd/server/main.go
```

## License and release notes

[MIT](LICENSE), copyright Dimas Victor, 2026. See [CHANGELOG.md](CHANGELOG.md) for v0.1.0.
