# Bidder-Specific Parameters Guide

**How to pass bidder-specific parameters to Catalyst PBS bidders**

This guide explains how to configure and pass required parameters to each bidder adapter.

---

## Overview

Each bidder adapter may require specific parameters to be passed in the bid request. These parameters are sent in the `imp.ext.{bidderName}` field of the OpenRTB bid request.

### Standard Format

```json
{
  "id": "auction-id",
  "imp": [{
    "id": "imp-1",
    "banner": {
      "w": 300,
      "h": 250
    },
    "ext": {
      "rubicon": {
        "accountId": 12345,
        "siteId": 67890,
        "zoneId": 123456
      },
      "appnexus": {
        "placementId": 13232354
      }
    }
  }],
  "site": {
    "domain": "example.com",
    "publisher": {
      "id": "pub123"
    }
  }
}
```

---

## Rubicon/Magnite Parameters

**Bidder Name**: `rubicon`

### Required Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `accountId` | integer | Your Rubicon account ID | `12345` |
| `siteId` | integer | Site ID in Rubicon platform | `67890` |
| `zoneId` | integer | Zone/placement ID | `123456` |

### Optional Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `inventory` | object | First-party inventory data | `{"rating": "5-star"}` |
| `visitor` | object | First-party visitor data | `{"ucat": ["news"]}` |
| `video` | object | Video-specific params | `{"language": "en"}` |

### Example Request

```json
{
  "id": "test-auction-123",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [
        {"w": 300, "h": 250},
        {"w": 728, "h": 90}
      ]
    },
    "ext": {
      "rubicon": {
        "accountId": 12345,
        "siteId": 67890,
        "zoneId": 123456,
        "inventory": {
          "rating": "5-star",
          "prodtype": "tech"
        }
      }
    }
  }],
  "site": {
    "domain": "techblog.example.com",
    "page": "https://techblog.example.com/articles/latest",
    "publisher": {
      "id": "pub123"
    }
  },
  "device": {
    "ua": "Mozilla/5.0...",
    "ip": "203.0.113.42"
  },
  "user": {
    "id": "user-xyz-789"
  }
}
```

### Client Integration

Your client (JS script or other integration) should construct the OpenRTB request with bidder params in `imp.ext`:

```json
POST /openrtb2/auction
Content-Type: application/json

{
  "id": "auction-123",
  "imp": [{
    "id": "1",
    "banner": {"format": [{"w": 300, "h": 250}]},
    "ext": {
      "rubicon": {
        "accountId": 12345,
        "siteId": 67890,
        "zoneId": 123456
      }
    }
  }],
  "site": {"domain": "example.com", "publisher": {"id": "pub123"}}
}
```

---

## AppNexus/Xandr Parameters

**Bidder Name**: `appnexus`

### Required Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `placementId` | integer | Placement ID from AppNexus | `13232354` |

**OR**

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `member` | string | Member ID | `"958"` |
| `invCode` | string | Inventory code | `"my_placement"` |

### Optional Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `keywords` | object | Key-value targeting | `{"genre": ["rock", "pop"]}` |
| `trafficSourceCode` | string | Traffic source identifier | `"my_source"` |
| `reserve` | float | Reserve price in USD | `0.50` |

### Example Request

```json
{
  "id": "test-auction-456",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [{"w": 300, "h": 250}]
    },
    "ext": {
      "appnexus": {
        "placementId": 13232354,
        "keywords": {
          "genre": ["rock", "pop"],
          "age": ["25-34"]
        },
        "reserve": 0.50
      }
    }
  }],
  "site": {
    "domain": "musicsite.example.com",
    "publisher": {
      "id": "pub123"
    }
  }
}
```

### Client Integration

```json
POST /openrtb2/auction
Content-Type: application/json

{
  "id": "auction-456",
  "imp": [{
    "id": "1",
    "banner": {"format": [{"w": 300, "h": 250}]},
    "ext": {
      "appnexus": {
        "placementId": 13232354,
        "keywords": {
          "genre": ["rock", "pop"],
          "age": ["25-34"]
        },
        "reserve": 0.50
      }
    }
  }],
  "site": {"domain": "example.com", "publisher": {"id": "pub123"}}
}
```

---

## PubMatic Parameters

**Bidder Name**: `pubmatic`

### Required Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `publisherId` | string | Publisher ID | `"156209"` |
| `adSlot` | string | Ad slot identifier | `"my-ad-unit@300x250"` |

### Optional Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `pmzoneid` | string | Zone ID | `"zone1"` |
| `lat` | float | Latitude for geo-targeting | `37.7749` |
| `lon` | float | Longitude for geo-targeting | `-122.4194` |
| `kadfloor` | string | Floor price | `"0.50"` |

### Example Request

```json
{
  "id": "test-auction-789",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [{"w": 300, "h": 250}]
    },
    "ext": {
      "pubmatic": {
        "publisherId": "156209",
        "adSlot": "homepage-banner@300x250",
        "pmzoneid": "zone1",
        "kadfloor": "0.50"
      }
    }
  }],
  "site": {
    "domain": "newssite.example.com",
    "publisher": {
      "id": "pub123"
    }
  }
}
```

---

## How Parameters Flow Through Catalyst

### 1. Client Builds OpenRTB Request

Your client (JS script, server-side integration, or other source) constructs the OpenRTB bid request with bidder-specific parameters in `imp.ext`:

```json
{
  "imp": [{
    "ext": {
      "rubicon": {
        "accountId": 12345,
        "siteId": 67890,
        "zoneId": 123456
      }
    }
  }]
}
```

### 2. Request Sent to Catalyst PBS

```bash
POST https://catalyst.springwire.ai/openrtb2/auction
Content-Type: application/json

{
  "id": "auction-123",
  "imp": [{
    "id": "1",
    "banner": {"w": 300, "h": 250},
    "ext": {
      "rubicon": {
        "accountId": 12345,
        "siteId": 67890,
        "zoneId": 123456
      }
    }
  }],
  "site": {
    "domain": "example.com",
    "publisher": {"id": "pub123"}
  }
}
```

### 3. Catalyst Processes Request

```
1. Auction handler receives request
2. Publisher validation (pub123 is registered)
3. Extract bidders from imp.ext (finds "rubicon")
4. Call Rubicon adapter with impression
5. Rubicon adapter extracts params from imp.ext.rubicon
6. Adapter builds request to Rubicon's endpoint
7. Send to Rubicon PBS with accountId/siteId/zoneId
```

### 4. Adapter Forwards to Bidder

The Rubicon adapter forwards the entire `imp.ext.rubicon` object to Rubicon's PBS endpoint at `https://prebid-server.rubiconproject.com/openrtb2/auction`, preserving all parameters.

### 5. Response Flow

```
1. Rubicon returns bid response
2. Adapter parses bids
3. Catalyst collects all bidder responses
4. Returns unified OpenRTB response to client
```

---

## Testing Bidder Parameters

### Test with curl

```bash
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-123",
    "imp": [{
      "id": "1",
      "banner": {"w": 300, "h": 250},
      "ext": {
        "rubicon": {
          "accountId": 12345,
          "siteId": 67890,
          "zoneId": 123456
        }
      }
    }],
    "site": {
      "domain": "example.com",
      "publisher": {"id": "pub123"}
    },
    "device": {
      "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
      "ip": "203.0.113.42"
    }
  }'
```

### Check Logs

```bash
# View auction logs to confirm parameters were passed
docker compose logs -f catalyst | grep -i "rubicon"

# Should see:
# - Rubicon adapter called
# - Parameters extracted from ext
# - Request sent to Rubicon endpoint
```

### Verify in Rubicon Dashboard

1. Log in to Rubicon/Magnite platform
2. Navigate to Reports → Bid Requests
3. Filter by Account ID (12345)
4. Verify requests are being received with correct siteId and zoneId

---

## Common Issues and Solutions

### Issue 1: "Missing required parameter"

**Symptom**: No bids from Rubicon, logs show parameter errors

**Cause**: Required parameters not in request

**Solution**: Verify imp.ext.rubicon contains accountId, siteId, and zoneId

```javascript
// ❌ Wrong - missing parameters
bids: [{
  bidder: 'rubicon',
  params: {}
}]

// ✅ Correct - all required params
bids: [{
  bidder: 'rubicon',
  params: {
    accountId: 12345,
    siteId: 67890,
    zoneId: 123456
  }
}]
```

### Issue 2: "Invalid accountId"

**Symptom**: Rubicon rejects requests with 401/403 errors

**Cause**: Wrong account credentials or inactive account

**Solution**:
1. Verify accountId in Rubicon dashboard
2. Ensure account is active
3. Check account has permission for programmatic bidding
4. Contact Rubicon support if credentials are incorrect

### Issue 3: Parameters Not Passed Through

**Symptom**: Rubicon receives requests but parameters are missing

**Cause**: Client not including params in imp.ext

**Solution**: Verify your client is building requests correctly

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

Check Catalyst logs to see what it's receiving:
```bash
docker compose logs -f catalyst | grep -i "imp.ext"
```

### Issue 4: Wrong Parameter Types

**Symptom**: Parameters sent but bidder rejects them

**Cause**: Sending string instead of integer, or vice versa

**Solution**: Use correct types as documented

```javascript
// ❌ Wrong - accountId as string
params: {
  accountId: "12345",  // Should be integer
  siteId: "67890",
  zoneId: "123456"
}

// ✅ Correct - proper types
params: {
  accountId: 12345,    // Integer
  siteId: 67890,       // Integer
  zoneId: 123456       // Integer
}
```

---

## Multi-Bidder Configuration

You can configure multiple bidders per impression:

```json
{
  "imp": [{
    "id": "1",
    "banner": {"w": 300, "h": 250},
    "ext": {
      "rubicon": {
        "accountId": 12345,
        "siteId": 67890,
        "zoneId": 123456
      },
      "appnexus": {
        "placementId": 13232354
      },
      "pubmatic": {
        "publisherId": "156209",
        "adSlot": "slot1@300x250"
      }
    }
  }]
}
```

Catalyst will:
1. Extract all bidders from imp.ext
2. Call each adapter in parallel
3. Return all bids to Prebid.js for auction

---

## Dynamic Bidder Configuration

For dynamically registered bidders (via Redis), parameters work the same way:

```json
{
  "imp": [{
    "ext": {
      "custom_bidder_123": {
        "param1": "value1",
        "param2": "value2"
      }
    }
  }]
}
```

See [DYNAMIC-BIDDERS-GUIDE.md](DYNAMIC-BIDDERS-GUIDE.md) for more on dynamic bidder management.

---

## Best Practices

### 1. Validate Parameters in Your Client

Ensure your client validates required parameters before sending to Catalyst:

```javascript
// Example: Client-side validation before sending
function validateRubiconParams(params) {
  const required = ['accountId', 'siteId', 'zoneId'];
  for (const field of required) {
    if (!params[field]) {
      throw new Error(`Rubicon: ${field} is required`);
    }
  }
  return true;
}
```

### 2. Use Environment-Specific Parameters

Configure different parameters for dev/staging/production:

```json
{
  "environments": {
    "dev": {
      "rubicon": {
        "accountId": 11111,
        "siteId": 22222,
        "zoneId": 33333
      }
    },
    "production": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }
}
```

### 3. Monitor Parameter Errors

```bash
# Check for parameter validation errors
docker compose logs catalyst | grep -i "parameter"
docker compose logs catalyst | grep -i "required"
docker compose logs catalyst | grep -i "invalid"
```

### 4. Document Your Parameters

Create a configuration file documenting all bidder parameters:

```json
// bidders-config.json
{
  "rubicon": {
    "accountId": 12345,
    "siteId": 67890,
    "zones": {
      "homepage_banner": 123456,
      "article_sidebar": 123457,
      "footer_leaderboard": 123458
    }
  },
  "appnexus": {
    "placements": {
      "homepage_banner": 13232354,
      "article_sidebar": 13232355
    }
  }
}
```

---

## Parameter Reference by Bidder

| Bidder | Key Param | Type | Required | Example |
|--------|-----------|------|----------|---------|
| **Rubicon** | accountId | int | ✅ | 12345 |
| | siteId | int | ✅ | 67890 |
| | zoneId | int | ✅ | 123456 |
| **AppNexus** | placementId | int | ✅ | 13232354 |
| | member | string | ❌ | "958" |
| | invCode | string | ❌ | "placement" |
| **PubMatic** | publisherId | string | ✅ | "156209" |
| | adSlot | string | ✅ | "slot@300x250" |
| **Demo** | (none) | - | - | - |

---

## Additional Resources

- **OpenRTB 2.5 Spec**: [IAB OpenRTB](https://www.iab.com/wp-content/uploads/2016/03/OpenRTB-API-Specification-Version-2-5-FINAL.pdf)
- **Prebid Server Docs**: [Prebid Server Overview](https://docs.prebid.org/prebid-server/overview/prebid-server-overview.html)
- **Rubicon/Magnite**: Contact your account manager for parameter documentation
- **AppNexus/Xandr**: [Xandr Documentation](https://docs.xandr.com/)
- **Catalyst Examples**: See `examples/` directory for working bid requests

---

**Need Help?**

- Check logs: `docker compose logs -f catalyst`
- Test endpoint: `https://catalyst.springwire.ai/info/bidders`
- Review README.md for general setup

---

**Last Updated**: 2026-01-13
**Version**: 1.0.0
