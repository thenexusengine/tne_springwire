# Publisher Management - PostgreSQL Guide

This guide explains how to manage publishers and their bidder configurations in Catalyst using PostgreSQL.

## Overview

Publisher configuration is stored in PostgreSQL (single source of truth) for:
- **Publisher identification** and domain validation
- **Bidder parameters** (accountIds, siteIds, placement IDs, etc.)
- **Static configuration** that rarely changes

Redis is reserved for high-speed data:
- User IDs / Extended IDs (EIDs)
- Auction state
- Rate limiting counters

## Architecture

```
PostgreSQL (Source of Truth)
├── publishers table
│   ├── publisher_id (e.g., "totalsportspro")
│   ├── name (e.g., "Total Sports Pro")
│   ├── allowed_domains (e.g., "totalsportspro.com")
│   └── bidder_params (JSONB with bidder configs)
│
└── On auction request:
    1. Validate publisher_id from site.publisher.id
    2. Check allowed_domains
    3. Load bidder_params for enabled bidders
    4. Merge params into bid requests
```

## Database Schema

The `publishers` table structure:

```sql
CREATE TABLE publishers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    publisher_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    allowed_domains TEXT NOT NULL,
    bidder_params JSONB DEFAULT '{}'::jsonb,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    contact_email VARCHAR(255)
);
```

### Bidder Parameters Format

Bidder parameters are stored as JSONB in this format:

```json
{
  "rubicon": {
    "accountId": 26298,
    "siteId": 556630,
    "zoneId": 3767186
  },
  "pubmatic": {
    "publisherId": "12345",
    "adSlot": "billboard"
  },
  "appnexus": {
    "placementId": 54321
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

### OpenX
```json
{
  "openx": {
    "unit": "12345"
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

## Bid Multiplier (Revenue Sharing)

The `bid_multiplier` field enables transparent revenue sharing between the platform and publishers. This allows Catalyst to take a percentage cut while ensuring publishers meet their floor prices.

### How It Works

The bid multiplier affects TWO critical points in the auction:

1. **Floor Prices** - Multiplied UP before auction
2. **Winning Bids** - Divided DOWN before returning to publisher

#### Example Flow (5% Platform Cut)

**Publisher Config:**
```json
{
  "publisher_id": "totalsportspro",
  "bid_multiplier": 1.05,
  "floor_price": 0.50
}
```

**Auction Flow:**
1. Publisher sets floor: £0.50
2. Catalyst multiplies: £0.50 × 1.05 = £0.525 (adjusted floor)
3. DSPs bid against £0.525 floor
4. Winning bid: £0.60 (meets adjusted floor)
5. Catalyst divides: £0.60 ÷ 1.05 = £0.571 (returned to publisher)
6. Platform keeps: £0.60 - £0.571 = £0.029 (~5%)

**Result:** Publisher receives £0.571 (above their £0.50 floor), platform keeps ~5%

### Multiplier Values

| Multiplier | Platform Cut | Publisher Receives | Example (£1 bid) |
|------------|--------------|-------------------|------------------|
| 1.00 | 0% | 100% | £1.00 → £1.00 |
| 1.05 | ~5% | ~95% | £1.00 → £0.95 |
| 1.11 | ~10% | ~90% | £1.00 → £0.90 |
| 1.25 | ~20% | ~80% | £1.00 → £0.80 |

**Formula:**
- Platform cut = 1 - (1 / multiplier)
- Publisher receives = bid / multiplier
- Adjusted floor = floor × multiplier

### Setting Bid Multipliers

**Add publisher with multiplier:**
```bash
# Publisher with 5% platform cut
./manage-publishers.sh add publisher1 'Publisher Name' 'domain.com' '{}'
./manage-publishers.sh update publisher1 bid_multiplier 1.05
```

**Update existing publisher:**
```bash
# Set 5% platform cut
./manage-publishers.sh update totalsportspro bid_multiplier 1.05

# Set 10% platform cut
./manage-publishers.sh update totalsportspro bid_multiplier 1.11

# Remove cut (100% to publisher)
./manage-publishers.sh update totalsportspro bid_multiplier 1.00
```

**Check current multiplier:**
```bash
./manage-publishers.sh check totalsportspro
```

Output:
```
═══════════════════════════════════════════════════
Publisher: totalsportspro
═══════════════════════════════════════════════════
  Name: Total Sports Pro
  Status: active
  Allowed Domains: totalsportspro.com
  Bid Multiplier: 1.05 (platform keeps ~4.8%)
  Bidder Params: {...}
  Created: 2026-01-13 22:00:00
```

### Database Schema

The `bid_multiplier` column:

```sql
ALTER TABLE publishers
ADD COLUMN bid_multiplier DECIMAL(6,4) DEFAULT 1.0000
CHECK (bid_multiplier >= 1.0000 AND bid_multiplier <= 10.0000);
```

- **Type:** DECIMAL(6,4) for precise calculations
- **Range:** 1.0000 to 10.0000 (0% to ~90% cut)
- **Default:** 1.0000 (no adjustment)

### Why This Approach?

**Protects Publisher Floors:**
- Publisher sets £0.50 floor
- Without multiplier adjustment, DSP bids £0.52
- After platform 5% cut: £0.494 (below publisher floor!) ❌

**With Multiplier:**
- Publisher sets £0.50 floor
- Floor adjusted to £0.525 before auction
- DSP bids £0.60 (meets adjusted floor)
- After platform cut: £0.571 (above publisher floor!) ✓

### Optimization

The `bid_multiplier` can be optimized based on:
- Publisher performance metrics
- Fill rates and win rates
- Competitive landscape
- Publisher tier (premium vs standard)

**Example tiered structure:**
```sql
-- Premium publishers (lower cut)
UPDATE publishers SET bid_multiplier = 1.03 WHERE tier = 'premium';

-- Standard publishers
UPDATE publishers SET bid_multiplier = 1.05 WHERE tier = 'standard';

-- Trial publishers (higher cut)
UPDATE publishers SET bid_multiplier = 1.10 WHERE tier = 'trial';
```

### Transparency

While the multiplier is transparent in the platform's operations, publishers see:
- Their configured floor prices remain unchanged
- Bid prices are always above their floors
- Consistent auction mechanics
- Platform takes agreed percentage

The multiplier is logged in debug mode:
```
Applied multiplier to floor price: imp=123 base_floor=0.50 multiplier=1.05 adjusted_floor=0.525
Applied bid multiplier: bidder=rubicon original=0.60 adjusted=0.571 platform_cut=0.029
```

## Management Script

Use `/Users/andrewstreets/tne-catalyst/deployment/manage-publishers.sh` to manage publishers.

### List Publishers

```bash
./manage-publishers.sh list
```

Output:
```
═══════════════════════════════════════════════════
Registered Publishers in Catalyst
═══════════════════════════════════════════════════

PUBLISHER ID         NAME                           DOMAINS                        STATUS
─────────────────────────────────────────────────────────────────────────────────────────
totalsportspro       Total Sports Pro               totalsportspro.com             active

Total: 1 publisher(s)
```

### Add Publisher

**Basic (no bidder params):**
```bash
./manage-publishers.sh add publisher_id 'Publisher Name' 'domain.com'
```

**With Rubicon parameters:**
```bash
./manage-publishers.sh add totalsportspro 'Total Sports Pro' 'totalsportspro.com' \
  '{"rubicon":{"accountId":26298,"siteId":556630,"zoneId":3767186}}'
```

**With multiple bidders:**
```bash
./manage-publishers.sh add pub123 'Multi-Bidder Publisher' 'example.com' \
  '{"rubicon":{"accountId":123,"siteId":456,"zoneId":789},"pubmatic":{"publisherId":"pub-456","adSlot":"billboard"},"appnexus":{"placementId":54321}}'
```

**Allow multiple domains:**
```bash
./manage-publishers.sh add pub123 'Publisher' 'example.com|*.example.com|test.com'
```

**Allow any domain (testing only):**
```bash
./manage-publishers.sh add testpub 'Test Publisher' '*'
```

### Check Publisher

```bash
./manage-publishers.sh check totalsportspro
```

Output:
```
═══════════════════════════════════════════════════
Publisher: totalsportspro
═══════════════════════════════════════════════════
  Name: Total Sports Pro
  Status: active
  Allowed Domains: totalsportspro.com
  Bidder Params: {"rubicon":{"accountId":26298,"siteId":556630,"zoneId":3767186}}
  Created: 2026-01-13 22:00:00

Domain Rules:
  • totalsportspro.com
```

### Update Publisher

**Update name:**
```bash
./manage-publishers.sh update totalsportspro name 'New Publisher Name'
```

**Update domains:**
```bash
./manage-publishers.sh update totalsportspro allowed_domains 'newdomain.com'
```

**Update bidder parameters:**
```bash
./manage-publishers.sh update totalsportspro bidder_params \
  '{"rubicon":{"accountId":999,"siteId":888,"zoneId":777}}'
```

**Pause publisher:**
```bash
./manage-publishers.sh update totalsportspro status 'paused'
```

**Reactivate publisher:**
```bash
./manage-publishers.sh update totalsportspro status 'active'
```

### Remove Publisher

```bash
./manage-publishers.sh remove totalsportspro
```

Note: This performs a soft delete (sets `status='archived'`). The publisher can be reactivated by updating status back to 'active'.

## How It Works

### 1. Auction Request

Prebid.js wrapper sends request:

```json
{
  "id": "auction-123",
  "imp": [{
    "id": "billboard",
    "banner": {"w": 728, "h": 90}
  }],
  "site": {
    "domain": "totalsportspro.com",
    "page": "https://totalsportspro.com/article",
    "publisher": {
      "id": "totalsportspro"
    }
  }
}
```

### 2. Publisher Validation

Catalyst checks PostgreSQL:
- Does `totalsportspro` exist?
- Is status `active`?
- Does `totalsportspro.com` match `allowed_domains`?

### 3. Bidder Parameter Injection

Catalyst loads bidder_params from PostgreSQL and merges into impression:

```json
{
  "imp": [{
    "id": "billboard",
    "banner": {"w": 728, "h": 90},
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

### 4. Parallel Bidder Calls

Catalyst makes parallel HTTP calls to configured bidders:
- Rubicon: `https://prebid-server.rubiconproject.com/openrtb2/auction`
- PubMatic: `https://hbopenbid.pubmatic.com/translator?source=prebid-server`
- AppNexus: `https://ib.adnxs.com/openrtb2`

### 5. Response

Best bids returned to Prebid.js wrapper.

## Environment Configuration

In `deployment/.env`:

```bash
# PostgreSQL Configuration
DB_HOST=postgres
DB_PORT=5432
DB_NAME=catalyst
DB_USER=catalyst
DB_PASSWORD=your_secure_password

# Publisher Authentication
PUBLISHER_AUTH_ENABLED=true
PUBLISHER_VALIDATE_DOMAIN=true
```

## Direct Database Access

Access PostgreSQL directly:

```bash
docker exec -it catalyst-postgres psql -U catalyst -d catalyst
```

### Useful Queries

**List all active publishers:**
```sql
SELECT publisher_id, name, allowed_domains, status
FROM publishers
WHERE status = 'active';
```

**Get publisher with bidder configs:**
```sql
SELECT publisher_id, name, bidder_params
FROM publishers
WHERE publisher_id = 'totalsportspro';
```

**Get specific bidder params:**
```sql
SELECT bidder_params->'rubicon' as rubicon_params
FROM publishers
WHERE publisher_id = 'totalsportspro';
```

**Count bidders per publisher:**
```sql
SELECT publisher_id, jsonb_object_keys(bidder_params) as bidders
FROM publishers
WHERE status = 'active';
```

**Update bidder params (add new bidder):**
```sql
UPDATE publishers
SET bidder_params = bidder_params || '{"pubmatic":{"publisherId":"12345","adSlot":"slot1"}}'::jsonb
WHERE publisher_id = 'totalsportspro';
```

## Testing

Test auction with your publisher:

```bash
curl -X POST http://localhost:8000/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-001",
    "imp": [{
      "id": "1",
      "banner": {"format": [{"w": 728, "h": 90}]},
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
        "id": "totalsportspro",
        "name": "Total Sports Pro"
      }
    }
  }'
```

Check logs:
```bash
docker compose logs catalyst --tail=50 | grep totalsportspro
```

## Troubleshooting

### Publisher Not Found

**Error:** `{"error":"unknown_publisher: publisher not registered"}`

**Solution:** Add publisher to database:
```bash
./manage-publishers.sh add your_pub_id 'Your Publisher' 'yourdomain.com'
```

### Domain Mismatch

**Error:** `{"error":"domain_mismatch: domain not allowed for publisher"}`

**Solution:** Update allowed domains:
```bash
./manage-publishers.sh update your_pub_id allowed_domains 'correct-domain.com'
```

### Database Connection Error

**Error:** `{"error":"database_error: failed to query publisher"}`

**Solution:** Check PostgreSQL is running:
```bash
docker compose ps postgres
docker compose logs postgres
```

### No Bidder Parameters

If bidders aren't receiving parameters, check:
```bash
./manage-publishers.sh check your_pub_id
```

Ensure `bidder_params` contains the required bidder configurations.

## Migration from Redis

If you previously used Redis for publishers:

1. **Export from Redis:**
```bash
docker exec catalyst-redis redis-cli HGETALL tne_catalyst:publishers
```

2. **Add to PostgreSQL:**
```bash
./manage-publishers.sh add <pub_id> '<name>' '<domains>' '<bidder_params>'
```

3. **Verify:**
```bash
./manage-publishers.sh list
```

## Best Practices

1. **Use specific domains** - Avoid `*` in production
2. **Document bidder params** - Add notes when adding publishers
3. **Test before production** - Use status='paused' initially
4. **Version control** - Keep publisher configs documented
5. **Regular backups** - Backup PostgreSQL publishers table
6. **Monitor logs** - Watch for publisher validation errors

## Backup & Restore

**Backup publishers table:**
```bash
docker exec catalyst-postgres pg_dump -U catalyst -d catalyst -t publishers > publishers_backup.sql
```

**Restore:**
```bash
cat publishers_backup.sql | docker exec -i catalyst-postgres psql -U catalyst -d catalyst
```

## Security

- Publisher validation is **enabled by default**
- Domain validation prevents unauthorized usage
- Use strong PostgreSQL passwords
- Limit PostgreSQL network access
- Regular security audits of bidder parameters
- Monitor for suspicious publisher activity

## Support

For issues or questions:
- Check logs: `docker compose logs catalyst`
- Review this documentation
- Check publisher status: `./manage-publishers.sh check <pub_id>`
- Verify database connection: `docker compose ps postgres`
