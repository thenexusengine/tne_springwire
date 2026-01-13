#!/bin/bash
# Test Geographic-Based Consent Filtering
# This script demonstrates how Catalyst validates consent based on user geographic location

CATALYST_URL="${CATALYST_URL:-https://catalyst.springwire.ai}"
PUBLISHER_ID="${PUBLISHER_ID:-pub123}"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Testing Geographic-Based Consent Filtering${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo ""

# Test 1: EU User with GDPR Consent
echo -e "${YELLOW}Test 1: EU User (Germany) with Valid GDPR Consent${NC}"
echo "Testing EU user with TCF consent string including Rubicon (GVL ID 52)"
echo ""

REQUEST_1=$(cat <<EOF
{
  "id": "test-geo-eu-$(date +%s)",
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
  "device": {
    "geo": {
      "country": "DEU",
      "city": "Berlin",
      "lat": 52.52,
      "lon": 13.405
    },
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  },
  "regs": {
    "gdpr": 1
  },
  "user": {
    "id": "test-user-eu-001",
    "consent": "COtybn4PA_zT4KjACBENAPCIAEBAAECAAIAAAARB"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
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
    echo -e "${GREEN}✓ Request accepted (EU geo with GDPR consent)${NC}"
    BID_COUNT=$(echo "$RESPONSE_BODY_1" | jq -r '.seatbid | length' 2>/dev/null || echo "0")
    if [ "$BID_COUNT" != "0" ] && [ "$BID_COUNT" != "null" ]; then
        echo -e "${GREEN}✓ Rubicon participated (has GDPR consent)${NC}"
    else
        echo -e "${YELLOW}⚠ No bids returned (may be test environment or no consent)${NC}"
    fi
else
    echo -e "${RED}✗ Request failed${NC}"
    echo "$RESPONSE_BODY_1" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_1"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Test 2: California User with CCPA Opt-Out
echo -e "${YELLOW}Test 2: US-CA User with CCPA Opt-Out (1YYN)${NC}"
echo "Testing California user who has opted out of data sale"
echo ""

REQUEST_2=$(cat <<EOF
{
  "id": "test-geo-ca-optout-$(date +%s)",
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
  "device": {
    "geo": {
      "country": "USA",
      "region": "CA",
      "city": "Los Angeles",
      "lat": 34.0522,
      "lon": -118.2437
    },
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  },
  "regs": {
    "us_privacy": "1YYN"
  },
  "user": {
    "id": "test-user-ca-002"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
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
        echo -e "${GREEN}✓ All bidders skipped (user opted out of CCPA)${NC}"
    else
        echo -e "${YELLOW}⚠ Some bids returned (unexpected - user opted out)${NC}"
    fi
else
    echo -e "${RED}✗ Request failed${NC}"
    echo "$RESPONSE_BODY_2" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_2"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Test 3: California User WITHOUT Opt-Out
echo -e "${YELLOW}Test 3: US-CA User WITHOUT Opt-Out (1YNN)${NC}"
echo "Testing California user who has NOT opted out"
echo ""

REQUEST_3=$(cat <<EOF
{
  "id": "test-geo-ca-no-optout-$(date +%s)",
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
  "device": {
    "geo": {
      "country": "USA",
      "region": "CA",
      "city": "San Francisco",
      "lat": 37.7749,
      "lon": -122.4194
    },
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  },
  "regs": {
    "us_privacy": "1YNN"
  },
  "user": {
    "id": "test-user-ca-003"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
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
        echo -e "${GREEN}✓ Bidders participated (user did NOT opt out)${NC}"
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

# Test 4: Non-Regulated Region (Japan)
echo -e "${YELLOW}Test 4: Non-Regulated Region (Japan)${NC}"
echo "Testing user from region with no specific privacy law"
echo ""

REQUEST_4=$(cat <<EOF
{
  "id": "test-geo-japan-$(date +%s)",
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
  "device": {
    "geo": {
      "country": "JPN",
      "city": "Tokyo",
      "lat": 35.6762,
      "lon": 139.6503
    },
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  },
  "user": {
    "id": "test-user-japan-004"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
  }
}
EOF
)

RESPONSE_4=$(curl -s -w "\n%{http_code}" -X POST "$CATALYST_URL/openrtb2/auction" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_4")

RESPONSE_BODY_4=$(echo "$RESPONSE_4" | head -n -1)
HTTP_CODE_4=$(echo "$RESPONSE_4" | tail -n 1)

echo -e "${BLUE}Response Status: $HTTP_CODE_4${NC}"
if [ "$HTTP_CODE_4" = "200" ]; then
    echo -e "${GREEN}✓ Request accepted${NC}"
    BID_COUNT=$(echo "$RESPONSE_BODY_4" | jq -r '.seatbid | length' 2>/dev/null || echo "0")
    if [ "$BID_COUNT" != "0" ] && [ "$BID_COUNT" != "null" ]; then
        echo -e "${GREEN}✓ Bidders participated (no privacy law applies)${NC}"
    else
        echo -e "${YELLOW}⚠ No bids returned (may be test environment)${NC}"
    fi
else
    echo -e "${RED}✗ Request failed${NC}"
    echo "$RESPONSE_BODY_4" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_4"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Test 5: EU User WITHOUT GDPR Flag (Should Fail)
echo -e "${YELLOW}Test 5: EU User WITHOUT GDPR Flag (Expected to Fail)${NC}"
echo "Testing EU user without regs.gdpr=1 (should be blocked)"
echo ""

REQUEST_5=$(cat <<EOF
{
  "id": "test-geo-eu-no-gdpr-$(date +%s)",
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
  "device": {
    "geo": {
      "country": "FRA",
      "city": "Paris"
    },
    "ua": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "ip": "203.0.113.42"
  },
  "user": {
    "id": "test-user-eu-005"
  },
  "site": {
    "domain": "test.example.com",
    "page": "https://test.example.com/",
    "publisher": {
      "id": "$PUBLISHER_ID"
    }
  }
}
EOF
)

RESPONSE_5=$(curl -s -w "\n%{http_code}" -X POST "$CATALYST_URL/openrtb2/auction" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_5")

RESPONSE_BODY_5=$(echo "$RESPONSE_5" | head -n -1)
HTTP_CODE_5=$(echo "$RESPONSE_5" | tail -n 1)

echo -e "${BLUE}Response Status: $HTTP_CODE_5${NC}"
if [ "$HTTP_CODE_5" = "400" ]; then
    echo -e "${GREEN}✓ Request correctly blocked (EU user without GDPR consent)${NC}"
    ERROR_MSG=$(echo "$RESPONSE_BODY_5" | jq -r '.reason' 2>/dev/null || echo "")
    if [ "$ERROR_MSG" != "" ]; then
        echo -e "${BLUE}Reason: $ERROR_MSG${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Unexpected response code (expected 400)${NC}"
    echo "$RESPONSE_BODY_5" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY_5"
fi

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""

# Summary
echo -e "${YELLOW}Test Summary:${NC}"
echo ""
echo "1. EU (Germany) with GDPR → Bidders with GVL ID in consent participate"
echo "2. US-CA with opt-out (1YYN) → All bidders skipped"
echo "3. US-CA without opt-out (1YNN) → Bidders participate"
echo "4. Japan (no law) → Bidders participate normally"
echo "5. EU without GDPR flag → Request blocked (400 error)"
echo ""
echo -e "${YELLOW}Verification Steps:${NC}"
echo ""
echo "1. Check Catalyst logs for geo-based filtering:"
echo "   docker compose logs -f catalyst | grep -i 'geographic\\|regulation'"
echo ""
echo "2. Look for log messages like:"
echo "   - 'Detected regulation regulation=GDPR country=DEU'"
echo "   - 'Skipping bidder - no consent for user's geographic location'"
echo "   - 'regulation=CCPA country=USA region=CA'"
echo ""
echo "3. Verify country codes:"
echo "   - EU: DEU (Germany), FRA (France), GBR (UK)"
echo "   - US: USA with region codes CA, VA, CO, CT, UT"
echo "   - Others: JPN (Japan), BRA (Brazil), CAN (Canada)"
echo ""
echo "4. Test with real geo data from your users"
echo ""
echo -e "${BLUE}════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${GREEN}Testing Complete!${NC}"
echo ""
echo "For more information, see:"
echo "  - GEO-CONSENT-GUIDE.md"
echo "  - TCF-VENDOR-CONSENT-GUIDE.md"
echo "  - BIDDER-PARAMS-GUIDE.md"
echo ""
