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

---

## Deployment

For production deployment to **catalyst.springwire.ai**:

- **[DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md)** - Quick start guide (~30 minutes)
- **[deployment/README.md](deployment/README.md)** - Comprehensive deployment documentation
- **[deployment/DEPLOYMENT-CHECKLIST.md](deployment/DEPLOYMENT-CHECKLIST.md)** - Pre-deployment checklist

All deployment is via Docker Compose on catalyst.springwire.ai.

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
| `REDIS_URL` | string | `""` | Redis connection URL (alternative to discrete params) |
| `REDIS_HOST` | string | `"localhost"` | Redis hostname |
| `REDIS_PORT` | int | `6379` | Redis port |
| `REDIS_PASSWORD` | string | `""` | Redis password |
| `REDIS_DB` | int | `0` | Redis database number |
| `REDIS_MAX_IDLE` | int | `10` | Max idle connections |
| `REDIS_MAX_ACTIVE` | int | `50` | Max active connections |
| `REDIS_POOL_SIZE` | int | `50` | Connection pool size |
| `REDIS_IDLE_TIMEOUT` | duration | `300s` | Idle connection timeout |
| `REDIS_POOL_TIMEOUT` | duration | `4s` | Pool wait timeout |
| `REDIS_AUCTION_TTL` | int | `300` | Auction data TTL (seconds) |
| `REDIS_CACHE_TTL` | int | `3600` | General cache TTL (seconds) |

**Note**: Use either `REDIS_URL` (connection string) OR discrete parameters (HOST, PORT, etc), not both.

#### IDR Integration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `IDR_URL` | string | `""` | IDR service endpoint |
| `IDR_API_KEY` | string | `""` | API key for IDR service |
| `IDR_TIMEOUT_MS` | int | `150` | IDR request timeout (milliseconds) |
| `IDR_ENABLED` | bool | `true` | Enable IDR demand routing |
| `CURRENCY_CONVERSION_ENABLED` | bool | `true` | Enable multi-currency bid conversion |

#### IVT Detection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `IVT_MONITORING_ENABLED` | bool | `true` | Enable IVT detection and logging |
| `IVT_BLOCKING_ENABLED` | bool | `false` | Block high-score traffic |
| `IVT_CHECK_UA` | bool | `true` | Check user agent patterns |
| `IVT_CHECK_REFERER` | bool | `true` | Validate referer against domain |
| `IVT_CHECK_GEO` | bool | `false` | Geographic filtering (requires GeoIP database) |
| `IVT_GEOIP_DB_PATH` | string | `""` | Path to MaxMind GeoLite2 database file |
| `IVT_ALLOWED_COUNTRIES` | string | `""` | Comma-separated country codes (whitelist) |
| `IVT_BLOCKED_COUNTRIES` | string | `""` | Comma-separated country codes (blacklist) |
| `IVT_REQUIRE_REFERER` | bool | `false` | Strict mode - require referer header |

**Note**: `IVT_CHECK_GEO=true` requires MaxMind GeoLite2 database. See [GEOIP_SETUP.md](internal/middleware/GEOIP_SETUP.md) for setup instructions.

#### Database Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DB_HOST` | string | `"localhost"` | PostgreSQL hostname |
| `DB_PORT` | string | `"5432"` | PostgreSQL port |
| `DB_USER` | string | `"catalyst"` | PostgreSQL username |
| `DB_PASSWORD` | string | `""` | PostgreSQL password |
| `DB_NAME` | string | `"catalyst"` | PostgreSQL database name |
| `DB_SSL_MODE` | string | `"disable"` | PostgreSQL SSL mode (disable, require, verify-full) |
| `DB_MAX_OPEN_CONNS` | int | `25` | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | int | `5` | Maximum idle database connections |

#### Privacy Compliance

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PBS_ENFORCE_GDPR` | bool | `true` | Enforce GDPR consent |
| `PBS_ENFORCE_CCPA` | bool | `true` | Enforce CCPA consent |
| `PBS_ENFORCE_COPPA` | bool | `true` | Enforce COPPA compliance |
| `PBS_GEO_ENFORCEMENT` | bool | `true` | Auto-detect regulation from device.geo/user.geo |
| `PBS_ANONYMIZE_IP` | bool | `true` | Anonymize IP addresses when GDPR applies |
| `PBS_PRIVACY_STRICT_MODE` | bool | `true` | Reject invalid consent (false = strip PII) |
| `PBS_DISABLE_GDPR_ENFORCEMENT` | bool | `false` | Disable GDPR for testing only |

**Note**: Privacy middleware checks both `device.geo` and `user.geo` for regulation enforcement (audit fix Jan 2026). See [GEO-CONSENT-GUIDE.md](GEO-CONSENT-GUIDE.md) for details.

#### Publisher Authentication

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PUBLISHER_AUTH_ENABLED` | bool | `true` | Enable publisher validation |
| `PUBLISHER_AUTH_USE_REDIS` | bool | `true` | Use Redis for publisher lookup (faster) |
| `PUBLISHER_ALLOW_UNREGISTERED` | bool | `false` | Allow requests without publisher ID |
| `PUBLISHER_VALIDATE_DOMAIN` | bool | `false` | Validate domain matches registered |
| `REGISTERED_PUBLISHERS` | string | `""` | `pub1:domain1.com,pub2:domain2.com` format |

**Note**: When `PUBLISHER_AUTH_ENABLED=true`, `/openrtb2/auction` bypasses general API key auth. When disabled, auction requires API keys.

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

### ModSecurity Web Application Firewall (WAF)

Production-ready WAF protection with OWASP Core Rule Set and custom OpenRTB rules.

**What It Protects Against:**
- SQL injection, XSS, command injection attacks
- Protocol violations and malformed requests
- Known CVE vulnerabilities
- Malicious bot traffic and scanner probes
- HTTP protocol anomalies

**Custom OpenRTB Rules:**
- Validates OpenRTB 2.x request structure
- Enforces required fields (id, imp, site/app)
- Size limits on arrays (max 100 impressions)
- Blocked parameter injection attacks

**Deployment Options:**

1. **Reverse Proxy Mode** (Recommended for production)
```bash
cd deployment
./deploy-waf.sh

# WAF runs on :80/:443, proxies to Catalyst on :8000
# Automatic SSL with Let's Encrypt
```

2. **Sidecar Mode** (Docker Compose)
```bash
docker-compose -f deployment/docker-compose-modsecurity.yml up -d
```

3. **Standalone Testing**
```bash
docker run -p 80:80 \
  -e BACKEND_URL=http://catalyst:8000 \
  -e MODSEC_AUDIT_LOG=/var/log/modsec_audit.log \
  your-waf-image
```

**Configuration:**
- `PARANOIA_LEVEL`: 1 (default) to 4 (strict) - Higher = more rules enabled
- `ANOMALY_THRESHOLD`: Score threshold for blocking (default: 5)
- `MODSEC_AUDIT_LOG`: Full audit log path for investigation

**See full documentation**: [deployment/WAF-README.md](deployment/WAF-README.md)

---

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

### Publisher Management

Domain-based access control for auction requests with multiple configuration methods.

**Features:**
- Per-publisher domain whitelisting
- Wildcard subdomain support (*.example.com)
- Rate limiting per publisher (100 RPS default)
- Redis-based dynamic updates (no restart required)
- REST API for programmatic management
- CLI management script included

**Method 1: Management Script** (Easiest)
```bash
cd deployment

# List all publishers
./manage-publishers.sh list

# Add new publisher
./manage-publishers.sh add pub123 "example.com|*.example.com"

# Check specific publisher
./manage-publishers.sh check pub123

# Update domains
./manage-publishers.sh update pub123 "newdomain.com"

# Remove publisher
./manage-publishers.sh remove pub123
```

**Method 2: REST API** (For UX Integration)
```bash
# List all publishers
curl https://catalyst.springwire.ai/admin/publishers

# Get specific publisher
curl https://catalyst.springwire.ai/admin/publishers/pub123

# Create publisher
curl -X POST https://catalyst.springwire.ai/admin/publishers \
  -H "Content-Type: application/json" \
  -d '{"id":"pub123","allowed_domains":"example.com|*.example.com"}'

# Update publisher
curl -X PUT https://catalyst.springwire.ai/admin/publishers/pub123 \
  -H "Content-Type: application/json" \
  -d '{"allowed_domains":"newdomain.com"}'

# Delete publisher
curl -X DELETE https://catalyst.springwire.ai/admin/publishers/pub123
```

**Method 3: Environment Variables** (Static)
```bash
# In .env file (requires restart)
REGISTERED_PUBLISHERS=pub123:example.com|*.example.com,pub456:another.com
```

**Response Format:**
```json
{
  "publishers": [
    {
      "id": "pub123",
      "allowed_domains": "example.com|*.example.com",
      "domain_list": ["example.com", "*.example.com"]
    }
  ],
  "count": 1
}
```

**Building a UX:**

The REST API is designed for integration with admin UIs. Example JavaScript:

```javascript
// Fetch all publishers
async function fetchPublishers() {
  const response = await fetch('/admin/publishers');
  const data = await response.json();
  return data.publishers;
}

// Add new publisher
async function addPublisher(id, domains) {
  const response = await fetch('/admin/publishers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: id,
      allowed_domains: domains
    })
  });
  return response.json();
}
```

See **[PUBLISHER-CONFIG-GUIDE.md](PUBLISHER-CONFIG-GUIDE.md)** for complete documentation.

### Bidder-Specific Parameters

Each bidder adapter requires specific parameters in the OpenRTB request.

**Rubicon/Magnite requires:**
```json
{
  "imp": [{
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }]
}
```

**Testing:**
```bash
# Test Rubicon parameters
cd examples
./test-rubicon-params.sh

# Test with custom values
./test-rubicon-params.sh <account_id> <site_id> <zone_id>

# Test with curl
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d @rubicon-bid-request.json
```

**Example Files:**
- `examples/rubicon-bid-request.json` - Single bidder example with your Rubicon credentials
- `examples/multi-bidder-request.json` - Multiple bidders (Rubicon, AppNexus, PubMatic)
- `examples/test-rubicon-params.sh` - Automated test script

See **[BIDDER-PARAMS-GUIDE.md](BIDDER-PARAMS-GUIDE.md)** for complete documentation on all bidders.

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

Comprehensive privacy enforcement with **automatic geographic detection** and support for 7+ global privacy regulations.

#### Supported Regulations

- **GDPR** (EU/EEA/UK) - TCF 2.0 consent framework
- **CCPA** (California, USA) - Do Not Sell enforcement
- **COPPA** (USA) - Children's privacy protection
- **VCDPA** (Virginia), **CPA** (Colorado), **CTDPA** (Connecticut), **UCPA** (Utah)
- **LGPD** (Brazil), **PIPEDA** (Canada), **PDPA** (Singapore)

#### How It Works

**1. Geographic Auto-Detection** (Audit Fix - Jan 2026)

The system automatically detects applicable privacy regulations from geographic data:

```javascript
// Checks BOTH locations for enforcement (prevents bypass)
if (request.device?.geo?.country) {
  // Check device location (current)
}
if (request.user?.geo?.country) {
  // Check user location (home address)
}
```

**Security Note**: Prior to Jan 2026, only `device.geo` was checked. Users could bypass GDPR by omitting device.geo and sending only user.geo. This vulnerability has been fixed.

**2. IP Anonymization**

When GDPR applies and `PBS_ANONYMIZE_IP=true`:
- IPv4: Last octet zeroed (e.g., `192.168.1.100` → `192.168.1.0`)
- IPv6: Last 80 bits zeroed
- Preserves OpenRTB extensions during anonymization

**3. TCF Vendor Consent**

Per-bidder consent validation using IAB GVL IDs:
- Checks vendor-specific consent in TCF string
- Filters bidders without user consent
- Supports purpose and special feature consent

**4. Strict vs Permissive Mode**

- `PBS_PRIVACY_STRICT_MODE=true`: Reject invalid/missing consent (return 400)
- `PBS_PRIVACY_STRICT_MODE=false`: Strip PII and continue auction

#### Configuration Examples

**GDPR (European Union)**
```bash
PBS_ENFORCE_GDPR=true
PBS_GEO_ENFORCEMENT=true      # Auto-detect from geo
PBS_ANONYMIZE_IP=true         # Required for GDPR
PBS_PRIVACY_STRICT_MODE=true  # Reject invalid consent
```

**CCPA (California)**
```bash
PBS_ENFORCE_CCPA=true
PBS_GEO_ENFORCEMENT=true
# Respects "Do Not Sell" (usprivacy string)
```

**Development/Testing**
```bash
PBS_DISABLE_GDPR_ENFORCEMENT=true  # ⚠️ Testing only!
```

#### Compliance Documentation

- **[GEO-CONSENT-GUIDE.md](GEO-CONSENT-GUIDE.md)** - Geographic enforcement rules
- **[TCF-VENDOR-CONSENT-GUIDE.md](TCF-VENDOR-CONSENT-GUIDE.md)** - TCF 2.0 implementation

#### Recent Improvements (Audit Fixes - Jan 2026)

1. ✅ **Dual Geo-Check**: Validates both `device.geo` AND `user.geo` (prevents bypass)
2. ✅ **Extension Preservation**: IP anonymization preserves all OpenRTB extensions
3. ✅ **Proper Error Codes**: Returns 400 (not 500) for invalid consent

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

> **Note**: As of January 2026, the system uses **static bidders only**. Dynamic bidder loading from PostgreSQL was removed for performance and security.

**Current Static Bidders**: `rubicon`, `pubmatic`, `appnexus`, `demo`

To add a new bidder, create a static adapter and register it in the exchange:

1. **Create adapter file** in `internal/adapters/<bidder>/`:
```go
package mybidder

import "github.com/thenexusengine/tne_springwire/internal/openrtb"

type Adapter struct {
    endpoint string
}

func NewAdapter(endpoint string) *Adapter {
    return &Adapter{endpoint: endpoint}
}

func (a *Adapter) MakeBids(req *openrtb.BidRequest,
    params map[string]interface{}) (*openrtb.BidResponse, error) {
    // Implement bidding logic
}
```

2. **Register in exchange** (`internal/exchange/exchange.go`):
```go
func (e *Exchange) initializeStaticBidders() {
    e.bidders["mybidder"] = mybidder.NewAdapter("https://mybidder.com/rtb")
}
```

3. **Publishers configure params** in their bidder_params JSONB field

For detailed migration guide, see [BIDDER-MANAGEMENT.md](deployment/BIDDER-MANAGEMENT.md)

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
- **IVT Detection Guide**: [docs/ivt-detection.md](docs/ivt-detection.md)
- **OpenRTB Spec**: https://www.iab.com/guidelines/real-time-bidding-rtb-project/

### Community
- **GitHub Issues**: https://github.com/thenexusengine/tne_springwire/issues

### Related Projects
- **TNE Engine** - Publisher-facing SDK
- **TNE IDR** - Intelligent demand router with ML optimization

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

**Built for transparency and scale by The Nexus Engine**
