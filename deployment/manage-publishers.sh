#!/bin/bash
# Publisher Management Script for Catalyst (PostgreSQL)
# Usage: ./manage-publishers.sh <command> [options]

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

# List all publishers
list_publishers() {
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}Registered Publishers in Catalyst${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"

    local query="SELECT publisher_id, name, allowed_domains, status FROM publishers ORDER BY publisher_id;"
    local output=$(exec_sql "$query")

    if [ -z "$output" ]; then
        echo -e "${YELLOW}No publishers registered yet${NC}"
        echo ""
        echo "Add your first publisher:"
        echo "  $0 add totalsportspro 'Total Sports Pro' 'totalsportspro.com'"
        return
    fi

    echo ""
    printf "%-20s %-30s %-30s %s\n" "PUBLISHER ID" "NAME" "DOMAINS" "STATUS"
    echo "─────────────────────────────────────────────────────────────────────────────────────────"

    echo "$output" | while IFS='|' read -r pub_id name domains status; do
        printf "%-20s %-30s %-30s %s\n" "$pub_id" "$name" "$domains" "$status"
    done

    local count=$(echo "$output" | wc -l | tr -d ' ')
    echo ""
    echo -e "${GREEN}Total: $count publisher(s)${NC}"
}

# Add publisher
add_publisher() {
    local pub_id=$1
    local name=$2
    local domains=$3
    local bidder_params=${4:-'{}'}

    if [ -z "$pub_id" ] || [ -z "$name" ] || [ -z "$domains" ]; then
        echo -e "${RED}Error: Missing arguments${NC}"
        echo ""
        echo "Usage: $0 add <publisher_id> <name> <domains> [bidder_params]"
        echo ""
        echo "Examples:"
        echo "  $0 add totalsportspro 'Total Sports Pro' 'totalsportspro.com'"
        echo "  $0 add publisher2 'Publisher 2' 'example.com|*.example.com'"
        echo "  $0 add testpub 'Test Publisher' '*' '{\"rubicon\":{\"accountId\":123}}'"
        exit 1
    fi

    # Check if already exists
    local existing=$(exec_sql "SELECT publisher_id FROM publishers WHERE publisher_id='$pub_id';")
    if [ -n "$existing" ]; then
        echo -e "${YELLOW}Warning: Publisher '$pub_id' already exists${NC}"
        echo -e "${YELLOW}Use 'update' command to change configuration${NC}"
        exit 1
    fi

    # Insert publisher
    local query="INSERT INTO publishers (publisher_id, name, allowed_domains, bidder_params, status) VALUES ('$pub_id', '$name', '$domains', '$bidder_params'::jsonb, 'active');"
    exec_sql "$query"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Successfully added publisher${NC}"
        echo ""
        echo -e "  Publisher ID: ${BLUE}$pub_id${NC}"
        echo -e "  Name: ${BLUE}$name${NC}"
        echo -e "  Allowed Domains: ${BLUE}$domains${NC}"
        echo -e "  Bidder Params: ${BLUE}$bidder_params${NC}"
        echo ""
        echo -e "${YELLOW}Remember to configure CORS if needed:${NC}"
        echo "  CORS_ALLOWED_ORIGINS=https://yourdomain.com"
    else
        echo -e "${RED}✗ Failed to add publisher${NC}"
        exit 1
    fi
}

# Remove publisher
remove_publisher() {
    local pub_id=$1

    if [ -z "$pub_id" ]; then
        echo -e "${RED}Error: Missing publisher ID${NC}"
        echo ""
        echo "Usage: $0 remove <publisher_id>"
        echo ""
        echo "Example:"
        echo "  $0 remove totalsportspro"
        exit 1
    fi

    # Check if exists
    local existing=$(exec_sql "SELECT publisher_id, name FROM publishers WHERE publisher_id='$pub_id';")
    if [ -z "$existing" ]; then
        echo -e "${YELLOW}Publisher '$pub_id' not found${NC}"
        exit 1
    fi

    # Soft delete (archive)
    local query="UPDATE publishers SET status='archived' WHERE publisher_id='$pub_id';"
    exec_sql "$query"

    echo -e "${GREEN}✓ Successfully archived publisher: $pub_id${NC}"
    echo -e "${YELLOW}Previous details: $existing${NC}"
}

# Check publisher
check_publisher() {
    local pub_id=$1

    if [ -z "$pub_id" ]; then
        echo -e "${RED}Error: Missing publisher ID${NC}"
        echo ""
        echo "Usage: $0 check <publisher_id>"
        echo ""
        echo "Example:"
        echo "  $0 check totalsportspro"
        exit 1
    fi

    local query="SELECT publisher_id, name, allowed_domains, bidder_params, status, created_at FROM publishers WHERE publisher_id='$pub_id';"
    local output=$(exec_sql "$query")

    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    if [ -z "$output" ]; then
        echo -e "${YELLOW}Publisher '$pub_id' NOT REGISTERED${NC}"
        echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
        echo ""
        echo "Add this publisher:"
        echo "  $0 add $pub_id 'Publisher Name' 'domain.com'"
    else
        IFS='|' read -r pub_id name domains bidder_params status created_at <<< "$output"
        echo -e "${GREEN}Publisher: $pub_id${NC}"
        echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
        echo -e "  Name: ${GREEN}$name${NC}"
        echo -e "  Status: ${GREEN}$status${NC}"
        echo -e "  Allowed Domains: ${BLUE}$domains${NC}"
        echo -e "  Bidder Params: ${BLUE}$bidder_params${NC}"
        echo -e "  Created: ${BLUE}$created_at${NC}"

        # Parse and display domains
        echo ""
        echo "Domain Rules:"
        IFS='|' read -ra DOMAINS <<< "$domains"
        for domain in "${DOMAINS[@]}"; do
            domain=$(echo "$domain" | xargs) # trim whitespace
            if [ "$domain" = "*" ]; then
                echo -e "  • ${YELLOW}$domain${NC} (any domain - permissive!)"
            elif [[ "$domain" == \** ]]; then
                echo -e "  • ${BLUE}$domain${NC} (wildcard subdomain)"
            else
                echo -e "  • ${BLUE}$domain${NC}"
            fi
        done
    fi
}

# Update publisher
update_publisher() {
    local pub_id=$1
    local field=$2
    local value=$3

    if [ -z "$pub_id" ] || [ -z "$field" ] || [ -z "$value" ]; then
        echo -e "${RED}Error: Missing arguments${NC}"
        echo ""
        echo "Usage: $0 update <publisher_id> <field> <value>"
        echo ""
        echo "Fields: name, allowed_domains, bidder_params, status"
        echo ""
        echo "Examples:"
        echo "  $0 update totalsportspro name 'New Publisher Name'"
        echo "  $0 update totalsportspro allowed_domains 'newdomain.com'"
        echo "  $0 update totalsportspro bidder_params '{\"rubicon\":{\"accountId\":999}}'"
        echo "  $0 update totalsportspro status 'paused'"
        exit 1
    fi

    # Validate field
    case $field in
        name|allowed_domains|status)
            local query="UPDATE publishers SET $field='$value' WHERE publisher_id='$pub_id';"
            ;;
        bidder_params)
            local query="UPDATE publishers SET $field='$value'::jsonb WHERE publisher_id='$pub_id';"
            ;;
        *)
            echo -e "${RED}Invalid field: $field${NC}"
            echo "Valid fields: name, allowed_domains, bidder_params, status"
            exit 1
            ;;
    esac

    exec_sql "$query"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Successfully updated publisher${NC}"
        echo ""
        echo -e "  Publisher ID: ${BLUE}$pub_id${NC}"
        echo -e "  Field: ${BLUE}$field${NC}"
        echo -e "  New Value: ${BLUE}$value${NC}"
    else
        echo -e "${RED}✗ Failed to update publisher${NC}"
        exit 1
    fi
}

# Show help
show_help() {
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}Catalyst Publisher Management (PostgreSQL)${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  list                                  List all publishers"
    echo "  add <id> <name> <domains> [params]   Add a new publisher"
    echo "  remove <id>                           Archive a publisher"
    echo "  check <id>                            Check publisher details"
    echo "  update <id> <field> <value>           Update publisher field"
    echo ""
    echo "Examples:"
    echo "  $0 list"
    echo "  $0 add totalsportspro 'Total Sports Pro' 'totalsportspro.com'"
    echo "  $0 check totalsportspro"
    echo "  $0 update totalsportspro status 'paused'"
    echo "  $0 remove totalsportspro"
    echo ""
    echo "Bidder Parameters Example:"
    echo "  $0 add pub123 'Publisher' 'domain.com' '{\"rubicon\":{\"accountId\":26298,\"siteId\":556630,\"zoneId\":3767186}}'"
    echo ""
}

# Main
case "${1:-}" in
    list)
        check_postgres
        list_publishers
        ;;
    add)
        check_postgres
        add_publisher "$2" "$3" "$4" "$5"
        ;;
    remove)
        check_postgres
        remove_publisher "$2"
        ;;
    check)
        check_postgres
        check_publisher "$2"
        ;;
    update)
        check_postgres
        update_publisher "$2" "$3" "$4"
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
