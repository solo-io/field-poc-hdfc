#!/bin/bash
# test-ratelimit.sh - Test rate limit allocations
#
# Usage:
#   ./test-ratelimit.sh [options]
#
# Options:
#   --create            Create a test rate limit allocation
#   --list              List all rate limit allocations
#   --test [N]          Send N requests rapidly to trigger rate limiting (default: 20)
#   --delete ID         Delete a rate limit allocation by ID
#   --token <jwt>       Use a pre-obtained JWT as Authorization: Bearer
#   --user <name>       Fetch token for a named test user (password: Passwd00)
#   --username <u>      Keycloak username (use with --password)
#   --password <p>      Keycloak password (default: Passwd00)
#   -H, --header        Add custom header (can be repeated)
#   -v, --verbose       Show request/response headers for debugging
#   --help              Show this help message
#
# Known test users (all use password 'Passwd00'):
#   acme-corp-admin   org_id=acme-corp, is_org=true
#   user1             org_id=acme-corp, team_id=team-alpha
#   user2             org_id=acme-corp, team_id=team-alpha
#   team-alpha-admin  org_id=acme-corp, team_id=team-alpha
#   team-beta-admin   org_id=acme-corp, team_id=team-beta

set -euo pipefail

# Configuration
NAMESPACE="${NAMESPACE:-agentgateway-system}"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_URL="${API_URL:-http://localhost:8080/api/v1}"
MODEL="${MODEL:-gpt-4o-mini}"
TEAM_ID="${TEAM_ID:-test-team}"
ORG_ID="${ORG_ID:-test-org}"

# Keycloak config for token fetch
KEYCLOAK_URL="${KEYCLOAK_URL:-}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-agw-dev}"
KEYCLOAK_CLIENT_ID="${KEYCLOAK_CLIENT_ID:-quota-management}"
KEYCLOAK_CLIENT_SECRET="${KEYCLOAK_CLIENT_SECRET:-quota-management-secret}"

# Auth state
AUTH_TOKEN=""
KC_USERNAME=""
KC_PASSWORD="Passwd00"

# Custom headers array
CUSTOM_HEADERS=()
VERBOSE=false

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }
log_error() { echo -e "${RED}[FAIL]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

fetch_token() {
  local username="$1"
  local password="$2"

  if [[ -z "$KEYCLOAK_URL" ]]; then
    log_error "KEYCLOAK_URL is required to fetch tokens (e.g. KEYCLOAK_URL=https://keycloak.example.com)"
    exit 1
  fi

  local token_url="${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/protocol/openid-connect/token"
  log_info "Fetching token for '${username}' from ${KEYCLOAK_REALM}..."

  local response
  response=$(curl -sk -X POST "$token_url" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=${KEYCLOAK_CLIENT_ID}" \
    -d "client_secret=${KEYCLOAK_CLIENT_SECRET}" \
    -d "username=${username}" \
    -d "password=${password}" \
    -d "scope=openid email profile" 2>&1)

  local token
  token=$(echo "$response" | jq -r '.access_token // empty' 2>/dev/null)

  if [[ -z "$token" || "$token" == "null" ]]; then
    local err
    err=$(echo "$response" | jq -r '.error_description // .error // .' 2>/dev/null || echo "$response")
    log_error "Failed to fetch token: $err"
    exit 1
  fi

  AUTH_TOKEN="$token"
  log_success "Token obtained for '${username}'"

  # Decode and display relevant claims for visibility
  local payload
  payload=$(echo "$token" | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null || true)
  if [[ -n "$payload" ]]; then
    local org_id team_id is_org
    org_id=$(echo "$payload" | jq -r '.org_id // empty' 2>/dev/null || true)
    team_id=$(echo "$payload" | jq -r '.team_id // empty' 2>/dev/null || true)
    is_org=$(echo "$payload" | jq -r '.is_org // empty' 2>/dev/null || true)
    [[ -n "$org_id" ]] && log_info "  claim org_id:  $org_id" || true
    [[ -n "$team_id" ]] && log_info "  claim team_id: $team_id" || true
    [[ -n "$is_org" ]] && log_info "  claim is_org:  $is_org" || true
  fi
}

show_help() {
  cat << 'EOF'
test-ratelimit.sh - Test rate limit allocations

Usage:
  ./test-ratelimit.sh [options]

Options:
  --create              Create a test rate limit allocation (5 req/min for TEAM_ID)
  --create-token        Create a token-based rate limit (1000 tokens/min)
  --list                List all rate limit allocations
  --approve ID          Approve a pending rate limit allocation
  --test [N]            Send N requests rapidly (default: 20)
  --delete ID           Delete a rate limit allocation by ID
  --token <jwt>         Use a pre-obtained JWT as Authorization: Bearer
  --user <name>         Fetch token for a named test user (password: Passwd00)
  --username <u>        Keycloak username (use with --password)
  --password <p>        Keycloak password (default: Passwd00)
  -H, --header VALUE    Add custom header (can be repeated)
  -v, --verbose         Show request/response headers for debugging
  --help                Show this help message

Environment variables:
  GATEWAY_URL             Gateway URL (default: http://localhost:8080)
  API_URL                 API URL for management (default: http://localhost:8080/api/v1)
  MODEL                   Model to use (default: gpt-4o-mini)
  TEAM_ID                 Team ID for rate limit (default: test-team)
  ORG_ID                  Org ID for rate limit (default: test-org)
  NAMESPACE               Kubernetes namespace (default: agentgateway-system)
  KEYCLOAK_URL            Keycloak base URL (required for token fetch)
  KEYCLOAK_REALM          Keycloak realm (default: agw-dev)
  KEYCLOAK_CLIENT_ID      OAuth client ID (default: quota-management)
  KEYCLOAK_CLIENT_SECRET  OAuth client secret (default: quota-management-secret)

Known test users (all use password 'Passwd00'):
  acme-corp-admin   org_id=acme-corp, is_org=true
  user1             org_id=acme-corp, team_id=team-alpha
  user2             org_id=acme-corp, team_id=team-alpha
  team-alpha-admin  org_id=acme-corp, team_id=team-alpha
  team-beta-admin   org_id=acme-corp, team_id=team-beta

Examples:
  # Create and test rate limiting
  ./test-ratelimit.sh --create                    # Create allocation (pending)
  ./test-ratelimit.sh --approve <ID>              # Approve the allocation
  ./test-ratelimit.sh --test 10                   # Send 10 rapid requests

  # With custom team
  TEAM_ID=ml-team ./test-ratelimit.sh --create --test 20

  # Fetch token for a test user and send requests
  KEYCLOAK_URL=https://keycloak.example.com ./test-ratelimit.sh --user user1 --test 10

  # Use a pre-obtained token
  ./test-ratelimit.sh --token "$MY_JWT" --test 5

  # List and delete
  ./test-ratelimit.sh --list
  ./test-ratelimit.sh --delete <allocation-id>
EOF
}

list_allocations() {
  log_info "Listing rate limit allocations..."
  echo ""

  response=$(curl -s "$API_URL/rate-limits" \
    -H "Content-Type: application/json" \
    -H "x-org-id: $ORG_ID")

  if echo "$response" | jq -e '.allocations' >/dev/null 2>&1; then
    count=$(echo "$response" | jq '.allocations | length')
    if [[ "$count" == "0" ]]; then
      log_warn "No rate limit allocations found"
    else
      echo -e "${CYAN}Found $count allocation(s):${NC}"
      echo ""
      echo "$response" | jq -r '.allocations[] | "ID: \(.id)\n  Team: \(.team_id) | Model: \(.model_pattern)\n  Token Limit: \(.token_limit // "N/A") \(.token_unit // "")\n  Request Limit: \(.request_limit // "N/A") \(.request_unit // "")\n  Status: \(.approval_status) | Enabled: \(.enabled) | Enforcement: \(.enforcement)\n"'
    fi
  else
    log_error "Failed to list allocations"
    echo "$response" | jq . 2>/dev/null || echo "$response"
  fi
}

create_allocation() {
  local limit_type="${1:-request}"

  log_info "Creating $limit_type-based rate limit allocation..."
  log_info "  Team: $TEAM_ID"
  log_info "  Org: $ORG_ID"
  log_info "  Model: $MODEL"

  local payload
  if [[ "$limit_type" == "token" ]]; then
    log_info "  Limit: 1000 tokens/minute"
    payload=$(cat <<EOF
{
  "team_id": "$TEAM_ID",
  "org_id": "$ORG_ID",
  "model_pattern": "$MODEL",
  "token_limit": 1000,
  "token_unit": "MINUTE",
  "enforcement": "enforced",
  "description": "Test token rate limit - 1000 tokens/min"
}
EOF
)
  else
    log_info "  Limit: 5 requests/minute"
    payload=$(cat <<EOF
{
  "team_id": "$TEAM_ID",
  "org_id": "$ORG_ID",
  "model_pattern": "$MODEL",
  "request_limit": 5,
  "request_unit": "MINUTE",
  "enforcement": "enforced",
  "description": "Test request rate limit - 5 req/min"
}
EOF
)
  fi

  echo ""
  response=$(curl -s -X POST "$API_URL/rate-limits" \
    -H "Content-Type: application/json" \
    -H "x-org-id: $ORG_ID" \
    -H "x-team-id: $TEAM_ID" \
    -d "$payload")

  if echo "$response" | jq -e '.id' >/dev/null 2>&1; then
    id=$(echo "$response" | jq -r '.id')
    status=$(echo "$response" | jq -r '.approval_status')
    log_success "Created allocation: $id"
    log_info "Status: $status"
    if [[ "$status" == "pending" ]]; then
      log_warn "Allocation needs approval. Run: ./test-ratelimit.sh --approve $id"
    fi
  else
    log_error "Failed to create allocation"
    echo "$response" | jq . 2>/dev/null || echo "$response"
  fi
}

approve_allocation() {
  local id="$1"

  log_info "Approving rate limit allocation: $id"

  response=$(curl -s -X POST "$API_URL/rate-limits/$id/approve" \
    -H "Content-Type: application/json" \
    -H "x-org-id: $ORG_ID" \
    -d '{"enforcement": "enforced"}')

  if echo "$response" | jq -e '.approval_status' >/dev/null 2>&1; then
    status=$(echo "$response" | jq -r '.approval_status')
    if [[ "$status" == "approved" ]]; then
      log_success "Allocation approved"
    else
      log_warn "Allocation status: $status"
    fi
  else
    log_error "Failed to approve allocation"
    echo "$response" | jq . 2>/dev/null || echo "$response"
  fi
}

delete_allocation() {
  local id="$1"

  log_info "Deleting rate limit allocation: $id"

  response=$(curl -s -X DELETE "$API_URL/rate-limits/$id" \
    -H "x-org-id: $ORG_ID")

  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API_URL/rate-limits/$id" \
    -H "x-org-id: $ORG_ID")

  if [[ "$http_code" == "204" ]] || [[ "$http_code" == "200" ]]; then
    log_success "Allocation deleted"
  else
    log_error "Failed to delete allocation (HTTP $http_code)"
    echo "$response"
  fi
}

send_requests() {
  local num_requests="$1"

  local headers=("-H" "Content-Type: application/json")

  # Add auth token if available, otherwise use manual headers
  if [[ -n "$AUTH_TOKEN" ]]; then
    headers+=("-H" "Authorization: Bearer $AUTH_TOKEN")
  else
    headers+=("-H" "x-gw-team-id: $TEAM_ID")
    headers+=("-H" "x-gw-org-id: $ORG_ID")
  fi
  headers+=("-H" "x-gw-llm-model: $MODEL")

  # Add custom headers
  for h in "${CUSTOM_HEADERS[@]+"${CUSTOM_HEADERS[@]}"}"; do
    headers+=("$h")
  done

  echo ""
  log_info "Sending $num_requests rapid requests to test rate limiting..."
  if [[ -n "$AUTH_TOKEN" ]]; then
    log_info "Auth: Bearer token (gateway extracts claims → x-gw-org-id, x-gw-team-id)"
  else
    log_info "Team: $TEAM_ID | Org: $ORG_ID"
  fi
  log_info "Model: $MODEL"
  echo ""

  local success=0
  local rate_limited=0
  local other=0

  for i in $(seq 1 "$num_requests"); do
    printf "Request %2d: " "$i"

    response=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL/openai/v1/chat/completions" \
      "${headers[@]}" \
      -d '{
        "model": "'"$MODEL"'",
        "messages": [{"role": "user", "content": "What is kubernetes?"}],
        "max_tokens": 10
      }' 2>&1) || true

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [[ "$response" == *"curl:"* ]] || [[ -z "$http_code" ]] || ! [[ "$http_code" =~ ^[0-9]+$ ]]; then
      echo -e "${RED}CONNECTION FAILED${NC}"
      ((other++))
    elif [[ "$http_code" == "200" ]]; then
      echo -e "${GREEN}OK${NC}"
      ((success++))
    elif [[ "$http_code" == "429" ]]; then
      retry_after=$(echo "$body" | jq -r '.error.retry_after // empty' 2>/dev/null || true)
      if [[ -n "$retry_after" ]]; then
        echo -e "${YELLOW}RATE LIMITED (retry after ${retry_after}s)${NC}"
      else
        echo -e "${YELLOW}RATE LIMITED${NC}"
      fi
      ((rate_limited++))
    else
      echo -e "${RED}HTTP $http_code${NC}"
      ((other++))
    fi
  done

  echo ""
  echo -e "${CYAN}Summary:${NC}"
  echo -e "  ${GREEN}Success:${NC}      $success"
  echo -e "  ${YELLOW}Rate Limited:${NC} $rate_limited"
  echo -e "  ${RED}Other:${NC}        $other"

  if [[ $rate_limited -gt 0 ]]; then
    log_success "Rate limiting is working!"
  elif [[ $success -eq $num_requests ]]; then
    log_warn "No rate limiting triggered. Check if allocation exists and is approved."
  fi
}

# Main
main() {
  local do_create=false
  local do_create_token=false
  local do_list=false
  local do_test=false
  local do_approve=""
  local do_delete=""
  local num_requests=20

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --create)
        do_create=true
        shift
        ;;
      --create-token)
        do_create_token=true
        shift
        ;;
      --list)
        do_list=true
        shift
        ;;
      --approve)
        shift
        if [[ $# -gt 0 ]]; then
          do_approve="$1"
          shift
        else
          log_error "Missing allocation ID for --approve"
          exit 1
        fi
        ;;
      --delete)
        shift
        if [[ $# -gt 0 ]]; then
          do_delete="$1"
          shift
        else
          log_error "Missing allocation ID for --delete"
          exit 1
        fi
        ;;
      --test)
        do_test=true
        shift
        if [[ $# -gt 0 && "$1" =~ ^[0-9]+$ ]]; then
          num_requests="$1"
          shift
        fi
        ;;
      --token)
        shift
        if [[ $# -gt 0 ]]; then
          AUTH_TOKEN="$1"
          shift
        else
          log_error "Missing value for --token"
          exit 1
        fi
        ;;
      --user)
        shift
        if [[ $# -gt 0 ]]; then
          KC_USERNAME="$1"
          shift
        else
          log_error "Missing value for --user"
          exit 1
        fi
        ;;
      --username)
        shift
        if [[ $# -gt 0 ]]; then
          KC_USERNAME="$1"
          shift
        else
          log_error "Missing value for --username"
          exit 1
        fi
        ;;
      --password)
        shift
        if [[ $# -gt 0 ]]; then
          KC_PASSWORD="$1"
          shift
        else
          log_error "Missing value for --password"
          exit 1
        fi
        ;;
      -H|--header)
        shift
        if [[ $# -gt 0 ]]; then
          CUSTOM_HEADERS+=("-H" "$1")
          shift
        else
          log_error "Missing value for --header"
          exit 1
        fi
        ;;
      -v|--verbose)
        VERBOSE=true
        shift
        ;;
      --help|-h)
        show_help
        exit 0
        ;;
      *)
        if [[ "$1" =~ ^[0-9]+$ ]]; then
          num_requests="$1"
          do_test=true
          shift
        else
          log_error "Unknown option: $1"
          show_help
          exit 1
        fi
        ;;
    esac
  done

  # Fetch token if username was given and no token already set
  if [[ -z "$AUTH_TOKEN" && -n "$KC_USERNAME" ]]; then
    fetch_token "$KC_USERNAME" "$KC_PASSWORD"
  fi

  if [[ "$do_list" == "true" ]]; then
    list_allocations
  fi

  if [[ "$do_create" == "true" ]]; then
    create_allocation "request"
  fi

  if [[ "$do_create_token" == "true" ]]; then
    create_allocation "token"
  fi

  if [[ -n "$do_approve" ]]; then
    approve_allocation "$do_approve"
  fi

  if [[ -n "$do_delete" ]]; then
    delete_allocation "$do_delete"
  fi

  if [[ "$do_test" == "true" ]]; then
    send_requests "$num_requests"
  fi

  if [[ "$do_create" == "false" && "$do_create_token" == "false" && "$do_list" == "false" && "$do_test" == "false" && -z "$do_approve" && -z "$do_delete" ]]; then
    log_warn "No action specified."
    show_help
    exit 1
  fi
}

main "$@"
