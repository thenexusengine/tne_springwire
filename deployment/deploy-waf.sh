#!/bin/bash
# Deploy ModSecurity WAF for Catalyst Auction Server
# Usage: ./deploy-waf.sh [test|production]

set -e

MODE="${1:-test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "╔═══════════════════════════════════════════════════════════╗"
echo "║   Catalyst Auction Server - ModSecurity WAF Deployment    ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check prerequisites
echo -e "${YELLOW}[1/7] Checking prerequisites...${NC}"
command -v docker >/dev/null 2>&1 || { echo -e "${RED}Error: docker is required but not installed.${NC}" >&2; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo -e "${RED}Error: docker-compose is required but not installed.${NC}" >&2; exit 1; }
echo -e "${GREEN}✓ Prerequisites met${NC}"
echo ""

# Create log directories
echo -e "${YELLOW}[2/7] Creating log directories...${NC}"
mkdir -p "${SCRIPT_DIR}/nginx-logs"
mkdir -p "${SCRIPT_DIR}/modsec-logs"
chmod 755 "${SCRIPT_DIR}/nginx-logs"
chmod 755 "${SCRIPT_DIR}/modsec-logs"
echo -e "${GREEN}✓ Log directories created${NC}"
echo ""

# Set WAF mode based on argument
echo -e "${YELLOW}[3/7] Configuring WAF mode: ${MODE}...${NC}"
if [ "$MODE" = "test" ]; then
    export MODSEC_RULE_ENGINE="DetectionOnly"
    export PARANOIA=1
    export ANOMALY_INBOUND=10
    echo -e "${YELLOW}  Mode: DETECTION ONLY (logs but doesn't block)${NC}"
    echo -e "${YELLOW}  Paranoia Level: 1 (basic protection)${NC}"
    echo -e "${YELLOW}  Anomaly Threshold: 10 (permissive)${NC}"
elif [ "$MODE" = "production" ]; then
    export MODSEC_RULE_ENGINE="On"
    export PARANOIA=2
    export ANOMALY_INBOUND=5
    echo -e "${GREEN}  Mode: BLOCKING (logs AND blocks attacks)${NC}"
    echo -e "${GREEN}  Paranoia Level: 2 (moderate protection)${NC}"
    echo -e "${GREEN}  Anomaly Threshold: 5 (balanced)${NC}"
else
    echo -e "${RED}Error: Invalid mode. Use 'test' or 'production'${NC}"
    exit 1
fi
echo ""

# Build ModSecurity image
echo -e "${YELLOW}[4/7] Building ModSecurity + Nginx image...${NC}"
docker build -t catalyst-nginx-waf:latest -f "${SCRIPT_DIR}/nginx-modsecurity.Dockerfile" "${SCRIPT_DIR}"
echo -e "${GREEN}✓ Image built successfully${NC}"
echo ""

# Stop existing nginx container
echo -e "${YELLOW}[5/7] Stopping existing nginx container...${NC}"
docker-compose -f "${SCRIPT_DIR}/docker-compose-modsecurity.yml" stop nginx 2>/dev/null || true
docker rm -f catalyst-nginx-waf 2>/dev/null || true
echo -e "${GREEN}✓ Cleanup complete${NC}"
echo ""

# Start WAF-protected stack
echo -e "${YELLOW}[6/7] Starting WAF-protected stack...${NC}"
docker-compose -f "${SCRIPT_DIR}/docker-compose-modsecurity.yml" up -d nginx
echo -e "${GREEN}✓ Stack started${NC}"
echo ""

# Wait for health check
echo -e "${YELLOW}[7/7] Waiting for health check...${NC}"
sleep 5
RETRIES=0
MAX_RETRIES=30
while [ $RETRIES -lt $MAX_RETRIES ]; do
    if docker exec catalyst-nginx-waf wget --no-verbose --tries=1 --spider http://localhost:80/health 2>&1 | grep -q "200 OK"; then
        echo -e "${GREEN}✓ WAF is healthy and running!${NC}"
        break
    fi
    RETRIES=$((RETRIES + 1))
    echo -n "."
    sleep 2
done

if [ $RETRIES -eq $MAX_RETRIES ]; then
    echo -e "${RED}✗ Health check failed${NC}"
    echo "Checking logs..."
    docker logs --tail 50 catalyst-nginx-waf
    exit 1
fi
echo ""

# Display summary
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║                   Deployment Summary                      ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo ""
echo -e "WAF Mode:           ${GREEN}${MODSEC_RULE_ENGINE}${NC}"
echo -e "Paranoia Level:     ${PARANOIA}"
echo -e "Anomaly Threshold:  ${ANOMALY_INBOUND}"
echo ""
echo "Endpoints:"
echo "  - HTTP:  http://catalyst.springwire.ai"
echo "  - HTTPS: https://catalyst.springwire.ai"
echo "  - Health: https://catalyst.springwire.ai/health"
echo ""
echo "Logs:"
echo "  - Nginx Access:  ${SCRIPT_DIR}/nginx-logs/access.log"
echo "  - Nginx Error:   ${SCRIPT_DIR}/nginx-logs/error.log"
echo "  - ModSec Audit:  ${SCRIPT_DIR}/modsec-logs/audit.log"
echo ""
echo "Commands:"
echo "  - View logs:     docker logs -f catalyst-nginx-waf"
echo "  - ModSec audit:  tail -f ${SCRIPT_DIR}/modsec-logs/audit.log | jq ."
echo "  - Test attack:   curl -X POST https://catalyst.springwire.ai/openrtb2/auction -d '{\"id\":\"' OR 1=1--\"}'"
echo ""

if [ "$MODE" = "test" ]; then
    echo -e "${YELLOW}⚠️  WARNING: Running in DETECTION mode (attacks are logged but NOT blocked)${NC}"
    echo -e "${YELLOW}   Monitor logs for 1-2 weeks, then deploy in production mode:${NC}"
    echo -e "${YELLOW}   $ ./deploy-waf.sh production${NC}"
else
    echo -e "${GREEN}✓ Running in PRODUCTION mode (attacks are blocked)${NC}"
fi
echo ""
echo "For more information, see: deployment/WAF-README.md"
echo ""
