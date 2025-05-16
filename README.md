# Blockchain Gateway

A high-performance Go API gateway for interacting with multiple blockchain networks through a unified interface.

## Overview

Blockchain Gateway provides a simplified way to interact with various blockchain RPC endpoints through a single, consistent API. It abstracts away the differences between blockchain implementations, allowing developers to focus on building their applications rather than managing multiple RPC connections and protocols.

## Features

- ✅ Support for multiple blockchains (Ethereum, Bitcoin, Polygon, and more to come)
- ✅ Unified API for all blockchain interactions using the Gin framework
- ✅ Batch requests across multiple chains
- ✅ Convenient REST endpoints for common blockchain operations
- ✅ Built-in request rate limiting and throttling
- ✅ Automatic retries for failed requests
- ✅ High performance with concurrent processing
- ✅ Health checks and monitoring endpoints
- ✅ Configurable timeouts and connection pooling

## Installation

### Prerequisites

- Go 1.24.3 or higher
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
# Install Air (specific version for compatibility)
make install-air
# Or manually install: go install github.com/air-verse/air@latest

# Run with Air for live-reloading
make dev
```

With Air running, any changes to your Go files will automatically trigger a rebuild and restart of the server. The project includes a pre-configured `.air.toml` file with appropriate settings.

If you encounter any issues with Air configuration:

- Check that you're using the compatible version (v1.40.4)
- Refer to `.air.example.toml` for a detailed configuration reference
- Try running with the `GIN_MODE=debug` environment variable for more verbose output

## Using Docker

### Quick Start with Docker

```bash
# Build the Docker image
docker build -t blockchain-gateway .

# Run the container
docker run -p 8080:8080 blockchain-gateway
```

### Development with Docker Compose

For development with hot-reloading, we provide a Docker Compose configuration:

```bash
# Start the development environment with hot-reloading
docker-compose up blockchain-gateway-dev

# Access the API at http://localhost:8080
```

This development setup:

- Mounts your local code into the container
- Uses Air for hot-reloading
- Sets Gin to debug mode for detailed logs
- Shares Go module cache between runs for faster builds

### Production Docker Setup

For a production-like environment:

```bash
# Start the production container
docker-compose up blockchain-gateway-prod

# Access the API at http://localhost:8081
```

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

# Install Air (specific version v1.40.4)
make install-air

# Run with live-reloading (Air)
make dev
```

## Configuration

Configuration can be provided via environment variables:

| Variable                | Description                                           | Default                            |
| ----------------------- | ----------------------------------------------------- | ---------------------------------- |
| `PORT`                  | Server port                                           | `8080`                             |
| `GIN_MODE`              | Gin mode (debug, release)                             | `release`                          |
| `LOG_LEVEL`             | Logging level (debug, info, warn, error)              | `info`                             |
| `REQUEST_TIMEOUT`       | Default request timeout in seconds for RPC client     | `30`                               |
| `ETH_RPC_URL`           | Ethereum RPC URL                                      | `https://ethereum.publicnode.com`  |
| `BTC_RPC_URL`           | Bitcoin RPC URL                                       | `https://btc.getblock.io/mainnet`  |
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

Development mode (`GIN_MODE=debug`) provides:

- Detailed request/response logging
- Additional debug endpoints
- Extended timeouts for easier debugging
- `/debug/routes` endpoint showing all registered routes

## API Usage

The API offers both raw JSON-RPC access and convenient REST endpoints for common operations.

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
      "jsonrpc": "1.0",
      "result": 800000,
      "id": 1
    }
  ]
}
```

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
- `orderBy` (string, optional, default: `market_cap_rank`): Column to sort by. Allowed: `market_cap_rank`, `name`, `current_price`, `last_updated`, `data_fetched_at`.
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

## Health Check

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
- **API Layer** - Provides a RESTful interface for interacting with the gateway
- **Request Pipeline** - Handles request validation, processing, and response formatting

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
