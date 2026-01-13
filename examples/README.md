# Catalyst Example Bid Requests

This directory contains example OpenRTB bid requests and test scripts for Catalyst PBS.

## Architecture

Catalyst is a **server-side PBS (Prebid Server)** that receives OpenRTB 2.x bid requests. Your client (JavaScript, server-side integration, or other) constructs these requests and sends them to:

```
POST https://catalyst.springwire.ai/openrtb2/auction
Content-Type: application/json
```

These examples show the exact request format Catalyst expects, including bidder-specific parameters.

## Files

### Bid Request Examples

**rubicon-bid-request.json**
- Complete OpenRTB bid request for Rubicon/Magnite
- Includes your actual credentials (accountId: 26298, siteId: 556630, zoneId: 3767186)
- Shows proper formatting for banner impressions
- Ready to send from your client to Catalyst

**multi-bidder-request.json**
- OpenRTB request with multiple bidders in a single auction
- Includes Rubicon, AppNexus, and PubMatic
- Shows how to pass different parameters to each bidder
- Demonstrates geo-targeting and content categories

### Test Scripts

**test-rubicon-params.sh**
- Automated test script for Rubicon bidder parameters
- Pre-configured with your Rubicon credentials
- Color-coded output for easy debugging
- Includes verification steps and troubleshooting tips

**test-tcf-consent.sh**
- Tests TCF v2 vendor consent validation for GDPR compliance
- Demonstrates three scenarios: with consent, without consent, and no GDPR
- Shows how Catalyst skips bidders without proper TCF consent
- Validates GVL ID checking for Rubicon (52), PubMatic (76), and AppNexus (32)

## Usage

### Test Rubicon Parameters

```bash
# Run with default credentials (your Rubicon account)
./test-rubicon-params.sh

# Test with different credentials
./test-rubicon-params.sh <account_id> <site_id> <zone_id>

# Test against local Catalyst instance
CATALYST_URL=http://localhost:8000 ./test-rubicon-params.sh
```

### Test TCF Vendor Consent

```bash
# Run TCF consent validation tests
./test-tcf-consent.sh

# Test against local Catalyst instance
CATALYST_URL=http://localhost:8000 ./test-tcf-consent.sh

# Check logs to see consent validation in action
docker compose logs -f catalyst | grep -i "TCF\|consent\|gvl"
```

This script tests three scenarios:
1. **GDPR with consent** - Bidders with GVL IDs in consent string participate
2. **GDPR without consent** - All bidders skipped (no consent string provided)
3. **No GDPR flag** - Bidders participate normally (consent not required)

### Test Geographic-Based Consent

```bash
# Run geo-based consent filtering tests
./test-geo-consent.sh

# Test against local Catalyst instance
CATALYST_URL=http://localhost:8000 ./test-geo-consent.sh

# Check logs to see geo-based filtering in action
docker compose logs -f catalyst | grep -i "geographic\|regulation"
```

This script tests five scenarios across different geographic regions:
1. **EU (Germany) with GDPR consent** - Bidders with consent participate
2. **US-CA with CCPA opt-out** - All bidders skipped (user opted out)
3. **US-CA without opt-out** - Bidders participate normally
4. **Japan (no specific law)** - Bidders participate without restrictions
5. **EU without GDPR flag** - Request blocked (400 error)

**Key Features:**
- Automatically detects applicable regulation from `device.geo.country` and `device.geo.region`
- Validates correct consent string for detected region (TCF for EU, US Privacy for CA)
- Filters bidders based on geo-specific consent requirements
- Supports GDPR (EU/EEA), CCPA (California), and other US state laws (Virginia, Colorado, Connecticut, Utah)

See **[GEO-CONSENT-GUIDE.md](../GEO-CONSENT-GUIDE.md)** for complete documentation on geographic-based consent filtering.

### Test with curl

**Single bidder (Rubicon):**
```bash
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d @rubicon-bid-request.json
```

**Multiple bidders:**
```bash
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d @multi-bidder-request.json
```

### Modify for Your Use Case

**1. Change Publisher ID:**
```json
{
  "site": {
    "publisher": {
      "id": "your_publisher_id"
    }
  }
}
```

**2. Change Domain:**
```json
{
  "site": {
    "domain": "yourdomain.com",
    "page": "https://yourdomain.com/page"
  }
}
```

**3. Change Ad Sizes:**
```json
{
  "imp": [{
    "banner": {
      "format": [
        {"w": 320, "h": 50},    // Mobile banner
        {"w": 970, "h": 250}    // Large leaderboard
      ]
    }
  }]
}
```

**4. Add Multiple Impressions:**
```json
{
  "imp": [
    {
      "id": "1",
      "banner": {"format": [{"w": 300, "h": 250}]},
      "ext": {"rubicon": {...}}
    },
    {
      "id": "2",
      "banner": {"format": [{"w": 728, "h": 90}]},
      "ext": {"rubicon": {...}}
    }
  ]
}
```

## Troubleshooting

### No Bids Returned

**Check 1: Publisher Registration**
```bash
cd ../deployment
./manage-publishers.sh check pub123
```

**Check 2: Catalyst Logs**
```bash
docker compose logs -f catalyst | grep -i rubicon
```

**Check 3: Bid Request Format**
Validate your JSON:
```bash
cat rubicon-bid-request.json | jq '.'
```

### Authentication Errors (401/403)

Make sure your publisher is registered:
```bash
cd ../deployment
./manage-publishers.sh add pub123 "yourdomain.com"
```

### Invalid Parameters

Verify your Rubicon credentials in the Magnite dashboard:
- Account ID: 26298
- Site ID: 556630
- Zone ID: 3767186

If these don't match your actual credentials, update the examples:
```bash
# Update all examples with your credentials
sed -i '' 's/"accountId": 26298/"accountId": YOUR_ACCOUNT_ID/g' *.json
sed -i '' 's/"siteId": 556630/"siteId": YOUR_SITE_ID/g' *.json
sed -i '' 's/"zoneId": 3767186/"zoneId": YOUR_ZONE_ID/g' *.json
```

## Creating Your Own Examples

### Template for New Bidder

```json
{
  "id": "my-test-auction",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [{"w": 300, "h": 250}]
    },
    "ext": {
      "NEW_BIDDER": {
        "param1": "value1",
        "param2": "value2"
      }
    }
  }],
  "site": {
    "domain": "example.com",
    "publisher": {"id": "pub123"}
  },
  "device": {
    "ua": "Mozilla/5.0...",
    "ip": "203.0.113.42"
  }
}
```

### Test Your Custom Request

```bash
# Save as my-test-request.json
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d @my-test-request.json | jq '.'
```

## Additional Resources

- **[BIDDER-PARAMS-GUIDE.md](../BIDDER-PARAMS-GUIDE.md)** - Complete bidder parameter documentation
- **[PUBLISHER-CONFIG-GUIDE.md](../PUBLISHER-CONFIG-GUIDE.md)** - Publisher configuration guide
- **[README.md](../README.md)** - Main Catalyst documentation

## Your Rubicon Credentials

For reference, your Rubicon/Magnite credentials used in these examples:

- **Account ID**: 26298
- **Site ID**: 556630
- **Zone ID**: 3767186

These are already configured in:
- `rubicon-bid-request.json`
- `multi-bidder-request.json`
- `test-rubicon-params.sh`

**Security Note**: These examples are safe to commit to your repository as they only contain Rubicon placement IDs, not sensitive API keys.
