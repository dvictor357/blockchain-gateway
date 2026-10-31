# Blockchain Gateway

A high-performance Go API gateway for interacting with multiple blockchain networks through a unified interface.

## Overview

Blockchain Gateway provides a simplified way to interact with various blockchain RPC endpoints through a single, consistent API. It abstracts away the differences between blockchain implementations, allowing developers to focus on building their applications rather than managing multiple RPC connections and protocols.

## Features

- ✅ Support for multiple blockchains (Ethereum, Bitcoin, Polygon, and more to come)
- Unified API for all blockchain interactions using the [Blaze framework](https://github.com/dvictor357/blaze)
- ✅ Batch requests across multiple chains
- ✅ Convenient REST endpoints for common blockchain operations
- ✅ Per-client request rate limiting (by socket peer address)
- ✅ Strict validation of upstream JSON-RPC responses; node errors keep their code and data
- ✅ Chains are queried concurrently within a batch (requests on one chain run in order)
- ✅ Liveness endpoint (`/health`) and Swagger documentation
- ✅ Configurable request timeout and HTTP connection reuse
- ✅ Market data cached in PostgreSQL with tracked, transactional migrations

The gateway does **not** retry failed upstream requests: a failure is reported to the caller as it happens.

## Installation

### Prerequisites

- Go 1.25.5 or higher (required by Blaze)
- Git

# Building from source

```bash
# Clone the repository
git clone https://github.com/dvictor357/blockchain-gateway.git
cd blockchain-gateway

# Build the project
go build -o blockchain-gateway ./cmd/server

# Run the server
./blockchain-gateway
```

## Development with Air (Live Reload)

This project includes configuration for [Air](https://github.com/air-verse/air), which provides live-reloading during development.

```bash
# Install Air (pinned version, built with Go 1.25)
make install-air
# Or manually install: go install github.com/air-verse/air@v1.63.0

# Run with Air for live-reloading
make dev
```

With Air running, any changes to your Go files will automatically trigger a rebuild and restart of the server. The project includes a pre-configured `.air.toml` file with appropriate settings.

If you encounter any issues with Air configuration:

- Check that you're using the pinned version (v1.63.0)
- Refer to `.air.example.toml` for a detailed configuration reference
- Try running with the `APP_MODE=debug` environment variable for debug routes and extended timeouts

## Using Docker

### Quick Start with Docker

The gateway needs PostgreSQL. The image contains the binary and the SQL migrations, and **no configuration**: `.env*` files are excluded from the build context (`.dockerignore`) and never baked into layers. Supply settings as environment variables; a `.env` file is optional, and starting from environment variables alone works.

```bash
# Build the production image
docker build -t blockchain-gateway .

# Run it against an existing PostgreSQL server (all settings come from the environment)
docker run -p 8080:8080 \
  -e DB_HOST=db.example.internal -e DB_USER=gateway -e DB_PASSWORD=... -e DB_NAME=blockchain_gateway \
  blockchain-gateway
```

Migrations are applied automatically at startup from `./migrations` inside the image.

### Docker Compose (PostgreSQL included)

`docker-compose.yml` runs a real PostgreSQL service (`postgres:16-alpine`) with a persistent named volume (`postgres-data`) and a health check; the gateway services wait for it to be healthy. The gateway reaches it at `DB_HOST=postgres`, `DB_PORT=5432` (the database's internal port).

The gateway services are in the `dev` and `prod` profiles, so a bare `docker-compose up` starts only PostgreSQL and never two market-data fetchers against one database by accident. Both of these forms work:

```bash
# Development with hot-reloading (http://localhost:8080)
docker-compose up blockchain-gateway-dev        # or: docker-compose --profile dev up

# Production-like (http://localhost:8081)
docker-compose up blockchain-gateway-prod       # or: docker-compose --profile prod up
```

Database credentials interpolate from `DB_USER`, `DB_PASSWORD` and `DB_NAME` (shell environment or a Compose-level `.env`). The built-in defaults are **for local development only**; set your own values for any shared or production deployment. PostgreSQL applies them only when the data volume is first created.

The development setup mounts your local code, uses Air for hot-reloading, runs in debug mode, and shares the Go module cache between runs.

**Only one background market-data fetcher should run against a database.** When running more than one replica, set `MARKET_FETCHER_ENABLED=false` on all but one; those replicas still serve `/api/v1/markets` from the shared tables.

### Docker Build Options

The Dockerfile includes multiple build targets:

```bash
# Build development image
docker build --target development -t blockchain-gateway:dev .

# Build production image
docker build --target production -t blockchain-gateway:prod .
```

### Using Makefile

The project includes a Makefile with several helpful commands:

```bash
# Show all available commands
make help

# Build the application
make build

# Run tests
make test

# Format code
make fmt

# Install Air (pinned version v1.63.0)
make install-air

# Run with live-reloading (Air)
make dev
```

## Configuration

Configuration can be provided via environment variables:

| Variable                | Description                                           | Default                            |
| ----------------------- | ----------------------------------------------------- | ---------------------------------- |
| `PORT`                  | Server port                                           | `8080`                             |
| `APP_MODE`              | Application mode (`debug` enables debug routes)       | `release`                          |
| `LOG_LEVEL`             | Logging level (debug, info, warn, error)              | `info`                             |
| `REQUEST_TIMEOUT`       | Whole seconds, 1–120. Upstream HTTP call timeout and per-request budget (batches get twice this); invalid values stop startup | `30` |
| `RATE_LIMIT`            | Requests per minute per client socket address         | `120`                              |
| `ETH_RPC_URL`           | Ethereum RPC URL                                      | `https://ethereum.publicnode.com`  |
| `BTC_RPC_URL`           | Bitcoin RPC URL; may embed `user:password@` basic-auth credentials. Gateway transport errors omit the request URL. | `https://btc.getblock.io/mainnet` |
| `MARKET_FETCHER_ENABLED` | Run the background CoinGecko fetcher (keep it on for exactly one instance per database) | `true`        |
| `DB_HOST`               | PostgreSQL host                                       | `localhost`                        |
| `DB_PORT`               | PostgreSQL port                                       | `5432`                             |
| `DB_USER`               | PostgreSQL user                                       | `postgres`                         |
| `DB_PASSWORD`           | PostgreSQL password                                   |                                    |
| `DB_NAME`               | PostgreSQL database name                              | `blockchain_gateway`               |
| `DB_SSLMODE`            | PostgreSQL SSL mode                                   | `disable`                          |
| `COINGECKO_BASE_URL`    | CoinGecko API base URL (optional)                     | `https://api.coingecko.com/api/v3` |
| `COINGECKO_PER_PAGE`    | Number of coins to fetch per CoinGecko API call       | `100`                              |
| `COINGECKO_ORDER`       | CoinGecko API order parameter (e.g., market_cap_desc) | `market_cap_desc`                  |
| `COINGECKO_VS_CURRENCY` | CoinGecko API vs_currency parameter                   | `usd`                              |

The HTTP server's write timeout is the longest request budget plus a 10-second write margin; slow connections may still time out. Database passwords may contain spaces, quotes and backslashes.

Development mode (`APP_MODE=debug`) provides:

- Detailed request/response logging
- Additional debug endpoints
- Extended timeouts for easier debugging
- `/debug/routes` endpoint showing all registered routes

## API Usage

The API offers both raw JSON-RPC access and convenient REST endpoints for common operations.

### Security: the raw RPC endpoints are an unauthenticated passthrough

`POST /api/v1/chains/{chain}/query` and `POST /api/v1/batch` forward **any** JSON-RPC method to the configured node, without authentication and without a method allow-list. The gateway adds none by design. Therefore:

- Run it only on a trusted network, or behind a reverse proxy/API gateway that authenticates callers and restricts methods and paths.
- Anyone who can reach it can spend the quota of the RPC provider keys embedded in your RPC URLs, and can call whatever the node exposes (for example wallet, admin or debug methods, or broadcasting transactions). Use nodes and keys that are safe to expose that way.
- Rate limiting identifies clients by socket peer address only; forwarding headers are ignored.

### Limits and error responses

- **Request body:** at most **1 MiB** (1,048,576 bytes), measured over the whole body, and it must contain exactly one JSON value (trailing whitespace is fine). Larger bodies return `413 REQUEST_TOO_LARGE`; trailing content returns `400 INVALID_JSON`.
- **Batch size:** at most **10 RPC requests in total across all chains** (exactly 10 is accepted, 11 is rejected with `400 BATCH_TOO_LARGE`). A batch with no requests returns `400 EMPTY_BATCH`. Rejected batches contact no node. Requests for one chain run sequentially; chains run concurrently.
- **Timeouts:** each request has a budget of `REQUEST_TIMEOUT` seconds (batches: twice that). When it is exhausted in a batch, the remaining requests are answered with an error entry without contacting the node.

Errors use the envelope `{"error": "...", "code": "...", "details": [...]}`:

| Status | Code | Meaning |
| ------ | ---- | ------- |
| 400 | `VALIDATION_ERROR`, `INVALID_JSON`, `INVALID_LIMIT`, `INVALID_OFFSET`, `EMPTY_BATCH`, `BATCH_TOO_LARGE`, `NOT_APPLICABLE`, `INVALID_REQUEST` | Bad input, or the operation does not apply to the chain (for example gas price on Bitcoin) |
| 404 | `CHAIN_NOT_SUPPORTED`, `NOT_FOUND` | Unknown or disabled chain; resource does not exist |
| 413 | `REQUEST_TOO_LARGE` | Body over the limit |
| 429 | – | Rate limit exceeded |
| 501 | `OPERATION_NOT_SUPPORTED` | Operation the gateway cannot provide (Bitcoin address balance) |
| 502 | `UPSTREAM_RPC_ERROR` | The node returned a JSON-RPC error object |
| 502 | `UPSTREAM_ERROR` | Transport failure, non-2xx HTTP status, or an invalid/mismatched/malformed response from the node |
| 504 | `RPC_TIMEOUT`, `REQUEST_TIMEOUT` | The request deadline or HTTP client timeout expired |
| 500 | `INTERNAL_ERROR` | Anything else, including a client that cancelled its own request |

Upstream URLs, credentials and response bodies are never included in error responses. Upstream responses are validated strictly: the JSON-RPC version (Bitcoin's legacy envelope is accepted), a matching numeric `id`, exactly one of `result`/`error`, and well-formed error objects. `"result": null` from a node is a valid result for raw queries. Malformed numeric results (hex quantities, BTC amounts) are rejected as `UPSTREAM_ERROR` rather than treated as zero.

Inside a batch response, a node's JSON-RPC error keeps its original `code`, `message` and `data`; transport and protocol failures appear as `{"code": -32000, "message": "<generic text>"}`.

**Behavior changes to be aware of:** upstream HTTP 5xx and invalid envelopes now return 502 (previously they could succeed or return 500); timeouts return 504; Bitcoin address balance returns 501 (it used to return the node wallet's total balance); the batch limit now counts RPCs rather than chains; request bodies over 1 MiB or with trailing content are rejected; EVM chains such as BSC and Base now require well-formed EVM addresses and `0x`-prefixed 32-byte hashes.

### Raw JSON-RPC Access

#### List Supported Blockchains

```
GET /api/v1/chains
```

Response:

```json
{
  "chains": ["ethereum", "bitcoin", "polygon"],
  "count": 3
}
```

#### Query a Blockchain

```
POST /api/v1/chains/{chain}/query
```

Request Body:

```json
{
  "jsonrpc": "2.0",
  "method": "eth_blockNumber",
  "params": [],
  "id": 1
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "result": "0x1234567",
  "id": 1
}
```

#### Batch Query (Multiple Chains)

```
POST /api/v1/batch
```

Request Body:

```json
{
  "ethereum": [
    {
      "jsonrpc": "2.0",
      "method": "eth_blockNumber",
      "params": [],
      "id": 1
    },
    {
      "jsonrpc": "2.0",
      "method": "eth_getBalance",
      "params": ["0x742d35Cc6634C0532925a3b844Bc454e4438f44e", "latest"],
      "id": 2
    }
  ],
  "bitcoin": [
    {
      "jsonrpc": "1.0",
      "method": "getblockcount",
      "params": [],
      "id": 1
    }
  ]
}
```

Response:

```json
{
  "ethereum": [
    {
      "jsonrpc": "2.0",
      "result": "0x1234567",
      "id": 1
    },
    {
      "jsonrpc": "2.0",
      "result": "0x1a2b3c4d5e6f",
      "id": 2
    }
  ],
  "bitcoin": [
    {
      "jsonrpc": "2.0",
      "result": 800000,
      "id": 1
    }
  ]
}
```

Batch entries always use `"jsonrpc": "2.0"` regardless of the chain. See [Limits and error responses](#limits-and-error-responses) for the batch size limit.

### Convenience Endpoints

The API provides several convenient REST endpoints for common blockchain operations.

#### Get Account Balance

```
GET /api/v1/chains/{chain}/address/{address}/balance
```

Example:

```
GET /api/v1/chains/ethereum/address/0x742d35Cc6634C0532925a3b844Bc454e4438f44e/balance
```

Response:

```json
{
  "address": "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
  "balance": "1242500000000000000",
  "hex_balance": "0x113f4fbc91cb0000",
  "decimals": 18,
  "symbol": "ETH",
  "chain": "ethereum"
}
```

**Bitcoin is not supported** by this endpoint: it returns `501 OPERATION_NOT_SUPPORTED` without querying the node. Looking up a balance by address needs an address index, and none is configured (the gateway adds no external address provider). Raw `getbalance` through `/query` remains available, but it reports the connected node's **wallet** balance, not an arbitrary address's, and requires a node with a wallet.

#### Get Latest Block

```
GET /api/v1/chains/{chain}/block/latest
```

Example:

```
GET /api/v1/chains/ethereum/block/latest
```

Response:

```json
{
  "number": 18934257,
  "hash": "0x8d12a0d346a05cf0dd9e650a5e41baa531a2ef7a287572739ce5c5a36856ec7c",
  "parent_hash": "0x781d36b32c7cbf06d952baa1d827eb425bacfdf9c9afc30b735959054a3f2fc1",
  "timestamp": 1716403465,
  "transaction_count": 124,
  "chain": "ethereum"
}
```

#### Get Transaction Details

```
GET /api/v1/chains/{chain}/tx/{hash}
```

Example:

```
GET /api/v1/chains/ethereum/tx/0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71
```

Response:

```json
{
  "hash": "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
  "from": "0x95222290dd7278aa3ddd389cc1e1d165cc4bafe5",
  "to": "0xdef1c0ded9bec7f1a1670819833240f027b25eff",
  "value": "500000000000000000",
  "block_number": 18934220,
  "block_hash": "0x90e1a8e935cfd5970d6789a7afedb1dac09af91a7b8fc7dbe16008116ab19f9c",
  "status": "success",
  "chain": "ethereum"
}
```

For Bitcoin, `value` is the exact sum of all transaction outputs in satoshis (not the amount "sent"; Bitcoin has no from/to), computed without floating point. An amount that is malformed, negative or not a whole number of satoshis is rejected as `UPSTREAM_ERROR`.

#### Get Gas Price

```
GET /api/v1/chains/{chain}/gas-price
```

Example:

```
GET /api/v1/chains/ethereum/gas-price
```

Response:

```json
{
  "chain": "ethereum",
  "gas_price": "20000000000",
  "gas_price_hex": "0x4a817c800"
}
```

#### Get Transaction Count (Nonce)

```
GET /api/v1/chains/{chain}/address/{address}/nonce
```

Example:

```
GET /api/v1/chains/ethereum/address/0x742d35Cc6634C0532925a3b844Bc454e4438f44e/nonce
```

Response:

```json
{
  "chain": "ethereum",
  "address": "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
  "transaction_count": 42,
  "nonce": "0x2a"
}
```

#### Get Coin Markets

Retrieves a paginated list of coin market data, fetched periodically from CoinGecko and stored locally.

`GET /api/v1/markets`

Query Parameters:

- `limit` (int, optional, default: 20, max: 100): Number of records per page.
- `offset` (int, optional, default: 0): Number of records to skip for pagination.
- `orderBy` (string, optional, default: `market_cap_rank`): Column to sort by, case-insensitive. Allowed: `market_cap_rank`, `name`, `current_price`, `last_updated`, `data_fetched_at`. Ties are broken by `id`, so paging is stable. Unranked coins sort last ascending and first descending.
- `sortDirection` (string, optional, default: `asc`): Sort direction. Allowed: `asc`, `desc`.

Example:
`GET /api/v1/markets?limit=10&orderBy=current_price&sortDirection=desc`

Response:

```json
{
  "data": [
    {
      "id": "bitcoin",
      "symbol": "btc",
      "name": "Bitcoin",
      "image": "https://assets.coingecko.com/coins/images/1/large/bitcoin.png?1696501400",
      "current_price": 67000.0,
      "market_cap": 1320000000000,
      "market_cap_rank": 1,
      // ... other fields ...
      "last_updated": "2024-05-28T10:00:00Z",
      "data_fetched_at": "2024-05-28T10:01:05Z"
    }
    // ... more coins
  ],
  "pagination": {
    "total_records": 100,
    "limit": 10,
    "offset": 0,
    "current_page": 1,
    "total_pages": 10
  },
  "meta": {
    "last_data_update_from_source": "2024-05-28T10:01:05.123456789Z"
  }
}
```

The page, `total_records` and `last_data_update_from_source` come from a single database statement, so they are consistent with each other even while a refresh runs; an empty table or an offset past the last row still reports the total and timestamp (with `data` set to `null`). The query is bounded by the request timeout (`504 REQUEST_TIMEOUT` if exceeded).

## Health Check

`/health` is a **liveness** check only: it reports that the process is serving HTTP. It does not check PostgreSQL or any upstream node, and is not a readiness probe.

```
GET /health
```

Response:

```json
{
  "status": "ok",
  "time": "2023-05-15T14:30:45Z"
}
```

## Architecture

Blockchain Gateway is built with a modular architecture that makes it easy to add support for new blockchains:

- **Client Manager** - Manages connections to different blockchain networks
- **Blockchain Clients** - Implements blockchain-specific RPC protocols
- **Blaze API Layer** - Provides a RESTful interface for interacting with the gateway
- **Request Pipeline** - Handles request validation, processing, and response formatting

### Blaze integration

The gateway dogfoods Blaze at pinned revision `e6059b31909a` (`v0.0.0-20251217150610-e6059b31909a`). `api.NewRouter` exposes the native Blaze routes as an `http.Handler`, retaining the existing API paths, response data, validation errors, and graceful HTTP-server shutdown.

- Shared Blaze middleware wraps the HTTP boundary, so unmatched routes are also logged and rate-limited. The limiter releases its lock before downstream RPC or database work.
- Rate limits and logs identify clients by their socket peer, not caller-supplied forwarding headers. Behind a reverse proxy, clients share the proxy's bucket; forwarding headers are not a trusted client-identity source.
- RPC request decoding preserves numeric parameters and IDs without `float64` rounding.
- Blaze JSON responses use `application/json` and include a trailing newline. Trailing slashes match the same API route instead of redirecting.
- Swagger remains available at `/swagger/index.html`, with the generated specification at `/swagger/doc.json` and bundled assets served locally. No Gin Swagger adapter or AI-tool endpoints are included.
- Handler tests exercise the production implementation through Blaze routes. Run `go test -race -cover ./pkg/api` for API and middleware regressions.

### Startup, migrations and shutdown

- A `.env` file is optional; a missing file is ignored, a malformed one stops startup. Real environment variables take precedence over `.env`.
- Migrations (`migrations/NNN_description.sql`) run in numeric order at startup. Applied versions are recorded in `schema_migrations`; each migration runs in its own transaction under a PostgreSQL advisory lock, so concurrent instances apply each exactly once and a failed migration leaves no partial changes. Databases created before tracking existed are adopted by re-running the idempotent initial script. Migration files must not contain their own `BEGIN`/`COMMIT`.
- The database pool is bounded (10 open connections) and the initial connection attempt times out after 10 seconds.
- The market-data fetch runs every minute with a 50-second deadline and never overlaps itself. Run exactly one fetcher per database (`MARKET_FETCHER_ENABLED`).
- On SIGINT/SIGTERM the fetcher is cancelled, HTTP connections drain (up to 30 seconds), and the process waits for the fetcher to stop before closing the database.

### Testing

`go test -race ./...` runs the unit tests. Database integration tests run against a disposable PostgreSQL server when `GATEWAY_TEST_DATABASE_URL` (a `postgres://` URL) is set, each in its own temporary schema, and are skipped otherwise.


## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
