# TNE Catalyst - Auction Server

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://go.dev/)
[![OpenRTB](https://img.shields.io/badge/OpenRTB-2.x-green.svg)](https://www.iab.com/guidelines/real-time-bidding-rtb-project/)

Server-side header bidding auction engine with intelligent demand routing, invalid traffic detection, and privacy compliance. Built for scale and transparency.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Deployment](#deployment)
- [Configuration](#configuration)
- [Features](#features)
- [Monitoring](#monitoring)
- [Performance Tuning](#performance-tuning)
- [Operations](#operations)

---

## Overview

Catalyst is the server-side auction engine that powers The Nexus Engine's transparent ad exchange. It processes OpenRTB 2.x bid requests, orchestrates parallel bidder auctions, and integrates with the Intelligent Demand Router (IDR) for ML-optimized demand selection.

### Architecture

```
┌─────────────────┐
│  TNE Engine SDK │ (Publisher Website)
└────────┬────────┘
         │ bid request
         ▼
┌─────────────────┐
│   CATALYST      │ (This Server)
│  ┌───────────┐  │
│  │ Publisher │  │ ← Publisher authentication
│  │   Auth    │  │ ← IVT detection
│  └─────┬─────┘  │
│        │        │
│  ┌─────▼─────┐  │
│  │  Auction  │  │ ← OpenRTB 2.x protocol
│  │   Core    │  │ ← Parallel bidding
│  └─────┬─────┘  │
│        │        │
│  ┌─────▼─────┐  │
│  │    IDR    │◄─┼─ ML demand router
│  │ Selector  │  │
│  └─────┬─────┘  │
│        │        │
│  ┌─────▼─────┐  │
│  │  Bidder   │  │ ← Adapter pattern
│  │ Adapters  │  │ ← SSP/DSP connectors
│  └─────┬─────┘  │
└────────┼────────┘
         │
         ▼
   ┌───────────┐
   │  Bidders  │ (External SSPs/DSPs)
   └───────────┘
```

### Key Features

- **OpenRTB 2.x Compliant** - Industry-standard protocol
- **Intelligent Demand Routing** - ML-based demand source selection
- **Invalid Traffic Detection** - Real-time fraud protection
- **Privacy Compliance** - GDPR, CCPA, and COPPA enforcement
- **Publisher Authentication** - Domain validation and access control
- **Parallel Bidding** - Concurrent adapter execution
- **Server-Side Adapters** - Easy integration with new demand sources

---

## Quick Start

### Prerequisites

- Go 1.21 or higher
- Redis 7.x (for caching and IVT detection)
- PostgreSQL 14+ with TimescaleDB (for analytics and IDR)

### Local Development

```bash
# Clone the repository
git clone https://github.com/thenexusengine/tne_springwire.git
cd tne-catalyst

# Install dependencies
go mod download

# Run tests
go test ./...

# Start the server
PBS_PORT=8000 go run cmd/server/main.go
```

The server will start on `http://localhost:8000`.

### Using Docker Compose

For a complete local environment with all dependencies:

```bash
# Clone the dev environment
git clone https://github.com/yourusername/tne-dev-env.git
cd tne-dev-env

# Start all services (Catalyst, IDR, Redis, PostgreSQL)
docker-compose up -d

# View Catalyst logs
docker-compose logs -f catalyst

# Stop services
docker-compose down
```

Catalyst will be available at `http://localhost:8000`.

---

## Deployment

### Fly.io (Recommended for Production)

```bash
# Install Fly CLI
curl -L https://fly.io/install.sh | sh

# Login to Fly
fly auth login

# Deploy
fly deploy

# Set secrets
fly secrets set \
  REDIS_URL=redis://your-redis-host:6379/0 \
  DATABASE_URL=postgresql://user:pass@host:5432/catalyst \
  IDR_URL=https://your-idr-instance.com \
  PBS_HOST_URL=https://catalyst.springwire.ai

# Scale as needed
fly scale count 3  # 3 instances
fly scale vm shared-cpu-2x  # Upgrade CPU/RAM
```

### AWS Lightsail

```bash
# Create instance
aws lightsail create-container-service \
  --service-name tne-catalyst \
  --power small \
  --scale 2

# Build and push image
docker build -t tne-catalyst:latest .
aws lightsail push-container-image \
  --service-name tne-catalyst \
  --label catalyst \
  --image tne-catalyst:latest

# Deploy
aws lightsail create-container-service-deployment \
  --service-name tne-catalyst \
  --containers file://containers.json \
  --public-endpoint file://public-endpoint.json
```

### Docker

```bash
# Build image
docker build -t tne-catalyst:latest .

# Run container
docker run -d \
  --name catalyst \
  -p 8000:8000 \
  -e PBS_PORT=8000 \
  -e PBS_HOST_URL=https://catalyst.springwire.ai \
  -e REDIS_URL=redis://redis:6379/0 \
  -e IDR_URL=http://idr:5050 \
  -e IVT_MONITORING_ENABLED=true \
  -e IVT_BLOCKING_ENABLED=true \
  tne-catalyst:latest
```

---

## Configuration

### Environment Variables

#### Server Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PBS_PORT` | string | `"8000"` | Server port |
| `PBS_HOST_URL` | string | `""` | Public hostname for cookie sync (e.g., https://catalyst.springwire.ai) |
| `HOST` | string | `"0.0.0.0"` | Bind address |
| `LOG_LEVEL` | string | `"info"` | Logging level (debug, info, warn, error) |
| `CORS_ALLOWED_ORIGINS` | string | `""` | Comma-separated list of allowed CORS origins |

#### Redis Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `REDIS_URL` | string | `""` | Redis connection URL |
| `REDIS_MAX_IDLE` | int | `10` | Max idle connections |
| `REDIS_MAX_ACTIVE` | int | `50` | Max active connections |

#### IDR Integration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `IDR_URL` | string | `""` | IDR service endpoint |
| `IDR_TIMEOUT_MS` | int | `50` | IDR request timeout (ms) |
| `IDR_ENABLED` | bool | `true` | Enable IDR demand routing |

#### IVT Detection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `IVT_MONITORING_ENABLED` | bool | `true` | Enable IVT detection and logging |
| `IVT_BLOCKING_ENABLED` | bool | `false` | Block high-score traffic |
| `IVT_CHECK_UA` | bool | `true` | Check user agent patterns |
| `IVT_CHECK_REFERER` | bool | `true` | Validate referer against domain |
| `IVT_CHECK_GEO` | bool | `false` | Geographic filtering (requires GeoIP) |
| `IVT_ALLOWED_COUNTRIES` | string | `""` | Comma-separated country codes (whitelist) |
| `IVT_BLOCKED_COUNTRIES` | string | `""` | Comma-separated country codes (blacklist) |
| `IVT_REQUIRE_REFERER` | bool | `false` | Strict mode - require referer header |

#### Privacy Compliance

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PBS_ENFORCE_GDPR` | bool | `true` | Enforce GDPR consent |
| `PBS_ENFORCE_CCPA` | bool | `true` | Enforce CCPA consent |
| `PBS_ENFORCE_COPPA` | bool | `true` | Enforce COPPA compliance |

#### Publisher Authentication

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PUBLISHER_AUTH_ENABLED` | bool | `true` | Enable publisher validation |
| `PUBLISHER_ALLOW_UNREGISTERED` | bool | `false` | Allow requests without publisher ID |
| `PUBLISHER_VALIDATE_DOMAIN` | bool | `false` | Validate domain matches registered |
| `REGISTERED_PUBLISHERS` | string | `""` | `pub1:domain1.com,pub2:domain2.com` format |

### Example Configurations

#### Development

```bash
# .env.development
PBS_PORT=8000
HOST=0.0.0.0
LOG_LEVEL=debug

REDIS_URL=redis://localhost:6379/0
IDR_URL=http://localhost:5050
IDR_ENABLED=true

IVT_MONITORING_ENABLED=true
IVT_BLOCKING_ENABLED=false  # Monitor only, don't block
IVT_CHECK_UA=true
IVT_CHECK_REFERER=false     # Many dev requests have no referer

PUBLISHER_AUTH_ENABLED=true
PUBLISHER_ALLOW_UNREGISTERED=true  # Allow testing without registration
```

#### Production

```bash
# .env.production
PBS_PORT=8000
PBS_HOST_URL=https://catalyst.springwire.ai
HOST=0.0.0.0
LOG_LEVEL=info

# CORS - Allow publisher domains
CORS_ALLOWED_ORIGINS=https://example.com,https://*.example.com

REDIS_URL=redis://prod-redis:6379/0
IDR_URL=https://idr.thenexusengine.com
IDR_ENABLED=true
IDR_TIMEOUT_MS=50

# IVT Protection - Full blocking mode
IVT_MONITORING_ENABLED=true
IVT_BLOCKING_ENABLED=true   # Block suspicious traffic
IVT_CHECK_UA=true
IVT_CHECK_REFERER=true
IVT_ALLOWED_COUNTRIES=US,GB,CA,AU,NZ

# Privacy - All enabled
PBS_ENFORCE_GDPR=true
PBS_ENFORCE_CCPA=true
PBS_ENFORCE_COPPA=true

# Publisher Auth - Strict mode
PUBLISHER_AUTH_ENABLED=true
PUBLISHER_ALLOW_UNREGISTERED=false
PUBLISHER_VALIDATE_DOMAIN=true
REGISTERED_PUBLISHERS=pub-123:example.com,pub-456:*.mysite.com
```

---

## Features

### Invalid Traffic (IVT) Detection

Catalyst includes built-in fraud detection with configurable blocking. See full documentation: [docs/ivt-detection.md](docs/ivt-detection.md)

**Key Features:**
- User agent analysis (bots, scrapers, headless browsers)
- Referer validation against registered domains
- Geographic filtering (optional)
- Scoring system (0-100, threshold 70)
- Two modes: Monitoring (log only) or Blocking (reject)

**Quick Setup:**

```bash
# Phase 1: Monitor Only (recommended for 1-2 weeks)
IVT_MONITORING_ENABLED=true
IVT_BLOCKING_ENABLED=false

# Phase 2: Enable Blocking
IVT_MONITORING_ENABLED=true
IVT_BLOCKING_ENABLED=true
```

**Check Metrics:**
```bash
# View IVT logs
grep "IVT detected" /var/log/catalyst.log

# Headers added to all requests
X-IVT-Score: 50
X-IVT-Signals: 1
```

### Publisher Authentication

Domain-based access control for auction requests.

**Features:**
- Per-publisher domain whitelisting
- Wildcard subdomain support (*.example.com)
- Rate limiting per publisher (100 RPS default)
- Redis-based publisher registry

**Register Publishers Programmatically:**
```go
import "github.com/thenexusengine/tne_springwire/internal/middleware"

pubAuth := middleware.NewPublisherAuth(nil)

// Register publisher with allowed domains
pubAuth.RegisterPublisher("pub-123", "example.com|*.example.com")

// Or use Redis
redis.HSet(ctx, "tne_catalyst:publishers", "pub-123", "example.com")
```

### Intelligent Demand Router (IDR) Integration

ML-based demand source selection for optimized yield.

**How It Works:**
1. Auction request arrives at Catalyst
2. Catalyst queries IDR with publisher/domain context
3. IDR returns scored list of demand sources
4. Catalyst runs auction with selected adapters

**Configuration:**
```bash
IDR_URL=https://idr.thenexusengine.com
IDR_TIMEOUT_MS=50  # Fast timeout to avoid blocking
IDR_ENABLED=true
```

### Privacy Compliance

Built-in enforcement for GDPR, CCPA, and COPPA.

**GDPR:**
- Requires consent string in bid request
- Validates TCF 2.0 consent strings
- Blocks auctions without valid consent

**CCPA:**
- Respects "Do Not Sell" signals
- Validates US Privacy string
- Blocks California traffic without consent

**COPPA:**
- Enforces age-restricted content rules
- Validates coppa flag in bid request
- Blocks auctions for children's sites

---

## Monitoring

### Health Checks

```bash
# Basic health check
curl http://localhost:8000/health
# Returns: 200 OK

# Detailed status (coming soon)
curl http://localhost:8000/status
```

### Logging

Catalyst uses structured JSON logging:

```json
{
  "level": "info",
  "time": "2025-01-06T15:30:00Z",
  "publisher_id": "pub-123",
  "domain": "example.com",
  "auction_id": "abc123",
  "winner_cpm": 2.50,
  "bidder_count": 5,
  "message": "Auction complete"
}
```

**Log Levels:**
- `debug` - Verbose logging for development
- `info` - Normal operational logging
- `warn` - Warnings (IVT detections, rate limits)
- `error` - Errors requiring attention

### Metrics (Prometheus Format)

Expose metrics at `/metrics` endpoint:

```bash
# Auction metrics
catalyst_auctions_total{publisher="pub-123"} 1000
catalyst_auction_duration_ms{publisher="pub-123"} 45.2
catalyst_auction_errors_total{publisher="pub-123"} 5

# IVT metrics
catalyst_ivt_checked_total 1000
catalyst_ivt_flagged_total 50
catalyst_ivt_blocked_total 10

# Bidder metrics
catalyst_bidder_requests_total{bidder="appnexus"} 500
catalyst_bidder_responses_total{bidder="appnexus"} 490
catalyst_bidder_timeouts_total{bidder="appnexus"} 10
```

### Alerting

**Recommended Alerts:**

1. **High Error Rate**
   ```
   rate(catalyst_auction_errors_total[5m]) > 0.05
   ```

2. **IVT Spike**
   ```
   rate(catalyst_ivt_flagged_total[5m]) > 0.20
   ```

3. **Bidder Timeout Rate**
   ```
   rate(catalyst_bidder_timeouts_total[5m]) / rate(catalyst_bidder_requests_total[5m]) > 0.10
   ```

4. **High Latency**
   ```
   histogram_quantile(0.95, catalyst_auction_duration_ms) > 200
   ```

---

## Performance Tuning

### Benchmarks

```bash
# Run benchmarks
go test -bench=. ./internal/auction/...

# Expected results (on 2 CPU, 4GB RAM):
BenchmarkAuction-2           5000    250000 ns/op    # ~4000 QPS
BenchmarkIVTDetection-2     10000    100000 ns/op    # ~10000 QPS
```

### Optimization Tips

**1. Redis Connection Pool**
```bash
REDIS_MAX_IDLE=20     # Increase for high traffic
REDIS_MAX_ACTIVE=100  # Increase for burst capacity
```

**2. IDR Timeout**
```bash
IDR_TIMEOUT_MS=30     # Reduce for faster auctions (at cost of IDR accuracy)
```

**3. Goroutine Pools**
```go
// In code: Adjust concurrent bidder limit
config.MaxConcurrentBidders = 10  // Default: 5
```

**4. Disable Optional Features**
```bash
IVT_CHECK_GEO=false        # GeoIP lookup adds ~5ms
IVT_CHECK_REFERER=false    # Referer validation adds ~1ms
```

### Scaling Recommendations

| Traffic (QPS) | Instances | CPU | RAM | Redis | Notes |
|---------------|-----------|-----|-----|-------|-------|
| < 100 | 1 | 1 CPU | 1 GB | Shared | Development |
| 100-500 | 2 | 1 CPU | 2 GB | Shared | Small publisher |
| 500-2000 | 3-5 | 2 CPU | 4 GB | Dedicated | Medium publisher |
| 2000-10000 | 10-20 | 2 CPU | 4 GB | Cluster | Large publisher |
| > 10000 | 50+ | 4 CPU | 8 GB | Cluster | Enterprise |

---

## Operations

### Deployment Checklist

- [ ] Environment variables configured
- [ ] Redis connection verified
- [ ] PostgreSQL/TimescaleDB connection verified
- [ ] IDR endpoint accessible
- [ ] Publisher authentication configured
- [ ] IVT detection tuned (monitoring mode first!)
- [ ] Privacy compliance settings verified
- [ ] Health checks responding
- [ ] Metrics endpoint accessible
- [ ] Logging to centralized system
- [ ] Alerts configured
- [ ] Load balancer configured (if applicable)
- [ ] SSL/TLS certificates installed
- [ ] Backup strategy in place

### Common Operations

**View Active Publishers:**
```bash
redis-cli HGETALL tne_catalyst:publishers
```

**Add Publisher:**
```bash
redis-cli HSET tne_catalyst:publishers pub-123 "example.com|*.example.com"
```

**Remove Publisher:**
```bash
redis-cli HDEL tne_catalyst:publishers pub-123
```

**Check IVT Stats:**
```bash
grep "IVT detected" /var/log/catalyst.log | wc -l
grep "Request blocked" /var/log/catalyst.log | wc -l
```

**Restart Without Downtime (Fly.io):**
```bash
fly deploy --strategy rolling
```

### Troubleshooting

**Problem: High latency**
- Check IDR response time (should be < 50ms)
- Check Redis latency
- Review bidder adapter timeouts
- Check GeoIP database if enabled

**Problem: Legitimate traffic blocked**
- Review IVT logs for false positives
- Temporarily disable IVT blocking: `IVT_BLOCKING_ENABLED=false`
- Check referer validation settings
- Review publisher domain configuration

**Problem: No auctions processing**
- Verify publisher authentication settings
- Check `PUBLISHER_ALLOW_UNREGISTERED` flag
- Review publisher registration in Redis
- Check request logs for validation errors

**Problem: Memory leak**
- Profile with pprof: `go tool pprof http://localhost:8000/debug/pprof/heap`
- Check goroutine count: `curl http://localhost:8000/debug/pprof/goroutine?debug=1`
- Review Redis connection pool settings

---

## Development

### Project Structure

```
tne-catalyst/
├── cmd/
│   └── server/          # Main server entry point
├── internal/
│   ├── api/             # HTTP handlers
│   ├── auction/         # Auction core logic
│   ├── middleware/      # Auth, IVT, logging
│   ├── adapters/        # Bidder adapters
│   ├── idr/             # IDR client
│   └── storage/         # Redis, PostgreSQL
├── scripts/             # Test scripts
├── docs/                # Documentation
├── go.mod
└── Dockerfile
```

### Adding a New Bidder Adapter

1. Create adapter file in `internal/adapters/`:
```go
package adapters

type MyBidderAdapter struct {
    endpoint string
    timeout  time.Duration
}

func (a *MyBidderAdapter) MakeBids(ctx context.Context, req *openrtb2.BidRequest) (*BidResponse, error) {
    // Implement bidder-specific logic
}
```

2. Register adapter in `internal/adapters/registry.go`:
```go
func init() {
    Register("mybidder", &MyBidderAdapter{
        endpoint: "https://mybidder.com/rtb",
        timeout:  100 * time.Millisecond,
    })
}
```

3. Test the adapter:
```go
func TestMyBidderAdapter(t *testing.T) {
    // Write unit tests
}
```

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/middleware/...

# Integration tests
go test -tags=integration ./tests/...

# IVT detection test suite
go run scripts/test_ivt.go
```

---

## Support

### Documentation
- **Full Docs**: https://docs.thenexusengine.com
- **API Reference**: https://docs.thenexusengine.com/api/catalyst
- **IVT Detection Guide**: [docs/ivt-detection.md](docs/ivt-detection.md)
- **OpenRTB Spec**: https://www.iab.com/guidelines/real-time-bidding-rtb-project/

### Community
- **GitHub Issues**: https://github.com/thenexusengine/tne_springwire/issues
- **Discord**: https://discord.gg/thenexusengine
- **Email**: support@thenexusengine.com

### Related Projects
- **TNE Engine** - Publisher-facing SDK
- **TNE IDR** - Intelligent demand router with ML optimization

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

**Built for transparency and scale by The Nexus Engine**
