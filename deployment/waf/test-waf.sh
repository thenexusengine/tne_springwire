#!/bin/bash
# =============================================================================
# WAF Test Script for Catalyst
# =============================================================================
# Tests ModSecurity rules against common attack patterns.
# Run against a test/staging environment, NOT production!
# =============================================================================

set -e

# Configuration
BASE_URL="${WAF_TEST_URL:-http://localhost:8080}"
AUCTION_URL="$BASE_URL/openrtb2/auction"
HEALTH_URL="$BASE_URL/health"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0
WARNINGS=0

# Test function
test_request() {
    local name="$1"
    local expected_status="$2"
    local method="$3"
    local url="$4"
    local data="$5"
    local headers="$6"

    echo -n "Testing: $name... "

    # Build curl command
    cmd="curl -s -o /dev/null -w '%{http_code}' -X $method"

    if [ -n "$headers" ]; then
        cmd="$cmd $headers"
    fi

    if [ -n "$data" ]; then
        cmd="$cmd -d '$data'"
    fi

    cmd="$cmd '$url'"

    # Execute and get status
    status=$(eval $cmd 2>/dev/null || echo "000")

    if [ "$status" == "$expected_status" ]; then
        echo -e "${GREEN}PASS${NC} (got $status)"
        ((PASSED++))
    else
        echo -e "${RED}FAIL${NC} (expected $expected_status, got $status)"
        ((FAILED++))
    fi
}

echo "=============================================="
echo "Catalyst WAF Test Suite"
echo "Target: $BASE_URL"
echo "=============================================="
echo ""

# =============================================================================
# Health Check (should always pass)
# =============================================================================
echo "--- Health Check Tests ---"
test_request "Health endpoint accessible" "200" "GET" "$HEALTH_URL"

# =============================================================================
# Request Validation Tests
# =============================================================================
echo ""
echo "--- Request Validation Tests ---"

# Missing Content-Type
test_request "POST without Content-Type (should block)" "400" "POST" "$AUCTION_URL" \
    '{"id":"test"}' ""

# Wrong Content-Type
test_request "POST with wrong Content-Type (should block)" "415" "POST" "$AUCTION_URL" \
    '{"id":"test"}' "-H 'Content-Type: text/plain'"

# Empty body
test_request "POST with empty body (should block)" "400" "POST" "$AUCTION_URL" \
    "" "-H 'Content-Type: application/json'"

# =============================================================================
# OpenRTB Validation Tests
# =============================================================================
echo ""
echo "--- OpenRTB Validation Tests ---"

# Missing id field
test_request "Missing id field (should block)" "400" "POST" "$AUCTION_URL" \
    '{"imp":[{"id":"1"}]}' "-H 'Content-Type: application/json'"

# Missing imp field
test_request "Missing imp field (should block)" "400" "POST" "$AUCTION_URL" \
    '{"id":"test123"}' "-H 'Content-Type: application/json'"

# Valid minimal request (should pass)
test_request "Valid minimal request (should pass)" "200" "POST" "$AUCTION_URL" \
    '{"id":"test123","imp":[{"id":"1","banner":{"w":300,"h":250}}],"site":{"domain":"example.com"}}' \
    "-H 'Content-Type: application/json'"

# Both site and app (should block)
test_request "Both site and app (should block)" "400" "POST" "$AUCTION_URL" \
    '{"id":"test123","imp":[{"id":"1"}],"site":{"domain":"a.com"},"app":{"bundle":"com.a"}}' \
    "-H 'Content-Type: application/json'"

# =============================================================================
# Bid Manipulation Tests
# =============================================================================
echo ""
echo "--- Bid Manipulation Tests ---"

# Extremely high bidfloor
test_request "Bidfloor > 1000 (should block)" "400" "POST" "$AUCTION_URL" \
    '{"id":"test","imp":[{"id":"1","bidfloor":99999}],"site":{"domain":"a.com"}}' \
    "-H 'Content-Type: application/json'"

# Negative bidfloor
test_request "Negative bidfloor (should block)" "400" "POST" "$AUCTION_URL" \
    '{"id":"test","imp":[{"id":"1","bidfloor":-1}],"site":{"domain":"a.com"}}' \
    "-H 'Content-Type: application/json'"

# Extremely high tmax
test_request "tmax > 30000 (should block)" "400" "POST" "$AUCTION_URL" \
    '{"id":"test","imp":[{"id":"1"}],"site":{"domain":"a.com"},"tmax":999999}' \
    "-H 'Content-Type: application/json'"

# =============================================================================
# Bot Detection Tests
# =============================================================================
echo ""
echo "--- Bot Detection Tests ---"

# Scanner user agent
test_request "Nikto user agent (should block)" "403" "POST" "$AUCTION_URL" \
    '{"id":"test","imp":[{"id":"1"}],"site":{"domain":"a.com"}}' \
    "-H 'Content-Type: application/json' -H 'User-Agent: nikto/2.1'"

# SQLMap user agent
test_request "SQLMap user agent (should block)" "403" "POST" "$AUCTION_URL" \
    '{"id":"test","imp":[{"id":"1"}],"site":{"domain":"a.com"}}' \
    "-H 'Content-Type: application/json' -H 'User-Agent: sqlmap/1.0'"

# Missing user agent on auction
test_request "Missing User-Agent (should block)" "403" "POST" "$AUCTION_URL" \
    '{"id":"test","imp":[{"id":"1"}],"site":{"domain":"a.com"}}' \
    "-H 'Content-Type: application/json' -A ''"

# =============================================================================
# OWASP CRS Tests
# =============================================================================
echo ""
echo "--- OWASP CRS Tests ---"

# SQL injection attempt
test_request "SQL injection in request (should block)" "403" "POST" "$AUCTION_URL" \
    '{"id":"test; DROP TABLE users;--","imp":[{"id":"1"}],"site":{"domain":"a.com"}}' \
    "-H 'Content-Type: application/json'"

# Path traversal
test_request "Path traversal (should block)" "403" "GET" "$BASE_URL/../../../etc/passwd"

# XSS attempt in parameters
test_request "XSS in query param (should block)" "403" "GET" "$BASE_URL/info/bidders?q=<script>alert(1)</script>"

# =============================================================================
# Summary
# =============================================================================
echo ""
echo "=============================================="
echo "Test Summary"
echo "=============================================="
echo -e "Passed:   ${GREEN}$PASSED${NC}"
echo -e "Failed:   ${RED}$FAILED${NC}"
echo -e "Warnings: ${YELLOW}$WARNINGS${NC}"
echo ""

if [ $FAILED -gt 0 ]; then
    echo -e "${RED}Some tests failed. Review WAF configuration.${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
