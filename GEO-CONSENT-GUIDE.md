# Geographic-Based Consent Filtering Guide

**Catalyst PBS - Multi-Regional Privacy Compliance**

This guide explains how Catalyst automatically detects user geographic location and enforces appropriate privacy regulations (GDPR, CCPA, etc.) based on where the user is located.

---

## Overview

Different regions have different privacy regulations. Catalyst now automatically:
1. **Detects user location** from `device.geo.country` and `device.geo.region`
2. **Determines applicable regulation** (GDPR for EU, CCPA for California, etc.)
3. **Validates appropriate consent** (TCF v2 for GDPR, US Privacy String for CCPA)
4. **Filters bidders** based on geo-specific consent requirements

**Key Benefit**: You don't need to manually configure which regulation applies - Catalyst detects it automatically from the user's geographic location.

---

## Supported Regulations

### GDPR (General Data Protection Regulation)
- **Applies to**: EU/EEA countries + UK
- **Consent Format**: TCF v2 consent string (`user.consent`)
- **Requirement**: `regs.gdpr=1` must be set
- **Countries**: Austria, Belgium, Bulgaria, Croatia, Cyprus, Czech Republic, Denmark, Estonia, Finland, France, Germany, Greece, Hungary, Ireland, Italy, Latvia, Lithuania, Luxembourg, Malta, Netherlands, Poland, Portugal, Romania, Slovakia, Slovenia, Spain, Sweden, United Kingdom, Iceland, Liechtenstein, Norway

### US State Privacy Laws

#### CCPA (California)
- **Applies to**: California, USA (region code: CA)
- **Consent Format**: US Privacy String (`regs.us_privacy`)
- **Requirement**: 4-character string like "1YNN"
- **Filtering**: Bidders filtered if position 2 = 'Y' (user opted out)

#### VCDPA (Virginia)
- **Applies to**: Virginia, USA (region code: VA)
- **Consent Format**: US Privacy String
- **Same logic as CCPA**

#### CPA (Colorado)
- **Applies to**: Colorado, USA (region code: CO)
- **Consent Format**: US Privacy String
- **Same logic as CCPA**

#### CTDPA (Connecticut)
- **Applies to**: Connecticut, USA (region code: CT)
- **Consent Format**: US Privacy String
- **Same logic as CCPA**

#### UCPA (Utah)
- **Applies to**: Utah, USA (region code: UT)
- **Consent Format**: US Privacy String
- **Same logic as CCPA**

### Other Regulations (Coming Soon)
- **LGPD** (Brazil) - Detected but not enforced yet
- **PIPEDA** (Canada) - Detected but not enforced yet
- **PDPA** (Singapore) - Detected but not enforced yet

---

## How It Works

### 1. Request Arrives

A bid request comes in with device geo data:

```json
{
  "id": "auction-123",
  "device": {
    "geo": {
      "country": "FRA",  // France (EU country)
      "region": "",
      "city": "Paris"
    },
    "ip": "203.0.113.42"
  },
  "regs": {
    "gdpr": 1
  },
  "user": {
    "consent": "COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB..."
  }
}
```

### 2. Privacy Middleware Validates Geo-Consent Match

**Step 1**: Detect applicable regulation from geo
- `device.geo.country = "FRA"` → Detected regulation: **GDPR**

**Step 2**: Validate appropriate consent string is present
- GDPR detected → Check if `regs.gdpr=1` ✓
- GDPR detected → Check if `user.consent` exists ✓

**Step 3**: If mismatch, block request
- EU user without `regs.gdpr=1` → **BLOCKED**
- EU user without `user.consent` → **BLOCKED**

### 3. Exchange Filters Bidders by Geo

For each bidder in the auction:

**Step 1**: Get bidder's GVL ID
- Rubicon: GVL ID 52
- PubMatic: GVL ID 76

**Step 2**: Check if bidder should be filtered based on geo
- Detected regulation: GDPR
- Parse TCF consent string
- Check if vendor consent bit for GVL ID 52 is set
- If NOT set → **SKIP BIDDER** (don't make HTTP call)

**Step 3**: Call bidders with consent
- Only bidders with proper consent participate

---

## Configuration

### Environment Variables

```bash
# Enable geo-aware consent enforcement (default: true)
PBS_GEO_ENFORCEMENT=true

# Enable GDPR enforcement (default: true)
PBS_ENFORCE_GDPR=true

# Enable CCPA enforcement (default: true)
PBS_ENFORCE_CCPA=true

# Strict mode - block requests without consent (default: true)
PBS_PRIVACY_STRICT_MODE=true
```

### Disabling Geo Enforcement

To disable geo-based filtering (use only request flags):

```bash
PBS_GEO_ENFORCEMENT=false
```

When disabled, Catalyst will only enforce regulations based on explicit flags in the request (`regs.gdpr`, `regs.us_privacy`) without validating they match the user's geo.

---

## Testing Geo-Based Consent

### Example 1: EU User with GDPR Consent

```json
POST /openrtb2/auction

{
  "id": "test-eu-001",
  "device": {
    "geo": {
      "country": "DEU",  // Germany (EU)
      "city": "Berlin"
    },
    "ua": "Mozilla/5.0...",
    "ip": "203.0.113.42"
  },
  "regs": {
    "gdpr": 1
  },
  "user": {
    "consent": "COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB..."
  },
  "imp": [{
    "id": "1",
    "banner": {"format": [{"w": 300, "h": 250}]},
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }],
  "site": {
    "domain": "example.com",
    "publisher": {"id": "pub123"}
  }
}
```

**Expected Behavior**:
- Detected regulation: **GDPR** (Germany is EU country)
- Validates `regs.gdpr=1` is present ✓
- Validates `user.consent` is present ✓
- Parses TCF consent string
- Checks if Rubicon (GVL ID 52) has consent
- If consent exists → Rubicon participates
- If no consent → Rubicon skipped

### Example 2: California User with CCPA Opt-Out

```json
POST /openrtb2/auction

{
  "id": "test-ca-001",
  "device": {
    "geo": {
      "country": "USA",
      "region": "CA",    // California
      "city": "Los Angeles"
    },
    "ua": "Mozilla/5.0...",
    "ip": "203.0.113.42"
  },
  "regs": {
    "us_privacy": "1YYN"  // Position 2 = 'Y' means opted out
  },
  "imp": [{
    "id": "1",
    "banner": {"format": [{"w": 300, "h": 250}]},
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }],
  "site": {
    "domain": "example.com",
    "publisher": {"id": "pub123"}
  }
}
```

**Expected Behavior**:
- Detected regulation: **CCPA** (California, USA)
- Validates `regs.us_privacy` is present ✓
- Parses US Privacy String: "1YYN"
  - Version: 1
  - Notice: Y (given)
  - Opt-Out: **Y (user HAS opted out)**
  - LSPA: N
- **ALL BIDDERS SKIPPED** (user opted out of data sale)

### Example 3: California User WITHOUT Opt-Out

```json
{
  "regs": {
    "us_privacy": "1YNN"  // Position 2 = 'N' means NOT opted out
  }
}
```

**Expected Behavior**:
- Detected regulation: **CCPA**
- US Privacy String: "1YNN"
  - Opt-Out: **N (user has NOT opted out)**
- **All bidders participate** (user has not opted out)

### Example 4: Non-Regulated Region

```json
{
  "device": {
    "geo": {
      "country": "JPN",  // Japan (no specific regulation)
      "city": "Tokyo"
    }
  }
}
```

**Expected Behavior**:
- Detected regulation: **NONE**
- No geo-based filtering applied
- Bidders participate normally

---

## Logs and Debugging

### Geo Detection Logs

```bash
# View geo-based filtering logs
docker compose logs -f catalyst | grep -i "geographic\|regulation"

# Expected output for EU user:
# INFO: Detected regulation regulation=GDPR country=DEU
# INFO: Skipping bidder - no consent for user's geographic location
#       bidder=rubicon gvl_id=52 regulation=GDPR country=DEU

# Expected output for CA user with opt-out:
# INFO: Detected regulation regulation=CCPA country=USA region=CA
# INFO: Skipping bidder - no consent for user's geographic location
#       bidder=rubicon regulation=CCPA country=USA region=CA
```

### Test Script

Run the geo-based consent test script:

```bash
./examples/test-geo-consent.sh
```

This will test three scenarios:
1. EU user with GDPR consent
2. California user with CCPA opt-out
3. Non-regulated region

---

## Error Scenarios

### Error 1: EU User Without GDPR Flag

**Request:**
```json
{
  "device": {"geo": {"country": "FRA"}},
  "regs": {}  // Missing gdpr flag
}
```

**Response: 400 Bad Request**
```json
{
  "error": "Privacy compliance violation",
  "reason": "User in EU/EEA but GDPR consent not provided (regs.gdpr must be 1)",
  "regulation": "GDPR",
  "nbr": 0
}
```

### Error 2: California User Without US Privacy String

**Request:**
```json
{
  "device": {"geo": {"country": "USA", "region": "CA"}},
  "regs": {}  // Missing us_privacy
}
```

**Response: 400 Bad Request**
```json
{
  "error": "Privacy compliance violation",
  "reason": "User in US privacy state but consent string not provided (regs.us_privacy required)",
  "regulation": "CCPA",
  "nbr": 0
}
```

### Error 3: Wrong Consent Type for Geo

**Request:**
```json
{
  "device": {"geo": {"country": "FRA"}},  // EU country
  "regs": {"us_privacy": "1YNN"}  // Wrong! Should be GDPR
}
```

**Response: 400 Bad Request**
```json
{
  "error": "Privacy compliance violation",
  "reason": "User in EU/EEA but GDPR consent not provided (regs.gdpr must be 1)",
  "regulation": "GDPR"
}
```

---

## Best Practices

### 1. Always Include Device Geo

Ensure your bid requests always include `device.geo.country` and `device.geo.region`:

```json
{
  "device": {
    "geo": {
      "country": "USA",  // ISO 3166-1 alpha-3
      "region": "CA"     // Two-letter state code
    }
  }
}
```

### 2. Implement Server-Side Geo Detection

If your client-side integration doesn't have geo data, use server-side IP geolocation:

```javascript
// Server-side (before sending to Catalyst)
const geo = await geolocateIP(clientIP);
bidRequest.device.geo = {
  country: geo.country,  // "USA"
  region: geo.region,    // "CA"
  city: geo.city
};
```

### 3. Pass Appropriate Consent Strings

Based on user location:
- **EU users** → Include `regs.gdpr=1` and `user.consent` (TCF v2)
- **US privacy states** → Include `regs.us_privacy` (4-character string)
- **Other regions** → No special requirements

### 4. Test with Multiple Geos

Test your integration with different geo configurations:

```bash
# Test EU
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -d @examples/bid-requests/eu-gdpr-request.json

# Test California
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -d @examples/bid-requests/us-ca-ccpa-request.json

# Test non-regulated
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -d @examples/bid-requests/non-regulated-request.json
```

### 5. Monitor Geo-Based Filtering

Track how often bidders are filtered by geo:

```bash
# Count filtered bidders by regulation
docker compose logs catalyst | \
  grep "no consent for user's geographic location" | \
  grep -oP 'regulation=\K\w+' | \
  sort | uniq -c

# Example output:
# 150 GDPR
#  45 CCPA
#   5 VCDPA
```

---

## Integration Examples

### Prebid.js Integration

When using Catalyst as a server-side Prebid Server:

```javascript
// pbjs-config.js
pbjs.setConfig({
  s2sConfig: {
    endpoint: 'https://catalyst.springwire.ai/openrtb2/auction',
    // Geo will be added automatically by your server
    // Consent will be read from CMP
  },
  consentManagement: {
    gdpr: {
      cmpApi: 'iab',
      timeout: 10000
    },
    usp: {
      cmpApi: 'iab',
      timeout: 100
    }
  }
});
```

### Server-to-Server Integration

```javascript
// server.js
const buildBidRequest = (userIP, consentData) => {
  const geo = await detectGeo(userIP);

  return {
    id: uuid(),
    device: {
      ua: req.headers['user-agent'],
      ip: userIP,
      geo: {
        country: geo.country,  // Auto-detected
        region: geo.region     // Auto-detected
      }
    },
    regs: {
      // Include appropriate flag based on geo
      ...(isEU(geo.country) && { gdpr: 1 }),
      ...(isUSPrivacyState(geo.region) && { us_privacy: consentData.uspString })
    },
    user: {
      // Include appropriate consent based on geo
      ...(isEU(geo.country) && { consent: consentData.tcfString })
    },
    // ... rest of bid request
  };
};
```

---

## Architecture

### Geo Detection Flow

```
1. Bid Request Arrives
   ↓
2. Privacy Middleware Checks
   ├─ Extract device.geo.country
   ├─ Extract device.geo.region
   ├─ Detect applicable regulation
   └─ Validate appropriate consent string present
   ↓
3. Exchange Pre-Auction Check
   ├─ For each bidder:
   │  ├─ Get bidder GVL ID
   │  ├─ Detect regulation from geo
   │  ├─ Check consent based on regulation
   │  └─ Skip if no consent
   └─ Call bidders with consent
   ↓
4. Auction Response
```

### Key Functions

**Privacy Middleware (`internal/middleware/privacy.go`)**:
- `detectApplicableRegulation()` - Determines regulation from geo
- `validateGeoConsent()` - Validates consent matches geo
- `DetectRegulationFromGeo()` - Static helper for exchange
- `ShouldFilterBidderByGeo()` - Checks if bidder should be filtered

**Exchange (`internal/exchange/exchange.go`)**:
- Lines 1056-1089: Static bidder geo-based filtering
- Lines 1123-1156: Dynamic bidder geo-based filtering

---

## Compliance Notes

**Legal Disclaimer**: This implementation assists with technical compliance but does not constitute legal advice. Consult with legal counsel to ensure your use of Catalyst PBS meets all applicable privacy law requirements.

**Key Points**:
1. Catalyst detects applicable regulation based on user geo
2. Appropriate consent is validated before allowing data processing
3. Bidders without consent are excluded from auctions
4. Publishers must obtain valid consent via certified CMPs
5. All geo-to-regulation mappings should be reviewed with legal counsel

---

## Additional Resources

- **[TCF-VENDOR-CONSENT-GUIDE.md](TCF-VENDOR-CONSENT-GUIDE.md)** - GDPR/TCF v2 implementation details
- **[BIDDER-PARAMS-GUIDE.md](BIDDER-PARAMS-GUIDE.md)** - Bidder configuration guide
- **[README.md](README.md)** - Main Catalyst documentation
- **IAB TCF Specifications**: https://github.com/InteractiveAdvertisingBureau/GDPR-Transparency-and-Consent-Framework
- **IAB US Privacy String**: https://github.com/InteractiveAdvertisingBureau/USPrivacy

---

**Last Updated**: 2026-01-13
**Version**: 1.0.0
**Catalyst PBS**: Geographic-Based Privacy Compliance
