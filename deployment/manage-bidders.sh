#!/bin/bash
# Bidder Management Script for Catalyst (PostgreSQL)
# Usage: ./manage-bidders.sh <command> [options]

POSTGRES_CONTAINER="catalyst-postgres"
DB_NAME="${DB_NAME:-catalyst}"
DB_USER="${DB_USER:-catalyst}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if PostgreSQL is running
check_postgres() {
    if ! docker ps | grep -q $POSTGRES_CONTAINER; then
        echo -e "${RED}Error: PostgreSQL container '$POSTGRES_CONTAINER' not running${NC}"
        echo -e "${YELLOW}Start with: docker compose up -d${NC}"
        exit 1
    fi
}

# Execute SQL query
exec_sql() {
    local query="$1"
    docker exec $POSTGRES_CONTAINER psql -U $DB_USER -d $DB_NAME -t -A -c "$query" 2>/dev/null
}

# List all bidders
list_bidders() {
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}Registered Bidders in Catalyst${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"

    local query="SELECT bidder_code, bidder_name, endpoint_url, enabled, status FROM bidders ORDER BY bidder_code;"
    local output=$(exec_sql "$query")

    if [ -z "$output" ]; then
        echo -e "${YELLOW}No bidders registered yet${NC}"
        echo ""
        echo "Add your first bidder:"
        echo "  $0 add rubicon 'Rubicon/Magnite' 'https://prebid-server.rubiconproject.com/openrtb2/auction'"
        return
    fi

    echo ""
    printf "%-15s %-25s %-50s %-10s %s\n" "BIDDER CODE" "NAME" "ENDPOINT" "ENABLED" "STATUS"
    echo "────────────────────────────────────────────────────────────────────────────────────────────────────"

    echo "$output" | while IFS='|' read -r code name endpoint enabled status; do
        # Truncate endpoint if too long
        if [ ${#endpoint} -gt 47 ]; then
            endpoint="${endpoint:0:44}..."
        fi
        printf "%-15s %-25s %-50s %-10s %s\n" "$code" "$name" "$endpoint" "$enabled" "$status"
    done

    local count=$(echo "$output" | wc -l | tr -d ' ')
    echo ""
    echo -e "${GREEN}Total: $count bidder(s)${NC}"
}

# Add bidder
add_bidder() {
    local code=$1
    local name=$2
    local endpoint=$3
    local timeout=${4:-1000}
    local gvl_id=${5:-NULL}

    if [ -z "$code" ] || [ -z "$name" ] || [ -z "$endpoint" ]; then
        echo -e "${RED}Error: Missing arguments${NC}"
        echo ""
        echo "Usage: $0 add <bidder_code> <name> <endpoint_url> [timeout_ms] [gvl_vendor_id]"
        echo ""
        echo "Examples:"
        echo "  $0 add rubicon 'Rubicon/Magnite' 'https://prebid-server.rubiconproject.com/openrtb2/auction'"
        echo "  $0 add pubmatic 'PubMatic' 'https://hbopenbid.pubmatic.com/translator?source=prebid-server' 1500 76"
        echo "  $0 add custom 'Custom Bidder' 'https://custom.bidder.com/openrtb2' 2000"
        exit 1
    fi

    # Check if already exists
    local existing=$(exec_sql "SELECT bidder_code FROM bidders WHERE bidder_code='$code';")
    if [ -n "$existing" ]; then
        echo -e "${YELLOW}Warning: Bidder '$code' already exists${NC}"
        echo -e "${YELLOW}Use 'update' command to change configuration${NC}"
        exit 1
    fi

    # Insert bidder
    local query="INSERT INTO bidders (bidder_code, bidder_name, endpoint_url, timeout_ms, gvl_vendor_id, status) VALUES ('$code', '$name', '$endpoint', $timeout, $gvl_id, 'active');"
    exec_sql "$query"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Successfully added bidder${NC}"
        echo ""
        echo -e "  Bidder Code: ${BLUE}$code${NC}"
        echo -e "  Name: ${BLUE}$name${NC}"
        echo -e "  Endpoint: ${BLUE}$endpoint${NC}"
        echo -e "  Timeout: ${BLUE}${timeout}ms${NC}"
        if [ "$gvl_id" != "NULL" ]; then
            echo -e "  GVL Vendor ID: ${BLUE}$gvl_id${NC}"
        fi
        echo ""
        echo -e "${YELLOW}Next steps:${NC}"
        echo "  1. Configure publisher bidder_params for this bidder"
        echo "  2. Test with: ./manage-bidders.sh check $code"
    else
        echo -e "${RED}✗ Failed to add bidder${NC}"
        exit 1
    fi
}

# Remove bidder
remove_bidder() {
    local code=$1

    if [ -z "$code" ]; then
        echo -e "${RED}Error: Missing bidder code${NC}"
        echo ""
        echo "Usage: $0 remove <bidder_code>"
        echo ""
        echo "Example:"
        echo "  $0 remove rubicon"
        exit 1
    fi

    # Check if exists
    local existing=$(exec_sql "SELECT bidder_code, bidder_name FROM bidders WHERE bidder_code='$code';")
    if [ -z "$existing" ]; then
        echo -e "${YELLOW}Bidder '$code' not found${NC}"
        exit 1
    fi

    # Soft delete (archive)
    local query="UPDATE bidders SET status='archived', enabled=false WHERE bidder_code='$code';"
    exec_sql "$query"

    echo -e "${GREEN}✓ Successfully archived bidder: $code${NC}"
    echo -e "${YELLOW}Previous details: $existing${NC}"
}

# Check bidder
check_bidder() {
    local code=$1

    if [ -z "$code" ]; then
        echo -e "${RED}Error: Missing bidder code${NC}"
        echo ""
        echo "Usage: $0 check <bidder_code>"
        echo ""
        echo "Example:"
        echo "  $0 check rubicon"
        exit 1
    fi

    local query="SELECT bidder_code, bidder_name, endpoint_url, timeout_ms, enabled, status, supports_banner, supports_video, supports_native, gvl_vendor_id, description, created_at FROM bidders WHERE bidder_code='$code';"
    local output=$(exec_sql "$query")

    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    if [ -z "$output" ]; then
        echo -e "${YELLOW}Bidder '$code' NOT REGISTERED${NC}"
        echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
        echo ""
        echo "Add this bidder:"
        echo "  $0 add $code 'Bidder Name' 'https://bidder.example.com/openrtb2'"
    else
        IFS='|' read -r code name endpoint timeout enabled status banner video native gvl desc created <<< "$output"
        echo -e "${GREEN}Bidder: $code${NC}"
        echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
        echo -e "  Name: ${GREEN}$name${NC}"
        echo -e "  Status: ${GREEN}$status${NC}"
        echo -e "  Enabled: ${GREEN}$enabled${NC}"
        echo -e "  Endpoint: ${BLUE}$endpoint${NC}"
        echo -e "  Timeout: ${BLUE}${timeout}ms${NC}"

        # Capabilities
        echo ""
        echo "Supported Formats:"
        [ "$banner" = "t" ] && echo -e "  • ${GREEN}Banner${NC}" || echo -e "  • ${RED}Banner (disabled)${NC}"
        [ "$video" = "t" ] && echo -e "  • ${GREEN}Video${NC}" || echo -e "  • ${RED}Video (disabled)${NC}"
        [ "$native" = "t" ] && echo -e "  • ${GREEN}Native${NC}" || echo -e "  • ${RED}Native (disabled)${NC}"

        # GVL
        if [ "$gvl" != "" ]; then
            echo ""
            echo -e "  GVL Vendor ID: ${BLUE}$gvl${NC} (GDPR consent)"
        fi

        # Description
        if [ "$desc" != "" ]; then
            echo ""
            echo -e "  Description: ${BLUE}$desc${NC}"
        fi

        echo ""
        echo -e "  Created: ${BLUE}$created${NC}"
    fi
}

# Update bidder
update_bidder() {
    local code=$1
    local field=$2
    local value=$3

    if [ -z "$code" ] || [ -z "$field" ] || [ -z "$value" ]; then
        echo -e "${RED}Error: Missing arguments${NC}"
        echo ""
        echo "Usage: $0 update <bidder_code> <field> <value>"
        echo ""
        echo "Fields: bidder_name, endpoint_url, timeout_ms, status, enabled, gvl_vendor_id"
        echo "        supports_banner, supports_video, supports_native, supports_audio"
        echo ""
        echo "Examples:"
        echo "  $0 update rubicon bidder_name 'Rubicon (Updated)'"
        echo "  $0 update rubicon endpoint_url 'https://new-endpoint.com'"
        echo "  $0 update rubicon timeout_ms 2000"
        echo "  $0 update rubicon enabled false"
        echo "  $0 update rubicon status 'testing'"
        exit 1
    fi

    # Validate field
    case $field in
        bidder_name|endpoint_url|status|description|documentation_url|contact_email)
            local query="UPDATE bidders SET $field='$value' WHERE bidder_code='$code';"
            ;;
        timeout_ms|gvl_vendor_id)
            local query="UPDATE bidders SET $field=$value WHERE bidder_code='$code';"
            ;;
        enabled|supports_banner|supports_video|supports_native|supports_audio)
            # Convert to boolean
            if [ "$value" = "true" ] || [ "$value" = "t" ] || [ "$value" = "1" ]; then
                value="true"
            else
                value="false"
            fi
            local query="UPDATE bidders SET $field=$value WHERE bidder_code='$code';"
            ;;
        *)
            echo -e "${RED}Invalid field: $field${NC}"
            echo "Valid fields: bidder_name, endpoint_url, timeout_ms, status, enabled, gvl_vendor_id,"
            echo "              supports_banner, supports_video, supports_native, supports_audio"
            exit 1
            ;;
    esac

    exec_sql "$query"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Successfully updated bidder${NC}"
        echo ""
        echo -e "  Bidder Code: ${BLUE}$code${NC}"
        echo -e "  Field: ${BLUE}$field${NC}"
        echo -e "  New Value: ${BLUE}$value${NC}"
    else
        echo -e "${RED}✗ Failed to update bidder${NC}"
        exit 1
    fi
}

# Enable/disable bidder
toggle_bidder() {
    local code=$1
    local enable=$2

    if [ -z "$code" ] || [ -z "$enable" ]; then
        echo -e "${RED}Error: Missing arguments${NC}"
        exit 1
    fi

    local query="UPDATE bidders SET enabled=$enable WHERE bidder_code='$code';"
    exec_sql "$query"

    if [ $? -eq 0 ]; then
        if [ "$enable" = "true" ]; then
            echo -e "${GREEN}✓ Enabled bidder: $code${NC}"
        else
            echo -e "${YELLOW}✓ Disabled bidder: $code${NC}"
        fi
    else
        echo -e "${RED}✗ Failed to update bidder${NC}"
        exit 1
    fi
}

# Show help
show_help() {
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}Catalyst Bidder Management (PostgreSQL)${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  list                                  List all bidders"
    echo "  add <code> <name> <url> [timeout] [gvl]  Add a new bidder"
    echo "  remove <code>                         Archive a bidder"
    echo "  check <code>                          Check bidder details"
    echo "  update <code> <field> <value>         Update bidder field"
    echo "  enable <code>                         Enable a bidder"
    echo "  disable <code>                        Disable a bidder"
    echo ""
    echo "Examples:"
    echo "  $0 list"
    echo "  $0 add rubicon 'Rubicon/Magnite' 'https://prebid-server.rubiconproject.com/openrtb2/auction'"
    echo "  $0 add custom 'Custom SSP' 'https://custom.com/bid' 1500 999"
    echo "  $0 check rubicon"
    echo "  $0 update rubicon timeout_ms 2000"
    echo "  $0 disable rubicon"
    echo "  $0 enable rubicon"
    echo "  $0 remove rubicon"
    echo ""
    echo "Common Fields:"
    echo "  bidder_name        Display name"
    echo "  endpoint_url       OpenRTB endpoint URL"
    echo "  timeout_ms         Request timeout (100-10000ms)"
    echo "  enabled            true/false"
    echo "  status             active/testing/disabled/archived"
    echo "  gvl_vendor_id      IAB GVL ID for GDPR"
    echo "  supports_banner    true/false"
    echo "  supports_video     true/false"
    echo "  supports_native    true/false"
    echo ""
}

# Main
case "${1:-}" in
    list)
        check_postgres
        list_bidders
        ;;
    add)
        check_postgres
        add_bidder "$2" "$3" "$4" "$5" "$6"
        ;;
    remove)
        check_postgres
        remove_bidder "$2"
        ;;
    check)
        check_postgres
        check_bidder "$2"
        ;;
    update)
        check_postgres
        update_bidder "$2" "$3" "$4"
        ;;
    enable)
        check_postgres
        toggle_bidder "$2" "true"
        ;;
    disable)
        check_postgres
        toggle_bidder "$2" "false"
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo -e "${RED}Unknown command: ${1:-}${NC}"
        echo ""
        show_help
        exit 1
        ;;
esac
