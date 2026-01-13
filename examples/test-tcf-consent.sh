#!/bin/bash
# Test TCF Vendor Consent Validation
# This script demonstrates how Catalyst validates TCF v2 vendor consent for GDPR compliance

CATALYST_URL="${CATALYST_URL:-https://catalyst.springwire.ai}"
PUBLISHER_ID="${PUBLISHER_ID:-pub123}"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Testing TCF Vendor Consent Validation${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo ""

# Test 1: Request WITH GDPR and valid consent (includes Rubicon GVL ID 52)
echo -e "${YELLOW}Test 1: GDPR with Valid Consent${NC}"
echo "Testing with consent string that includes Rubicon (GVL ID 52)"
echo ""

# Note: This is a simplified example consent string for demonstration
# In production, use a real TCF v2 consent string from your CMP
CONSENT_STRING_WITH_RUBICON="COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB"

REQUEST_1=$(cat <<EOF
{
  "id": "test-tcf-with-consent-$(date +%s)",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [{"w": 300, "h": 250}]
    },
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }],
  "regs": {
    "gdpr": 1
  },
  "user": {
    "id": "test-user-001",
    "consent": "$CONSENT_STRING_WITH_RUBICON"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
  },
  "device": {
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  }
}
EOF
)

RESPONSE_1=$(curl -s -w "\n%{http_code}" -X POST "$CATALYST_URL/openrtb2/auction" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_1")

RESPONSE_BODY_1=$(echo "$RESPONSE_1" | head -n -1)
HTTP_CODE_1=$(echo "$RESPONSE_1" | tail -n 1)

echo -e "${BLUE}Response Status: $HTTP_CODE_1${NC}"
if [ "$HTTP_CODE_1" = "200" ]; then
    echo -e "${GREEN}✓ Request accepted${NC}"
    BID_COUNT=$(echo "$RESPONSE_BODY_1" | jq -r '.seatbid | length' 2>/dev/null || echo "0")
    if [ "$BID_COUNT" != "0" ] && [ "$BID_COUNT" != "null" ]; then
        echo -e "${GREEN}✓ Rubicon bidder participated (has consent)${NC}"
    else
        echo -e "${YELLOW}⚠ No bids returned (may be test environment)${NC}"
    fi
else
    echo -e "${RED}✗ Request failed${NC}"
    echo "$RESPONSE_BODY_1" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_1"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Test 2: Request WITH GDPR but NO consent string
echo -e "${YELLOW}Test 2: GDPR WITHOUT Consent String${NC}"
echo "Testing without consent string - all bidders should be skipped"
echo ""

REQUEST_2=$(cat <<EOF
{
  "id": "test-tcf-no-consent-$(date +%s)",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [{"w": 300, "h": 250}]
    },
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
    "id": "test-user-002"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
  },
  "device": {
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  }
}
EOF
)

RESPONSE_2=$(curl -s -w "\n%{http_code}" -X POST "$CATALYST_URL/openrtb2/auction" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_2")

RESPONSE_BODY_2=$(echo "$RESPONSE_2" | head -n -1)
HTTP_CODE_2=$(echo "$RESPONSE_2" | tail -n 1)

echo -e "${BLUE}Response Status: $HTTP_CODE_2${NC}"
if [ "$HTTP_CODE_2" = "200" ]; then
    echo -e "${GREEN}✓ Request accepted${NC}"
    BID_COUNT=$(echo "$RESPONSE_BODY_2" | jq -r '.seatbid | length' 2>/dev/null || echo "0")
    if [ "$BID_COUNT" = "0" ] || [ "$BID_COUNT" = "null" ]; then
        echo -e "${GREEN}✓ All bidders skipped (no consent)${NC}"
    else
        echo -e "${YELLOW}⚠ Some bids returned (unexpected)${NC}"
    fi
else
    echo -e "${RED}✗ Request failed${NC}"
    echo "$RESPONSE_BODY_2" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_2"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Test 3: Request WITHOUT GDPR (no consent required)
echo -e "${YELLOW}Test 3: No GDPR Flag (Consent Not Required)${NC}"
echo "Testing without GDPR flag - bidders should participate normally"
echo ""

REQUEST_3=$(cat <<EOF
{
  "id": "test-tcf-no-gdpr-$(date +%s)",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [{"w": 300, "h": 250}]
    },
    "ext": {
      "rubicon": {
        "accountId": 26298,
        "siteId": 556630,
        "zoneId": 3767186
      }
    }
  }],
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
  },
  "device": {
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  },
  "user": {
    "id": "test-user-003"
  }
}
EOF
)

RESPONSE_3=$(curl -s -w "\n%{http_code}" -X POST "$CATALYST_URL/openrtb2/auction" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_3")

RESPONSE_BODY_3=$(echo "$RESPONSE_3" | head -n -1)
HTTP_CODE_3=$(echo "$RESPONSE_3" | tail -n 1)

echo -e "${BLUE}Response Status: $HTTP_CODE_3${NC}"
if [ "$HTTP_CODE_3" = "200" ]; then
    echo -e "${GREEN}✓ Request accepted${NC}"
    BID_COUNT=$(echo "$RESPONSE_BODY_3" | jq -r '.seatbid | length' 2>/dev/null || echo "0")
    if [ "$BID_COUNT" != "0" ] && [ "$BID_COUNT" != "null" ]; then
        echo -e "${GREEN}✓ Bidders participated (GDPR not applicable)${NC}"
    else
        echo -e "${YELLOW}⚠ No bids returned (may be test environment)${NC}"
    fi
else
    echo -e "${RED}✗ Request failed${NC}"
    echo "$RESPONSE_BODY_3" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_3"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Summary
echo -e "${YELLOW}Test Summary:${NC}"
echo ""
echo "1. GDPR=1 with consent → Bidders with GVL ID in consent participate"
echo "2. GDPR=1 without consent → All bidders skipped (no consent string)"
echo "3. GDPR=0 or missing → Bidders participate normally (consent not required)"
echo ""
echo -e "${YELLOW}Verification Steps:${NC}"
echo ""
echo "1. Check Catalyst logs for TCF consent validation:"
echo "   docker compose logs -f catalyst | grep -i 'TCF\\|consent\\|gvl'"
echo ""
echo "2. Look for log messages like:"
echo "   - 'Checking TCF vendor consent for bidder=rubicon gvl_id=52'"
echo "   - 'Skipping bidder - no TCF vendor consent'"
echo "   - 'Bidder has consent (GVL ID 52)'"
echo ""
echo "3. Verify bidder GVL IDs:"
echo "   - Rubicon/Magnite: GVL ID 52"
echo "   - PubMatic: GVL ID 76"
echo "   - AppNexus/Xandr: GVL ID 32"
echo ""
echo "4. Test with real TCF consent strings from your CMP"
echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${GREEN}Testing Complete!${NC}"
echo ""
echo "For more information, see:"
echo "  - TCF-VENDOR-CONSENT-GUIDE.md"
echo "  - BIDDER-PARAMS-GUIDE.md"
echo ""
