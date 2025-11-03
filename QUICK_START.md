# Quick Start Guide - Blockchain Gateway Dev Branch

## 🚀 What's New in Dev Branch

We've successfully implemented **5 major improvements** to transform your blockchain gateway into a production-grade service:

### 1. ✅ **Redis-Based Rate Limiting** (pkg/resilience/rate_limiter.go)
- **Before**: In-memory only (single instance)
- **After**: Redis-distributed (multi-instance support)

### 2. ✅ **Circuit Breaker** (pkg/resilience/circuit_breaker.go)
- Protects against cascading failures
- Auto-recovery after 60s timeout

### 3. ✅ **Exponential Backoff Retry** (pkg/resilience/retry.go)
- 3 retries with exponential backoff
- Jitter to prevent thundering herd

### 4. ✅ **Optimized HTTP Client** (pkg/resilience/http_client.go)
- Connection pooling (100 max idle)
- Auto-retry for transient errors

### 5. ✅ **API Key Authentication** (pkg/auth/)
- Generate: `POST /api/v1/auth/api-keys`
- Revoke: `DELETE /api/v1/auth/api-keys/revoke`
- List: `GET /api/v1/auth/api-keys`

---

## 🏃‍♂️ Running the Gateway

### Start with Docker (Recommended)
```bash
docker-compose up -d
```

This starts:
- Redis (port 6379)
- Blockchain Gateway (port 8080)

### Manual Start
```bash
# Start Redis
redis-server

# In another terminal, start the gateway
go run ./cmd/server
```

---

## 🔑 Using API Keys

### 1. Generate an API Key
```bash
curl -X POST http://localhost:8080/api/v1/auth/api-keys \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My API Key",
    "scope": ["read", "blockchain"],
    "rate_limit": 1000
  }'
```

**Response**:
```json
{
  "api_key": "bg_abc123_xyz789",
  "metadata": {
    "id": "uuid...",
    "name": "My API Key",
    "scope": ["read", "blockchain"],
    "rate_limit": 1000,
    "is_active": true
  },
  "warning": "Save this API key now. You won't be able to see it again!"
}
```

### 2. Use the API Key
```bash
curl -X GET http://localhost:8080/api/v1/chains \
  -H "X-API-Key: bg_abc123_xyz789"
```

Or in Authorization header:
```bash
curl -X GET http://localhost:8080/api/v1/chains \
  -H "Authorization: Bearer bg_abc123_xyz789"
```

---

## 🔍 Testing Rate Limiting

### Without Redis (In-Memory)
```bash
# Set REDIS_ENABLED=false in .env
for i in {1..130}; do
  curl -X GET http://localhost:8080/health \
    -H "X-API-Key: your_key" \
    -w "Request $i: %{http_code}\n" \
    -o /dev/null -s
done
```

### With Redis (Distributed)
```bash
# REDIS_ENABLED=true (default with docker-compose)
# Rate limits are shared across all gateway instances
```

---

## 📊 Health Check

```bash
curl http://localhost:8080/health
```

**Response**:
```json
{
  "status": "ok",
  "time": "2025-11-03T12:00:00Z"
}
```

---

## 🧪 Testing the Improvements

### Test Circuit Breaker
1. Use a non-existent RPC URL in config
2. Make requests - circuit will open after 5 failures
3. Wait 60s - circuit will attempt recovery

### Test Retry Logic
1. Configure with failing RPC endpoint
2. Monitor logs - should see exponential backoff retries
3. Verify: 3 retries with delays: 1s, 2s, 4s

### Test Rate Limiting
1. Make rapid requests (>120/min)
2. Should receive 429 status
3. Headers show: `X-RateLimit-Limit`, `X-RateLimit-Remaining`

---

## 📦 Package Structure

```
pkg/
├── auth/              # API key authentication
├── resilience/        # Resilience patterns
│   ├── rate_limiter.go
│   ├── circuit_breaker.go
│   ├── retry.go
│   └── http_client.go
├── middleware/        # HTTP middleware
│   ├── auth.go
│   └── rate_limit.go
├── rediscache/        # Redis client
└── api/              # API handlers
    └── auth_handler.go
```

---

## 🔧 Configuration

### Environment Variables
```bash
# Redis (for rate limiting)
REDIS_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# Rate Limiting
RATE_LIMIT=120  # requests per minute

# Blockchain RPCs
ETH_RPC_URL=https://ethereum.publicnode.com
# ... more RPC URLs
```

### Docker Compose
```yaml
redis:
  image: redis:7-alpine
  ports: ["6379:6379"]
```

---

## 📈 Performance Improvements

| Feature | Before | After |
|---------|--------|-------|
| Rate Limiting | In-memory | Redis-distributed |
| HTTP Connections | No pooling | 100 max idle, 10 per host |
| Failed RPCs | Immediate failure | 3 retries w/ backoff |
| Authentication | None | HMAC-SHA256 API keys |
| Circuit Breaker | None | Auto-protection |

---

## 🎯 Next Steps

**Phase 2 Priorities**:
1. ⏳ Health checks for all dependencies
2. ⏳ Multi-layer caching (L1/L2/L3)
3. ⏳ WebSocket support
4. ⏳ Transaction simulation
5. ⏳ GraphQL API

---

## 📚 Full Documentation

See `IMPROVEMENTS.md` for detailed documentation of all improvements.

---

## 🐛 Troubleshooting

### Redis Connection Failed
```
Warning: Redis connection failed: dial tcp...
Falling back to in-memory rate limiting
```
**Solution**: Check Redis is running on port 6379

### API Key Validation Failed
```
Invalid or expired API key
```
**Solution**: Generate a new API key or check expiration

### Rate Limit Exceeded
```
Status: 429 Too Many Requests
```
**Solution**: Wait 1 minute or upgrade your rate limit

---

## 💡 Tips

1. **Production Deployment**: Always use Redis for rate limiting
2. **API Keys**: Save them immediately - they can't be retrieved again
3. **Scopes**: Use `read` for queries, `write` for transactions, `admin` for management
4. **Rate Limits**: Set appropriate limits per API key
5. **Monitoring**: Check logs for circuit breaker and retry information

---

## ✅ Verification Checklist

- [ ] Code compiles: `go build ./cmd/server`
- [ ] Redis running: `redis-cli ping`
- [ ] Gateway starts: `curl http://localhost:8080/health`
- [ ] Generate API key: `POST /api/v1/auth/api-keys`
- [ ] Use API key: `X-API-Key` header
- [ ] Rate limiting works: Test rapid requests

---

**Happy Building!** 🚀
