#!/bin/bash
# test-ui-budget.sh - Send requests to test UI-configured budgets
#
# Usage:
#   ./test-ui-budget.sh [options]
#
# Options:
#   --reset-db        Reset the PostgreSQL database
#   --flush-metrics   Flush Prometheus metrics by restarting budget-management
#   --test [N]        Send N requests (default: 10)
#   -H, --header      Add custom header (can be repeated)
#   --help            Show this help message

set -euo pipefail

# Configuration
NAMESPACE="${NAMESPACE:-agentgateway-system}"
POSTGRES_POD="${POSTGRES_POD:-budget-management-postgres-0}"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
MODEL="${MODEL:-gpt-4o-mini}"

# Custom headers array
CUSTOM_HEADERS=()

# Default identity headers from environment
[[ -n "${ORG_ID:-}" ]] && CUSTOM_HEADERS+=("-H" "x-org-id: $ORG_ID")
[[ -n "${TEAM_ID:-}" ]] && CUSTOM_HEADERS+=("-H" "x-team-id: $TEAM_ID")
[[ -n "${USER_ID:-}" ]] && CUSTOM_HEADERS+=("-H" "x-user-id: $USER_ID")

# Always include x-model header for accurate cost estimation
CUSTOM_HEADERS+=("-H" "x-model: $MODEL")

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
test-ui-budget.sh - Test UI-configured budgets

Usage:
  ./test-ui-budget.sh [options]

Options:
  --reset-db          Reset the PostgreSQL database
  --flush-metrics     Flush Prometheus metrics by restarting budget-management
  --test [N]          Send N requests (default: 10)
  -H, --header VALUE  Add custom header (can be repeated)
  --help              Show this help message

Environment variables:
  GATEWAY_URL       Gateway URL (default: http://localhost:8080)
  MODEL             Model to use (default: gpt-4o-mini)
  NAMESPACE         Kubernetes namespace (default: agentgateway-system)
  ORG_ID            Auto-add x-org-id header
  TEAM_ID           Auto-add x-team-id header
  USER_ID           Auto-add x-user-id header

Examples:
  ./test-ui-budget.sh --test 20             # Send 20 requests
  ./test-ui-budget.sh --reset-db            # Just reset DB
  ./test-ui-budget.sh --flush-metrics       # Just flush metrics
  ./test-ui-budget.sh --reset-db --test 10  # Reset DB, then send 10 requests
  MODEL=gpt-4o ./test-ui-budget.sh --test   # Use gpt-4o model, 10 requests

  # With custom headers (number can be anywhere)
  ./test-ui-budget.sh --test -H "x-org-id: acme" -H "x-team-id: engineering" 5
  ./test-ui-budget.sh -H "x-team-id: ml-team" 10

  # Using environment variables for headers
  ORG_ID=acme TEAM_ID=engineering ./test-ui-budget.sh --test 5
EOF
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
  log_info "Flushing Prometheus metrics (restarting budget-management)..."

  kubectl rollout restart deployment/budget-management -n "$NAMESPACE" 2>/dev/null

  if [[ $? -eq 0 ]]; then
    log_info "Waiting for rollout to complete..."
    kubectl rollout status deployment/budget-management -n "$NAMESPACE" --timeout=60s 2>/dev/null

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

  echo ""
  log_info "Sending $num_requests requests to $GATEWAY_URL/openai..."
  if [[ ${#CUSTOM_HEADERS[@]} -gt 0 ]]; then
    log_info "Custom headers: ${CUSTOM_HEADERS[*]}"
  fi
  echo ""

  for i in $(seq 1 "$num_requests"); do
    echo -n "Request $i: "

    response=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL/openai/v1/chat/completions" \
      -H "Content-Type: application/json" \
      ${CUSTOM_HEADERS[@]+"${CUSTOM_HEADERS[@]}"} \
      -d '{
        "model": "'"$MODEL"'",
        "messages": [{"role": "user", "content": "Say hello in one word"}],
        "max_tokens": 10
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
      --help|-h)
        show_help
        exit 0
        ;;
      *)
        # Handle positional numeric argument as request count
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
