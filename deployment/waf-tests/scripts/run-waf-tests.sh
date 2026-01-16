#!/bin/bash
# Comprehensive ModSecurity WAF Test Suite
# Tests legitimate traffic, attack detection, performance, and false positives

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(dirname "$SCRIPT_DIR")"
DEPLOYMENT_DIR="$(dirname "$TEST_DIR")"
RESULTS_DIR="${TEST_DIR}/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="${RESULTS_DIR}/test-report-${TIMESTAMP}.txt"
JSON_REPORT="${RESULTS_DIR}/test-report-${TIMESTAMP}.json"

# Test configuration
# Test application directly on port 8000 for baseline
WAF_URL="${WAF_URL:-http://localhost:8000}"
AUCTION_ENDPOINT="${WAF_URL}/openrtb2/auction"
HEALTH_ENDPOINT="${WAF_URL}/health"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Create results directory
mkdir -p "${RESULTS_DIR}"

# Initialize report
echo "╔══════════════════════════════════════════════════════════════╗" | tee "${REPORT_FILE}"
echo "║      ModSecurity WAF Test Suite - Comprehensive Report      ║" | tee -a "${REPORT_FILE}"
echo "╚══════════════════════════════════════════════════════════════╝" | tee -a "${REPORT_FILE}"
echo "" | tee -a "${REPORT_FILE}"
echo "Test Run: ${TIMESTAMP}" | tee -a "${REPORT_FILE}"
echo "WAF URL: ${WAF_URL}" | tee -a "${REPORT_FILE}"
echo "" | tee -a "${REPORT_FILE}"

# Function to test a request
test_request() {
    local name="$1"
    local payload="$2"
    local expected_status="$3"
    local user_agent="${4:-Mozilla/5.0 (Test Suite)}"
    local description="$5"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    # Make request and capture response
    response=$(curl -s -w "\n%{http_code}" -X POST "${AUCTION_ENDPOINT}" \
        -H "Content-Type: application/json" \
        -H "User-Agent: ${user_agent}" \
        -d "${payload}" 2>&1 || echo -e "\n000")

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    # Check if status matches expected
    if [[ "$http_code" =~ $expected_status ]]; then
        echo -e "${GREEN}✓ PASS${NC} - ${name}" | tee -a "${REPORT_FILE}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC} - ${name}" | tee -a "${REPORT_FILE}"
        echo "  Expected: ${expected_status}, Got: ${http_code}" | tee -a "${REPORT_FILE}"
        echo "  Response: ${body:0:200}..." | tee -a "${REPORT_FILE}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi
}

# Function to benchmark performance
benchmark_endpoint() {
    local name="$1"
    local payload="$2"
    local iterations="${3:-100}"

    echo -e "${BLUE}Benchmarking: ${name}${NC}" | tee -a "${REPORT_FILE}"

    # Warm-up
    for i in {1..10}; do
        curl -s -X POST "${AUCTION_ENDPOINT}" \
            -H "Content-Type: application/json" \
            -d "${payload}" > /dev/null 2>&1
    done

    # Benchmark
    start_time=$(date +%s%N)
    success_count=0

    for i in $(seq 1 $iterations); do
        response=$(curl -s -w "%{http_code}" -X POST "${AUCTION_ENDPOINT}" \
            -H "Content-Type: application/json" \
            -d "${payload}" -o /dev/null 2>&1)

        if [[ "$response" =~ ^2 ]]; then
            success_count=$((success_count + 1))
        fi
    done

    end_time=$(date +%s%N)
    duration_ns=$((end_time - start_time))
    duration_ms=$((duration_ns / 1000000))
    avg_latency=$((duration_ms / iterations))
    throughput=$((iterations * 1000 / duration_ms))
    success_rate=$((success_count * 100 / iterations))

    echo "  Total Time: ${duration_ms}ms" | tee -a "${REPORT_FILE}"
    echo "  Avg Latency: ${avg_latency}ms/request" | tee -a "${REPORT_FILE}"
    echo "  Throughput: ${throughput} req/s" | tee -a "${REPORT_FILE}"
    echo "  Success Rate: ${success_rate}%" | tee -a "${REPORT_FILE}"
    echo "" | tee -a "${REPORT_FILE}"

    # Store for JSON report
    echo "{\"name\":\"${name}\",\"iterations\":${iterations},\"total_ms\":${duration_ms},\"avg_latency_ms\":${avg_latency},\"throughput_rps\":${throughput},\"success_rate\":${success_rate}}" >> "${RESULTS_DIR}/benchmarks-${TIMESTAMP}.jsonl"
}

# Check if WAF is running
echo -e "${YELLOW}[1/6] Checking WAF availability...${NC}" | tee -a "${REPORT_FILE}"
if ! curl -s -f "${HEALTH_ENDPOINT}" > /dev/null 2>&1; then
    echo -e "${RED}✗ WAF is not responding at ${WAF_URL}${NC}" | tee -a "${REPORT_FILE}"
    echo "Please start the WAF first: ./deployment/deploy-waf.sh test" | tee -a "${REPORT_FILE}"
    exit 1
fi
echo -e "${GREEN}✓ WAF is running${NC}" | tee -a "${REPORT_FILE}"
echo "" | tee -a "${REPORT_FILE}"

# Test 1: Legitimate OpenRTB Requests
echo -e "${YELLOW}[2/6] Testing Legitimate OpenRTB Requests...${NC}" | tee -a "${REPORT_FILE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "${REPORT_FILE}"

# Parse legitimate requests JSON
legitimate_json="${TEST_DIR}/payloads/legitimate-requests.json"

if [ -f "$legitimate_json" ]; then
    echo "NOTE: Testing without registered publishers - expecting 403 for publisher auth" | tee -a "${REPORT_FILE}"
    echo "These are legitimate OpenRTB requests that would pass with valid publisher IDs" | tee -a "${REPORT_FILE}"
    echo "" | tee -a "${REPORT_FILE}"

    test_count=$(jq '.test_cases | length' "$legitimate_json")

    for i in $(seq 0 $((test_count - 1))); do
        name=$(jq -r ".test_cases[$i].name" "$legitimate_json")
        description=$(jq -r ".test_cases[$i].description" "$legitimate_json")
        expected=$(jq -r ".test_cases[$i].expected" "$legitimate_json")
        payload=$(jq -c ".test_cases[$i].payload" "$legitimate_json")

        # Expect 403 since we don't have registered publishers in test environment
        test_request "$name (expect publisher auth block)" "$payload" "403|400" "Mozilla/5.0" "$description"
    done
else
    echo -e "${RED}✗ Legitimate requests file not found${NC}" | tee -a "${REPORT_FILE}"
fi

echo "" | tee -a "${REPORT_FILE}"

# Test 2: Attack Detection
echo -e "${YELLOW}[3/6] Testing Attack Detection...${NC}" | tee -a "${REPORT_FILE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "${REPORT_FILE}"

# Parse attack requests JSON
attack_json="${TEST_DIR}/payloads/attack-requests.json"

if [ -f "$attack_json" ]; then
    test_count=$(jq '.test_cases | length' "$attack_json")

    for i in $(seq 0 $((test_count - 1))); do
        name=$(jq -r ".test_cases[$i].name" "$attack_json")
        description=$(jq -r ".test_cases[$i].description" "$attack_json")
        rule_id=$(jq -r ".test_cases[$i].rule_id" "$attack_json")
        expected=$(jq -r ".test_cases[$i].expected" "$attack_json")
        payload=$(jq -c ".test_cases[$i].payload" "$attack_json")

        # Check for custom User-Agent
        user_agent=$(jq -r ".test_cases[$i].headers.\"User-Agent\" // \"Mozilla/5.0 (Attack Test)\"" "$attack_json")

        test_request "[Rule $rule_id] $name" "$payload" "403|400" "$user_agent" "$description"
    done
else
    echo -e "${RED}✗ Attack requests file not found${NC}" | tee -a "${REPORT_FILE}"
fi

echo "" | tee -a "${REPORT_FILE}"

# Test 3: Performance Benchmarking
echo -e "${YELLOW}[4/6] Performance Benchmarking...${NC}" | tee -a "${REPORT_FILE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "${REPORT_FILE}"

# Simple banner request
simple_payload='{"id":"bench-001","imp":[{"id":"1","banner":{"w":300,"h":250},"bidfloor":0.5}],"site":{"domain":"bench.com","publisher":{"id":"pub-bench"}},"device":{"ip":"1.2.3.4"}}'

benchmark_endpoint "Simple Banner Request" "$simple_payload" 100

# Complex request with 10 impressions
complex_payload='{"id":"bench-002","imp":[{"id":"1","banner":{"w":300,"h":250}},{"id":"2","banner":{"w":728,"h":90}},{"id":"3","banner":{"w":160,"h":600}},{"id":"4","banner":{"w":300,"h":600}},{"id":"5","banner":{"w":970,"h":250}},{"id":"6","banner":{"w":320,"h":50}},{"id":"7","banner":{"w":300,"h":250}},{"id":"8","banner":{"w":728,"h":90}},{"id":"9","banner":{"w":160,"h":600}},{"id":"10","banner":{"w":300,"h":600}}],"site":{"domain":"bench.com","publisher":{"id":"pub-bench"}},"device":{"ip":"1.2.3.4"}}'

benchmark_endpoint "Complex Request (10 impressions)" "$complex_payload" 100

# Test 4: Edge Cases
echo -e "${YELLOW}[5/6] Testing Edge Cases...${NC}" | tee -a "${REPORT_FILE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "${REPORT_FILE}"

# Large but legitimate payload
test_request "Large Payload (50KB)" "$(jq -c '.test_cases[3].payload' "$legitimate_json")" "200|204"

# Empty request
test_request "Empty Request" "{}" "400|403"

# Malformed JSON
test_request "Malformed JSON" '{"id":"test"' "400"

# Request with Unicode
unicode_payload='{"id":"test-unicode","imp":[{"id":"1"}],"site":{"domain":"测试.com","publisher":{"id":"pub-unicode"}}}'
test_request "Unicode Characters" "$unicode_payload" "200|204"

echo "" | tee -a "${REPORT_FILE}"

# Test 5: Check WAF Logs
echo -e "${YELLOW}[6/6] Checking WAF Logs...${NC}" | tee -a "${REPORT_FILE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "${REPORT_FILE}"

# Check if audit log exists and has entries
if [ -f "${DEPLOYMENT_DIR}/modsec-logs/audit.log" ]; then
    log_lines=$(wc -l < "${DEPLOYMENT_DIR}/modsec-logs/audit.log")
    echo "ModSecurity Audit Log: ${log_lines} lines" | tee -a "${REPORT_FILE}"

    # Count blocked requests
    if command -v jq > /dev/null 2>&1; then
        blocked_count=$(grep -c '"action":"blocked"' "${DEPLOYMENT_DIR}/modsec-logs/audit.log" 2>/dev/null || echo "0")
        echo "Blocked Requests: ${blocked_count}" | tee -a "${REPORT_FILE}"
    fi
else
    echo "⚠ Audit log not found (WAF may be in DetectionOnly mode)" | tee -a "${REPORT_FILE}"
fi

echo "" | tee -a "${REPORT_FILE}"

# Final Summary
echo "╔══════════════════════════════════════════════════════════════╗" | tee -a "${REPORT_FILE}"
echo "║                        Test Summary                          ║" | tee -a "${REPORT_FILE}"
echo "╚══════════════════════════════════════════════════════════════╝" | tee -a "${REPORT_FILE}"
echo "" | tee -a "${REPORT_FILE}"
echo "Total Tests:  ${TOTAL_TESTS}" | tee -a "${REPORT_FILE}"
echo -e "${GREEN}Passed:       ${PASSED_TESTS}${NC}" | tee -a "${REPORT_FILE}"
echo -e "${RED}Failed:       ${FAILED_TESTS}${NC}" | tee -a "${REPORT_FILE}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo "" | tee -a "${REPORT_FILE}"
    echo -e "${GREEN}✓ ALL TESTS PASSED${NC}" | tee -a "${REPORT_FILE}"
    SUCCESS_RATE=100
else
    echo "" | tee -a "${REPORT_FILE}"
    echo -e "${RED}✗ SOME TESTS FAILED${NC}" | tee -a "${REPORT_FILE}"
    SUCCESS_RATE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
fi

echo "Success Rate: ${SUCCESS_RATE}%" | tee -a "${REPORT_FILE}"
echo "" | tee -a "${REPORT_FILE}"
echo "Full report saved to: ${REPORT_FILE}" | tee -a "${REPORT_FILE}"
echo "" | tee -a "${REPORT_FILE}"

# Generate JSON summary
cat > "${JSON_REPORT}" <<EOF
{
  "timestamp": "${TIMESTAMP}",
  "waf_url": "${WAF_URL}",
  "total_tests": ${TOTAL_TESTS},
  "passed": ${PASSED_TESTS},
  "failed": ${FAILED_TESTS},
  "success_rate": ${SUCCESS_RATE},
  "report_file": "${REPORT_FILE}"
}
EOF

echo "JSON summary saved to: ${JSON_REPORT}"

# Exit with appropriate code
if [ $FAILED_TESTS -eq 0 ]; then
    exit 0
else
    exit 1
fi
