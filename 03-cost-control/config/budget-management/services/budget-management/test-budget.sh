#!/bin/bash
# test-budget.sh - Send requests to test UI-configured budgets
#
# Usage:
#   ./test-budget.sh [options]
#
# Options:
#   --reset-db              Reset the PostgreSQL database
#   --flush-metrics         Flush Prometheus metrics by restarting quota-management
#   --test [N]              Send N requests (default: 10)
#   --token <jwt>           Use a pre-obtained JWT as Authorization: Bearer
#   --user <name>           Fetch token for a named test user (password: Passwd00)
#   --username <u>          Keycloak username (use with --password)
#   --password <p>          Keycloak password (default: Passwd00)
#   -H, --header <h>        Add custom header (can be repeated)
#   -v, --verbose           Show request/response headers for debugging
#   --help                  Show this help message
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
POSTGRES_POD="${POSTGRES_POD:-quota-management-postgres-0}"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
MODEL="${MODEL:-gpt-4o-mini}"

# Keycloak config for token fetch
KEYCLOAK_URL="${KEYCLOAK_URL:-}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-agw-dev}"
KEYCLOAK_CLIENT_ID="${KEYCLOAK_CLIENT_ID:-quota-management}"
KEYCLOAK_CLIENT_SECRET="${KEYCLOAK_CLIENT_SECRET:-quota-management-secret}"

# Auth state
AUTH_TOKEN=""
KC_USERNAME=""
KC_PASSWORD="Passwd00"

# Extra headers
CUSTOM_HEADERS=()
VERBOSE=false

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }
log_error() { echo -e "${RED}[FAIL]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

show_help() {
  cat << 'EOF'
test-budget.sh - Test UI-configured budgets

Usage:
  ./test-budget.sh [options]

Options:
  --reset-db              Reset the PostgreSQL database
  --flush-metrics         Flush Prometheus metrics by restarting quota-management
  --test [N]              Send N requests (default: 10)
  --token <jwt>           Use a pre-obtained JWT as Authorization: Bearer
  --user <name>           Fetch token for a named test user (password: Passwd00)
  --username <u>          Keycloak username (use with --password)
  --password <p>          Keycloak password (default: Passwd00)
  -H, --header <h>        Add custom header (can be repeated)
  -v, --verbose           Show request/response headers for debugging
  --help                  Show this help message

Environment variables:
  GATEWAY_URL             Gateway URL (default: http://localhost:8080)
  MODEL                   Model to use (default: gpt-4o-mini)
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
  # Fetch token for a test user and send 10 requests
  KEYCLOAK_URL=https://keycloak.example.com ./test-budget.sh --user user1 --test 10

  # Use a pre-obtained token
  ./test-budget.sh --token "$MY_JWT" --test 5

  # Fetch token with explicit credentials, override count
  KEYCLOAK_URL=https://keycloak.example.com ./test-budget.sh --username acme-corp-admin --password Passwd00 --test 20

  # Reset DB, then send requests as team-beta
  KEYCLOAK_URL=https://keycloak.example.com ./test-budget.sh --reset-db --user team-beta-admin --test 10
EOF
}

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

reset_database() {
  log_info "Resetting PostgreSQL database..."

  kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U budget -d budget_management -c "
    TRUNCATE TABLE request_reservations CASCADE;
    TRUNCATE TABLE usage_records CASCADE;
    TRUNCATE TABLE budget_definitions CASCADE;
  " 2>/dev/null

  if [[ $? -eq 0 ]]; then
    log_success "Database reset successfully"
  else
    log_error "Failed to reset database"
    exit 1
  fi
}

flush_prometheus() {
  log_info "Flushing Prometheus metrics (restarting quota-management)..."

  kubectl rollout restart deployment/quota-management -n "$NAMESPACE" 2>/dev/null

  if [[ $? -eq 0 ]]; then
    log_info "Waiting for rollout to complete..."
    kubectl rollout status deployment/quota-management -n "$NAMESPACE" --timeout=60s 2>/dev/null

    if [[ $? -eq 0 ]]; then
      log_success "Prometheus metrics flushed"
      sleep 2
    else
      log_error "Rollout failed"
      exit 1
    fi
  else
    log_error "Failed to restart deployment"
    exit 1
  fi
}

send_requests() {
  local num_requests="$1"

  local auth_headers=()
  if [[ -n "$AUTH_TOKEN" ]]; then
    auth_headers+=("-H" "Authorization: Bearer $AUTH_TOKEN")
  fi

  echo ""
  log_info "Sending $num_requests requests to $GATEWAY_URL/openai..."
  if [[ -n "$AUTH_TOKEN" ]]; then
    log_info "Auth: Bearer token (gateway extracts claims → x-gw-org-id, x-gw-team-id)"
  fi
  if [[ ${#CUSTOM_HEADERS[@]} -gt 0 ]]; then
    log_info "Extra headers: ${CUSTOM_HEADERS[*]}"
  fi
  echo ""

  local curl_verbose_flags=""
  if [[ "$VERBOSE" == "true" ]]; then
    curl_verbose_flags="-v"
  fi

  for i in $(seq 1 "$num_requests"); do
    echo -n "Request $i: "

    if [[ "$VERBOSE" == "true" ]]; then
      echo ""
      log_info "--- Request/Response Headers ---"
      curl $curl_verbose_flags -s -m 30 -w "\nHTTP_CODE:%{http_code}\n" "$GATEWAY_URL/openai/v1/chat/completions" \
        -H "Content-Type: application/json" \
        ${auth_headers[@]+"${auth_headers[@]}"} \
        ${CUSTOM_HEADERS[@]+"${CUSTOM_HEADERS[@]}"} \
        -d '{
          "model": "'"$MODEL"'",
          "messages": [{"role": "user", "content": "What is Kubernetes ?"}],
          "max_tokens": 1000
        }' 2>&1 | grep -E '^[<>*]|HTTP_CODE:' || true
      echo "--- End Headers ---"
      echo ""
    fi

    response=$(curl -s -m 30 -w "\n%{http_code}" "$GATEWAY_URL/openai/v1/chat/completions" \
      -H "Content-Type: application/json" \
      ${auth_headers[@]+"${auth_headers[@]}"} \
      ${CUSTOM_HEADERS[@]+"${CUSTOM_HEADERS[@]}"} \
      -d '{
        "model": "'"$MODEL"'",
        "messages": [{"role": "user", "content": "What is Kubernetes ?"}],
        "max_tokens": 1000
      }' 2>&1) || true

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [[ "$response" == *"curl:"* ]] || [[ -z "$http_code" ]] || ! [[ "$http_code" =~ ^[0-9]+$ ]]; then
      echo -e "${RED}✗ CONNECTION FAILED${NC}"
      echo "  $body"
    elif [ "$http_code" = "200" ]; then
      echo -e "${GREEN}✓ OK${NC}"
    elif [ "$http_code" = "429" ]; then
      echo -e "${RED}✗ RATE LIMITED (429)${NC}"
      echo "  $(echo "$body" | jq -r '.error.message // .message // .' 2>/dev/null || echo "$body")"
    else
      echo -e "${YELLOW}✗ HTTP $http_code${NC}"
      if [[ "$VERBOSE" == "true" ]]; then
        echo "  Body: $body"
      fi
    fi
  done
}

# Main
main() {
  local do_reset_db=false
  local do_flush_metrics=false
  local do_test=false
  local num_requests=10

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --reset-db)
        do_reset_db=true
        shift
        ;;
      --flush-metrics)
        do_flush_metrics=true
        shift
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

  if [[ "$do_reset_db" == "true" ]]; then
    reset_database
  fi

  if [[ "$do_flush_metrics" == "true" ]]; then
    flush_prometheus
  fi

  if [[ "$do_test" == "true" ]]; then
    send_requests "$num_requests"
  fi

  if [[ "$do_reset_db" == "false" && "$do_flush_metrics" == "false" && "$do_test" == "false" ]]; then
    log_warn "No action specified. Use --reset-db, --flush-metrics, or --test"
    show_help
    exit 1
  fi
}

main "$@"
