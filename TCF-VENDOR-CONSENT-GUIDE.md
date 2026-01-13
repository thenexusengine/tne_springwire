# TCF Vendor Consent Validation Guide

**Catalyst PBS - GDPR Compliance with IAB TCF v2**

This guide explains how Catalyst enforces IAB Transparency & Consent Framework (TCF) v2 vendor consent requirements for GDPR compliance.

---

## Overview

When GDPR applies to a bid request (`regs.gdpr=1`), Catalyst automatically validates that users have consented to each bidder's Global Vendor List (GVL) ID before allowing that bidder to participate in the auction.

**Key Features:**
- **Automatic TCF v2 parsing** - Extracts vendor consents from base64-encoded consent strings
- **Pre-auction filtering** - Bidders without consent are excluded before making HTTP calls
- **Support for all bidders** - Works with both static and dynamic bidders
- **Performance optimized** - Consent check happens in-memory before network calls

---

## How It Works

### 1. GDPR Detection

Catalyst checks if GDPR applies to a request:

```json
{
  "regs": {
    "gdpr": 1
  }
}
```

When `regs.gdpr=1`, vendor consent validation is enforced.

### 2. Consent String Parsing

The TCF v2 consent string is extracted from the bid request:

```json
{
  "user": {
    "consent": "COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB..."
  }
}
```

Catalyst parses this base64-encoded string to extract:
- Purpose consents (24 purposes)
- **Vendor consents** (variable list of GVL vendor IDs)
- Vendor list version
- Consent metadata

### 3. Bidder GVL ID Lookup

Each bidder has a registered GVL ID:

**Static Bidders (built-in):**
- Rubicon/Magnite: GVL ID **52**
- PubMatic: GVL ID **76**
- AppNexus/Xandr: GVL ID **32**

**Dynamic Bidders (Redis configuration):**
```json
{
  "bidder_code": "custom_ssp",
  "gvl_vendor_id": 123,
  ...
}
```

### 4. Consent Validation

Before calling each bidder, Catalyst checks:

```
IF regs.gdpr == 1:
  GET bidder.gvl_vendor_id
  IF gvl_vendor_id > 0:
    CHECK user.consent contains consent for gvl_vendor_id
    IF NOT consented:
      SKIP bidder (don't make HTTP request)
      LOG: "Skipping bidder - no TCF vendor consent"
```

### 5. Auction Response

Bidders without consent:
- **Not called** (no HTTP request made)
- **Excluded from auction** (no bids returned)
- **Logged** for debugging

Bidders with consent proceed normally and can compete for ad placements.

---

## TCF v2 Consent String Format

### Structure

The TCF v2 consent string is a base64url-encoded binary structure:

```
Core String Section (bits):
├─ Version (6 bits) = 2
├─ Created (36 bits) - timestamp
├─ Last Updated (36 bits)
├─ CMP ID (12 bits)
├─ CMP Version (12 bits)
├─ Consent Screen (6 bits)
├─ Consent Language (12 bits) - e.g., "en"
├─ Vendor List Version (12 bits)
├─ TCF Policy Version (6 bits)
├─ IsServiceSpecific (1 bit)
├─ UseNonStandardStacks (1 bit)
├─ Special Feature Opt Ins (12 bits)
├─ Purpose Consents (24 bits) - one per purpose
├─ Purpose Legitimate Interests (24 bits)
├─ Special Purposes (12 bits)
├─ Features (24 bits)
├─ Special Features (12 bits)
├─ Purpose Legitimate Interest (24 bits)
├─ NumEntries for Publisher (12 bits)
├─ MaxVendorId (16 bits)
└─ Vendor Consents Section:
    ├─ IsRangeEncoding (1 bit)
    └─ IF BitField:
        └─ Vendor bits (MaxVendorId bits)
       IF Range:
        ├─ NumEntries (12 bits)
        └─ For each entry:
            ├─ IsRange (1 bit)
            └─ IF single:
                └─ VendorID (16 bits)
               IF range:
                ├─ StartVendorID (16 bits)
                └─ EndVendorID (16 bits)
```

### Example Consent String

```
COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB...
```

When parsed, this might contain:
- Purpose consents: `[true, true, false, true, ...]` (24 purposes)
- Vendor consents: `{52: true, 76: true, 32: true, ...}` (GVL IDs)

---

## Configuration

### Static Bidders

GVL IDs are hardcoded in bidder adapters:

```go
// internal/adapters/rubicon/rubicon.go
func Info() adapters.BidderInfo {
    return adapters.BidderInfo{
        Enabled: true,
        GVLVendorID: 52, // Rubicon/Magnite's GVL ID
        ...
    }
}
```

### Dynamic Bidders

GVL IDs are set in Redis configuration:

```bash
redis-cli HSET tne_catalyst:bidders:custom_ssp config '{
  "bidder_code": "custom_ssp",
  "name": "Custom SSP",
  "gvl_vendor_id": 123,
  "endpoint": {
    "url": "https://ssp.example.com/bid",
    "timeout_ms": 500
  }
}'
```

**Important:** Dynamic bidders **must** set `gvl_vendor_id` if they require GDPR consent checking.

---

## Testing TCF Consent

### Example: Test Request with GDPR Consent

```json
POST /openrtb2/auction

{
  "id": "test-tcf-001",
  "imp": [{
    "id": "1",
    "banner": {"format": [{"w": 300, "h": 250}]},
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      },
      "pubmatic": {
        "publisherId": "156209",
        "adSlot": "slot1@300x250"
      }
    }
  }],
  "regs": {
    "gdpr": 1
  },
  "user": {
    "consent": "COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB..."
  },
  "site": {
    "domain": "example.com",
    "publisher": {"id": "pub123"}
  }
}
```

### Check Logs for Consent Validation

```bash
# View consent checking logs
docker compose logs -f catalyst | grep -i "TCF\|consent\|gvl"

# Expected output:
# INFO: Checking TCF vendor consent for bidder=rubicon gvl_id=52
# INFO: Bidder rubicon has consent (GVL ID 52)
# INFO: Checking TCF vendor consent for bidder=pubmatic gvl_id=76
# INFO: Skipping bidder - no TCF vendor consent bidder=pubmatic gvl_id=76
```

### Test Without Consent

Send a request with `regs.gdpr=1` but no `user.consent` string:

```json
{
  "regs": {"gdpr": 1},
  "user": {}
}
```

**Result:** All bidders with GVL IDs will be skipped (no consent string = no consent).

---

## Bidder GVL ID Reference

### Static Bidders (Built-in)

| Bidder | GVL ID | IAB Vendor Name |
|--------|--------|-----------------|
| rubicon | 52 | Magnite, Inc. |
| pubmatic | 76 | PubMatic, Inc. |
| appnexus | 32 | Xandr, Inc. |
| openx | 69 | OpenX |
| triplelift | 28 | TripleLift, Inc. |
| criteo | 91 | Criteo SA |
| medianet | 142 | Media.net |

### Verifying GVL IDs

Check the official IAB Global Vendor List:
- **Live GVL**: https://vendor-list.consensu.org/v2/vendor-list.json
- Search for vendor by name to find their GVL ID
- Example for Magnite/Rubicon:
  ```json
  "52": {
    "name": "Magnite, Inc.",
    "purposeIds": [1, 2, 3, 4, 7, 9],
    ...
  }
  ```

---

## Troubleshooting

### Issue 1: All Bidders Skipped

**Symptom:** Logs show all bidders skipped due to missing consent

**Possible Causes:**
1. **No consent string** - `user.consent` is missing or empty
2. **Invalid consent string** - Cannot be parsed (malformed base64)
3. **No vendor consents in string** - CMP didn't collect vendor consents

**Solution:**
- Verify `user.consent` is present in request
- Validate consent string with TCF decoder: https://iabtcf.com/#/decode
- Check CMP configuration to ensure vendor consents are being collected

### Issue 2: Specific Bidder Always Skipped

**Symptom:** One bidder consistently lacks consent while others work

**Possible Causes:**
1. **Wrong GVL ID** - Bidder has incorrect GVL ID configured
2. **User didn't consent** - User specifically opted out of this vendor
3. **GVL ID not in consent string** - Vendor not in CMP's vendor list

**Solution:**
- Verify bidder's GVL ID against IAB vendor list
- Check CMP vendor list includes this vendor
- Review user's actual consents using TCF decoder

### Issue 3: Dynamic Bidder Not Checking Consent

**Symptom:** Dynamic bidder ignores GDPR when it shouldn't

**Possible Causes:**
1. **No GVL ID configured** - `gvl_vendor_id` is null or 0
2. **Wrong config format** - GVL ID is string instead of integer

**Solution:**
```bash
# Check dynamic bidder config
redis-cli HGET tne_catalyst:bidders:custom_ssp config

# Verify gvl_vendor_id field:
{
  "gvl_vendor_id": 123  // Should be integer, not null or 0
}
```

### Issue 4: False Positives (Bidder Called Without Consent)

**Symptom:** Bidder participates even without consent

**Possible Causes:**
1. **GVL ID is 0** - Bidder has no GVL ID set (bypasses check)
2. **GDPR not flagged** - `regs.gdpr` is 0 or missing

**Solution:**
- Ensure bidder has valid GVL ID > 0
- Verify `regs.gdpr=1` in request
- Check privacy middleware is enabled

---

## Best Practices

### 1. Register All Bidders with IAB

Ensure all your demand partners are registered with the IAB Global Vendor List and have valid GVL IDs.

### 2. Configure Dynamic Bidders Correctly

Always set `gvl_vendor_id` for dynamic bidders:

```json
{
  "bidder_code": "new_ssp",
  "gvl_vendor_id": 456,  // Required for GDPR compliance
  "endpoint": {...}
}
```

### 3. Test with Real Consent Strings

Use your CMP to generate real consent strings for testing:
- Grant consent to specific vendors
- Deny consent to others
- Verify Catalyst behavior matches expectations

### 4. Monitor Consent Logs

Track how often bidders are skipped due to lack of consent:

```bash
# Count skipped bidders
docker compose logs catalyst | grep "no TCF vendor consent" | wc -l

# Group by bidder
docker compose logs catalyst | grep "no TCF vendor consent" | \
  grep -oP 'bidder=\K\w+' | sort | uniq -c
```

### 5. Update GVL IDs Regularly

Vendors may change their GVL IDs or add new purposes. Monitor IAB updates:
- Subscribe to IAB TCF announcements
- Regularly check vendor list for changes
- Update bidder configs as needed

---

## Privacy Middleware Integration

### How It Works

The privacy middleware enforces TCF purpose consents (Purpose 1, 2, 7):
- Purpose 1: Storage and access
- Purpose 2: Basic ads
- Purpose 7: Measure ad performance

The exchange enforces TCF vendor consents:
- Checks each bidder's GVL ID
- Only calls bidders with user consent

This two-tier approach ensures:
1. **Baseline compliance** - Required purposes enforced globally
2. **Vendor-specific compliance** - Each bidder checked individually

### Configuration

Privacy middleware settings (environment variables):

```bash
# Enable GDPR enforcement (default: true)
PBS_ENFORCE_GDPR=true

# Strict mode - block requests with missing purposes (default: true)
PBS_PRIVACY_STRICT_MODE=true

# Anonymize IPs when GDPR applies (default: true)
PBS_ANONYMIZE_IP=true
```

---

## Code Reference

### Key Files

**Privacy Middleware:**
- `internal/middleware/privacy.go` - TCF parsing and vendor consent checking

**Exchange:**
- `internal/exchange/exchange.go` - Pre-auction consent validation (lines 1056-1080, 1114-1138)

**Adapters:**
- `internal/adapters/adapter.go` - BidderInfo with GVLVendorID field
- `internal/adapters/rubicon/rubicon.go` - Rubicon GVL ID (52)
- `internal/adapters/pubmatic/pubmatic.go` - PubMatic GVL ID (76)
- `internal/adapters/appnexus/appnexus.go` - AppNexus GVL ID (32)

**Dynamic Bidders:**
- `internal/adapters/ortb/ortb.go` - GenericAdapter.GetGVLVendorID()

### TCF Parsing Functions

```go
// Parse TCF v2 consent string
tcfData, err := parseTCFv2String(consentString)

// Check vendor consent
hasConsent := tcfData.VendorConsents[gvlID]

// Static helper (no middleware instance needed)
hasConsent := middleware.CheckVendorConsentStatic(consentString, gvlID)
```

---

## IAB TCF Resources

- **IAB TCF v2 Specification**: https://github.com/InteractiveAdvertisingBureau/GDPR-Transparency-and-Consent-Framework
- **Global Vendor List**: https://vendor-list.consensu.org/
- **TCF Policy**: https://iabeurope.eu/tcf-2-0/
- **Consent String Decoder**: https://iabtcf.com/#/decode

---

## Compliance Notes

**Legal Disclaimer:** This implementation assists with technical compliance but does not constitute legal advice. Consult with legal counsel to ensure your use of Catalyst PBS meets all applicable GDPR requirements.

**Key Points:**
1. Catalyst enforces vendor consent when `regs.gdpr=1`
2. Bidders without consent are excluded from auctions
3. Publishers are responsible for obtaining valid consent via a certified CMP
4. Consent strings must be passed in `user.consent` field
5. All bidders must have valid GVL IDs for enforcement to work

---

**Last Updated**: 2026-01-13
**Version**: 1.0.0
**Catalyst PBS**: TCF v2 Compliance
