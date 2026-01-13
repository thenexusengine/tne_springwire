#!/bin/bash
# Test Rubicon bidder parameters
# Usage: ./test-rubicon-params.sh [account_id] [site_id] [zone_id]

CATALYST_URL="${CATALYST_URL:-https://catalyst.springwire.ai}"
ACCOUNT_ID="${1:-26298}"
SITE_ID="${2:-556630}"
ZONE_ID="${3:-3767186}"
PUBLISHER_ID="${PUBLISHER_ID:-pub123}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Testing Rubicon Bidder Parameters${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo ""
echo -e "${YELLOW}Configuration:${NC}"
echo -e "  Catalyst URL:   $CATALYST_URL"
echo -e "  Publisher ID:   $PUBLISHER_ID"
echo -e "  Account ID:     $ACCOUNT_ID"
echo -e "  Site ID:        $SITE_ID"
echo -e "  Zone ID:        $ZONE_ID"
echo ""

# Create test bid request
BID_REQUEST=$(cat <<EOF
{
  "id": "test-$(date +%s)",
  "imp": [{
    "id": "1",
    "banner": {
      "format": [
        {"w": 300, "h": 250},
        {"w": 728, "h": 90}
      ]
    },
    "bidfloor": 0.50,
    "bidfloorcur": "USD",
    "ext": {
      "rubicon": {
        "accountId": $ACCOUNT_ID,
        "siteId": $SITE_ID,
        "zoneId": $ZONE_ID,
        "inventory": {
          "rating": "test",
          "prodtype": "test"
        }
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
    "id": "test-user-123"
  },
  "at": 1,
  "tmax": 1000,
  "cur": ["USD"]
}
EOF
)

echo -e "${YELLOW}Sending test bid request...${NC}"
echo ""

# Send request and capture response
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$CATALYST_URL/openrtb2/auction" \
  -H "Content-Type: application/json" \
  -d "$BID_REQUEST")

# Split response body and status code
RESPONSE_BODY=$(echo "$RESPONSE" | head -n -1)
HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)

echo -e "${BLUE}Response Status: $HTTP_CODE${NC}"
echo ""

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ Request successful${NC}"
    echo ""
    echo -e "${YELLOW}Response:${NC}"
    echo "$RESPONSE_BODY" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY"

    # Check for bids
    BID_COUNT=$(echo "$RESPONSE_BODY" | jq -r '.seatbid | length' 2>/dev/null || echo "0")
    if [ "$BID_COUNT" != "0" ] && [ "$BID_COUNT" != "null" ]; then
        echo ""
        echo -e "${GREEN}✓ Received $BID_COUNT bid(s) from Rubicon${NC}"
        echo "$RESPONSE_BODY" | jq -r '.seatbid[] | .bid[] | "  Bid ID: \(.id), Price: \(.price) \(.cur // "USD")"' 2>/dev/null
    else
        echo ""
        echo -e "${YELLOW}⚠ No bids returned (this may be normal for test requests)${NC}"
    fi
elif [ "$HTTP_CODE" = "400" ]; then
    echo -e "${RED}✗ Bad Request - Check your parameters${NC}"
    echo ""
    echo "$RESPONSE_BODY" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY"
elif [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then
    echo -e "${RED}✗ Authentication Error${NC}"
    echo -e "${YELLOW}Make sure publisher '$PUBLISHER_ID' is registered${NC}"
    echo ""
    echo "To add publisher:"
    echo "  cd deployment"
    echo "  ./manage-publishers.sh add $PUBLISHER_ID 'test.example.com'"
elif [ "$HTTP_CODE" = "503" ]; then
    echo -e "${RED}✗ Service Unavailable${NC}"
    echo -e "${YELLOW}Check if Catalyst is running: docker compose ps${NC}"
else
    echo -e "${RED}✗ Unexpected status code: $HTTP_CODE${NC}"
    echo ""
    echo "$RESPONSE_BODY"
fi

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo ""

# Additional verification
echo -e "${YELLOW}Verification Steps:${NC}"
echo ""
echo "1. Check Catalyst logs for request:"
echo "   docker compose logs -f catalyst | grep -i rubicon"
echo ""
echo "2. Verify publisher is registered:"
echo "   cd deployment && ./manage-publishers.sh check $PUBLISHER_ID"
echo ""
echo "3. Test with different parameters:"
echo "   $0 <account_id> <site_id> <zone_id>"
echo ""
echo "4. Check Rubicon dashboard for bid requests:"
echo "   - Account ID: $ACCOUNT_ID"
echo "   - Site ID: $SITE_ID"
echo "   - Zone ID: $ZONE_ID"
echo ""
