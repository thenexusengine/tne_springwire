# Bidder Management - PostgreSQL Guide

This guide explains how to manage bidders and their configurations in Catalyst using PostgreSQL.

## Overview

Bidder configuration is stored in PostgreSQL for:
- **Bidder identification** (bidder_code, bidder_name)
- **Endpoint URLs** for OpenRTB auction requests
- **Capabilities** (banner, video, native, audio support)
- **GDPR compliance** (IAB GVL vendor IDs)
- **Timeout configuration** and HTTP headers

Publisher-specific bidder parameters (accountId, siteId, etc.) remain in the `publishers` table as JSONB.

## Architecture

```
PostgreSQL (Source of Truth)
├── bidders table (global bidder configs)
│   ├── bidder_code (e.g., "rubicon", "pubmatic")
│   ├── bidder_name (e.g., "Rubicon/Magnite")
│   ├── endpoint_url (OpenRTB endpoint)
│   ├── timeout_ms, enabled, status
│   └── capabilities (banner, video, native, audio)
│
└── publishers table (publisher-specific params)
    └── bidder_params (JSONB)
        ├── rubicon: {accountId, siteId, zoneId}
        ├── pubmatic: {publisherId, adSlot}
        └── appnexus: {placementId}

On auction request:
1. Load active bidders from bidders table
2. Get publisher's bidder_params from publishers table
3. Merge params and endpoint URLs
4. Make parallel OpenRTB requests to bidders
```

## Benefits of Database-Backed Bidders

**Before** (hardcoded):
- Endpoint URLs hardcoded in adapter files
- Code changes required to update endpoints
- Deployment needed for bidder configuration changes

**After** (database-backed):
- Change endpoint URLs without code changes
- Enable/disable bidders dynamically
- Regional routing potential
- Easy A/B testing of bidder endpoints
- Audit trail of configuration changes

## Database Schema

The `bidders` table structure:

```sql
CREATE TABLE bidders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bidder_code VARCHAR(50) UNIQUE NOT NULL,
    bidder_name VARCHAR(255) NOT NULL,
    endpoint_url TEXT NOT NULL,
    timeout_ms INTEGER DEFAULT 1000,
    enabled BOOLEAN DEFAULT true,
    status VARCHAR(50) DEFAULT 'active',

    -- Capabilities
    supports_banner BOOLEAN DEFAULT true,
    supports_video BOOLEAN DEFAULT false,
    supports_native BOOLEAN DEFAULT false,
    supports_audio BOOLEAN DEFAULT false,

    -- Privacy & Compliance
    gvl_vendor_id INTEGER,

    -- HTTP Configuration
    http_headers JSONB DEFAULT '{}'::jsonb,

    -- Metadata
    description TEXT,
    documentation_url TEXT,
    contact_email VARCHAR(255),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### Pre-Seeded Bidders

The migration includes these common bidders:

| Bidder Code | Name | GVL ID | Supports |
|------------|------|--------|----------|
| rubicon | Rubicon/Magnite | 52 | Banner, Video, Native |
| pubmatic | PubMatic | 76 | Banner, Video |
| appnexus | AppNexus/Xandr | 32 | Banner, Video, Native |
| ix | Index Exchange | 10 | Banner, Video |
| openx | OpenX | 69 | Banner, Video |
| criteo | Criteo | 91 | Banner |
| triplelift | TripleLift | 28 | Banner, Native |
| sovrn | Sovrn | 13 | Banner |
| demo | Demo Bidder | - | Banner, Video, Native (testing) |

## Management Script

Use `/Users/andrewstreets/tne-catalyst/deployment/manage-bidders.sh` to manage bidders.

### List Bidders

```bash
./manage-bidders.sh list
```

Output:
```
═══════════════════════════════════════════════════
Registered Bidders in Catalyst
═══════════════════════════════════════════════════

BIDDER CODE     NAME                      ENDPOINT                                           ENABLED    STATUS
────────────────────────────────────────────────────────────────────────────────────────────────────
rubicon         Rubicon/Magnite           https://prebid-server.rubiconproject.com/openr...  t          active
pubmatic        PubMatic                  https://hbopenbid.pubmatic.com/translator?sour...  t          active
appnexus        AppNexus/Xandr            https://ib.adnxs.com/openrtb2                      t          active

Total: 9 bidder(s)
```

### Check Bidder Details

```bash
./manage-bidders.sh check rubicon
```

Output:
```
═══════════════════════════════════════════════════
Bidder: rubicon
═══════════════════════════════════════════════════
  Name: Rubicon/Magnite
  Status: active
  Enabled: true
  Endpoint: https://prebid-server.rubiconproject.com/openrtb2/auction
  Timeout: 1500ms

Supported Formats:
  • Banner
  • Video
  • Native

  GVL Vendor ID: 52 (GDPR consent)
  Description: Rubicon Project (now Magnite) - requires accountId, siteId, zoneId
  Created: 2026-01-13 22:00:00
```

### Add Custom Bidder

**Basic bidder (banner only):**
```bash
./manage-bidders.sh add custom 'Custom SSP' 'https://custom.ssp.com/openrtb2'
```

**With timeout and GVL ID:**
```bash
./manage-bidders.sh add custom 'Custom SSP' 'https://custom.ssp.com/openrtb2' 2000 999
```

**Regional bidder:**
```bash
./manage-bidders.sh add rubicon-eu 'Rubicon EU' 'https://eu.prebid-server.rubiconproject.com/openrtb2/auction' 1500 52
```

### Update Bidder

**Change endpoint URL:**
```bash
./manage-bidders.sh update rubicon endpoint_url 'https://new-endpoint.rubiconproject.com/openrtb2/auction'
```

**Change timeout:**
```bash
./manage-bidders.sh update rubicon timeout_ms 2000
```

**Enable video support:**
```bash
./manage-bidders.sh update rubicon supports_video true
```

**Change status:**
```bash
./manage-bidders.sh update rubicon status 'testing'
```

### Enable/Disable Bidders

**Disable a bidder:**
```bash
./manage-bidders.sh disable rubicon
```

**Re-enable a bidder:**
```bash
./manage-bidders.sh enable rubicon
```

### Remove Bidder

```bash
./manage-bidders.sh remove custom
```

Note: This performs a soft delete (sets `status='archived'` and `enabled=false`).

## How It Works

### 1. Server Startup

On startup, Catalyst loads bidders from PostgreSQL:

```
2026-01-13 22:00:00 INFO Bidders loaded from PostgreSQL count=9
```

### 2. Auction Request

When an auction request arrives:

1. **Publisher Validation** (via publisher_auth middleware)
   - Checks publishers table for publisher_id
   - Validates allowed_domains
   - Loads publisher's bidder_params

2. **Bidder Selection**
   - Queries bidders table for active bidders
   - Filters by publisher's configured bidders
   - Checks format capabilities (banner/video/native)

3. **Request Assembly**
   - Gets endpoint_url from bidders table
   - Merges bidder_params from publishers table
   - Creates OpenRTB 2.x bid request

4. **Parallel Bidding**
   - Makes HTTP POST to each bidder's endpoint_url
   - Applies per-bidder timeout_ms
   - Includes custom http_headers if configured

### 3. Example Flow

**Publisher config in database:**
```json
{
  "publisher_id": "totalsportspro",
  "allowed_domains": "totalsportspro.com",
  "bidder_params": {
    "rubicon": {
      "accountId": 26298,
      "siteId": 556630,
      "zoneId": 3767186
    }
  }
}
```

**Bidder config in database:**
```json
{
  "bidder_code": "rubicon",
  "endpoint_url": "https://prebid-server.rubiconproject.com/openrtb2/auction",
  "timeout_ms": 1500,
  "enabled": true
}
```

**Resulting bid request to Rubicon:**
```
POST https://prebid-server.rubiconproject.com/openrtb2/auction
Timeout: 1500ms

{
  "id": "auction-123",
  "imp": [{
    "id": "1",
    "banner": {"w": 728, "h": 90},
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }],
  "site": {
    "domain": "totalsportspro.com",
    "publisher": {"id": "totalsportspro"}
  }
}
```

## Common Bidder Parameters

### Rubicon/Magnite
```json
{
  "rubicon": {
    "accountId": 26298,
    "siteId": 556630,
    "zoneId": 3767186
  }
}
```

### PubMatic
```json
{
  "pubmatic": {
    "publisherId": "12345",
    "adSlot": "slot-name"
  }
}
```

### AppNexus/Xandr
```json
{
  "appnexus": {
    "placementId": 54321
  }
}
```

### Index Exchange (IX)
```json
{
  "ix": {
    "siteId": "12345"
  }
}
```

### OpenX
```json
{
  "openx": {
    "unit": "12345",
    "delDomain": "publisher-d.openx.net"
  }
}
```

## Environment Configuration

In `deployment/.env`:

```bash
# PostgreSQL Configuration
DB_HOST=postgres
DB_PORT=5432
DB_NAME=catalyst
DB_USER=catalyst
DB_PASSWORD=your_secure_password
DB_SSL_MODE=disable

# Bidder defaults (optional)
DEFAULT_BIDDER_TIMEOUT=1000
MAX_BIDDERS_PER_REQUEST=50
```

## Direct Database Access

Access PostgreSQL directly:

```bash
docker exec -it catalyst-postgres psql -U catalyst -d catalyst
```

### Useful Queries

**List all active bidders:**
```sql
SELECT bidder_code, bidder_name, endpoint_url, enabled, status
FROM bidders
WHERE enabled = true AND status = 'active';
```

**Get bidders with capabilities:**
```sql
SELECT bidder_code, bidder_name,
       supports_banner, supports_video, supports_native
FROM bidders
WHERE enabled = true
ORDER BY bidder_code;
```

**Find bidders supporting video:**
```sql
SELECT bidder_code, bidder_name, endpoint_url
FROM bidders
WHERE supports_video = true
  AND enabled = true
  AND status = 'active';
```

**Update bidder endpoint:**
```sql
UPDATE bidders
SET endpoint_url = 'https://new-endpoint.com/openrtb2'
WHERE bidder_code = 'rubicon';
```

**Check bidder-publisher pairings (using view):**
```sql
SELECT * FROM v_publisher_bidders
WHERE publisher_id = 'totalsportspro';
```

**Get all publishers using a specific bidder:**
```sql
SELECT publisher_id, name, bidder_params->'rubicon' as rubicon_config
FROM publishers
WHERE bidder_params ? 'rubicon'
  AND status = 'active';
```

## Testing

Test auction with custom bidder endpoint:

```bash
curl -X POST http://localhost:8000/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-001",
    "imp": [{
      "id": "1",
      "banner": {"format": [{" w": 728, "h": 90}]},
      "ext": {
        "rubicon": {
          "accountId": 26298,
          "siteId": 556630,
          "zoneId": 3767186
        }
      }
    }],
    "site": {
      "domain": "totalsportspro.com",
      "page": "https://totalsportspro.com/",
      "publisher": {
        "id": "totalsportspro"
      }
    }
  }'
```

Check logs for bidder calls:

```bash
docker compose logs catalyst --tail=50 | grep bidder
```

## Troubleshooting

### Bidder Not Found

**Error:** `bidder not registered in database`

**Solution:** Add bidder:
```bash
./manage-bidders.sh add your_bidder 'Bidder Name' 'https://bidder.com/openrtb2'
```

### Timeout Issues

**Error:** Bids timing out

**Solutions:**
1. Increase bidder timeout:
   ```bash
   ./manage-bidders.sh update your_bidder timeout_ms 2000
   ```

2. Check endpoint URL is correct:
   ```bash
   ./manage-bidders.sh check your_bidder
   ```

3. Test endpoint directly:
   ```bash
   curl -X POST https://bidder.endpoint.com/openrtb2/auction \
     -H "Content-Type: application/json" \
     -d '{"id":"test","imp":[...]}'
   ```

### Endpoint Changes

**Scenario:** Bidder changed their endpoint URL

**Solution:**
```bash
./manage-bidders.sh update rubicon endpoint_url 'https://new-endpoint.com/openrtb2'
```

No code changes or deployment needed!

### Regional Routing

**Scenario:** Use different endpoints for different regions

**Solution:** Create region-specific bidder codes:
```bash
./manage-bidders.sh add rubicon-us 'Rubicon US' 'https://us.prebid-server.rubiconproject.com/openrtb2/auction'
./manage-bidders.sh add rubicon-eu 'Rubicon EU' 'https://eu.prebid-server.rubiconproject.com/openrtb2/auction'
```

Then configure publishers with appropriate regional bidder:
```bash
./manage-publishers.sh update publisher1 bidder_params '{"rubicon-us":{...}}'
./manage-publishers.sh update publisher2 bidder_params '{"rubicon-eu":{...}}'
```

## Advanced Features

### Custom HTTP Headers

Add authentication or custom headers to bidder requests:

```sql
UPDATE bidders
SET http_headers = '{"Authorization": "Bearer token123", "X-Custom-Header": "value"}'::jsonb
WHERE bidder_code = 'custom';
```

### A/B Testing Endpoints

Test new bidder endpoints before rolling out:

1. Create test bidder:
   ```bash
   ./manage-bidders.sh add rubicon-test 'Rubicon Test' 'https://test-endpoint.com/openrtb2' 1500 52
   ```

2. Configure test publisher:
   ```bash
   ./manage-publishers.sh update test-pub bidder_params '{"rubicon-test":{...}}'
   ```

3. Monitor performance, then promote:
   ```bash
   ./manage-bidders.sh update rubicon endpoint_url 'https://test-endpoint.com/openrtb2'
   ./manage-bidders.sh remove rubicon-test
   ```

### Format-Specific Bidders

Enable bidders only for specific formats:

```bash
# Video-only bidder
./manage-bidders.sh add video-ssp 'Video SSP' 'https://video.ssp.com/bid'
./manage-bidders.sh update video-ssp supports_banner false
./manage-bidders.sh update video-ssp supports_video true

# Native-only bidder
./manage-bidders.sh add native-ssp 'Native SSP' 'https://native.ssp.com/bid'
./manage-bidders.sh update native-ssp supports_banner false
./manage-bidders.sh update native-ssp supports_native true
```

## Best Practices

1. **Use descriptive bidder codes** - `rubicon-us` not `r1`
2. **Document changes** - Add description when adding bidders
3. **Test before enabling** - Use status='testing' initially
4. **Monitor timeouts** - Adjust timeout_ms based on bidder performance
5. **Keep GVL IDs updated** - For GDPR compliance
6. **Regional routing** - Use separate bidder codes for regions
7. **Backup configurations** - Regular PostgreSQL backups

## Backup & Restore

**Backup bidders table:**
```bash
docker exec catalyst-postgres pg_dump -U catalyst -d catalyst -t bidders > bidders_backup.sql
```

**Restore:**
```bash
cat bidders_backup.sql | docker exec -i catalyst-postgres psql -U catalyst -d catalyst
```

## Security

- **Endpoint validation**: Only HTTPS endpoints in production
- **Header sanitization**: Validate custom HTTP headers
- **Access control**: Limit who can modify bidder configs
- **Audit logging**: Track bidder configuration changes
- **Rate limiting**: Prevent bidder endpoint abuse

## Integration with Publishers

Publishers configure bidder-specific parameters in their `bidder_params`:

```bash
./manage-publishers.sh update totalsportspro bidder_params \
  '{
    "rubicon": {"accountId": 26298, "siteId": 556630, "zoneId": 3767186},
    "pubmatic": {"publisherId": "12345", "adSlot": "billboard"},
    "appnexus": {"placementId": 54321}
  }'
```

The bidders table provides the endpoint URLs, the publishers table provides the parameters.

## Support

For issues or questions:
- Check logs: `docker compose logs catalyst`
- Review bidder status: `./manage-bidders.sh check <bidder_code>`
- Test endpoint: `curl -X POST <endpoint_url>`
- Verify database: `docker compose ps postgres`
- Check documentation: `/Users/andrewstreets/tne-catalyst/deployment/PUBLISHER-MANAGEMENT.md`
