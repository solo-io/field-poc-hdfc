#!/bin/bash
set -e

#===============================================================================
# Keycloak Installation Script
# Installs Keycloak with PostgreSQL and configures realm, clients, and users
#===============================================================================

# Configuration (override via environment variables)
NAMESPACE="${KEYCLOAK_NAMESPACE:-keycloak}"
REALM="${KEYCLOAK_REALM:-agw-dev}"
ADMIN_USER="${KEYCLOAK_ADMIN_USER:-admin}"
ADMIN_PASS="${KEYCLOAK_ADMIN_PASS:-admin}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_step() { echo -e "${BLUE}[$1/7]${NC} $2"; }
log_ok() { echo -e "${GREEN}✓${NC} $1"; }
log_warn() { echo -e "${YELLOW}⚠${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1"; exit 1; }

#===============================================================================
# Step 1: Create Namespace
#===============================================================================
create_namespace() {
  log_step 1 "Creating namespace '${NAMESPACE}'..."
  kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
  log_ok "Namespace ready"
}

#===============================================================================
# Step 2: Deploy PostgreSQL
#===============================================================================
deploy_postgres() {
  log_step 2 "Deploying PostgreSQL..."

  kubectl apply -n ${NAMESPACE} -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
type: Opaque
stringData:
  POSTGRES_DB: keycloak
  POSTGRES_USER: postgres
  POSTGRES_PASSWORD: postgres123
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:16-alpine
        ports:
        - containerPort: 5432
        envFrom:
        - secretRef:
            name: postgres-secret
        resources:
          requests:
            memory: "256Mi"
            cpu: "200m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "postgres"]
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
EOF

  # Wait for PostgreSQL to be ready
  echo -n "  Waiting for PostgreSQL"
  kubectl wait --for=condition=available deployment/postgres -n ${NAMESPACE} --timeout=120s > /dev/null 2>&1 &
  while ! kubectl get pods -n ${NAMESPACE} -l app=postgres -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null | grep -q true; do
    echo -n "."
    sleep 2
  done
  echo ""
  log_ok "PostgreSQL ready"
}

#===============================================================================
# Step 3: Deploy Keycloak
#===============================================================================
deploy_keycloak() {
  log_step 3 "Deploying Keycloak..."

  kubectl apply -n ${NAMESPACE} -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-secret
type: Opaque
stringData:
  KEYCLOAK_ADMIN: ${ADMIN_USER}
  KEYCLOAK_ADMIN_PASSWORD: ${ADMIN_PASS}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
spec:
  replicas: 1
  selector:
    matchLabels:
      app: keycloak
  template:
    metadata:
      labels:
        app: keycloak
    spec:
      containers:
      - name: keycloak
        image: quay.io/keycloak/keycloak:26.0
        args: ["start-dev"]
        ports:
        - containerPort: 8080
        env:
        - name: KC_BOOTSTRAP_ADMIN_USERNAME
          valueFrom:
            secretKeyRef:
              name: keycloak-secret
              key: KEYCLOAK_ADMIN
        - name: KC_BOOTSTRAP_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: keycloak-secret
              key: KEYCLOAK_ADMIN_PASSWORD
        - name: KC_DB
          value: postgres
        - name: KC_DB_URL
          value: jdbc:postgresql://postgres:5432/keycloak
        - name: KC_DB_USERNAME
          value: postgres
        - name: KC_DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_PASSWORD
        - name: KC_HEALTH_ENABLED
          value: "true"
        - name: KC_METRICS_ENABLED
          value: "true"
        - name: KC_HTTP_ENABLED
          value: "true"
        - name: KC_HOSTNAME_STRICT
          value: "false"
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1"
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 60
          periodSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak
spec:
  type: LoadBalancer
  selector:
    app: keycloak
  ports:
  - name: http
    port: 8080
    targetPort: 8080
EOF

  log_ok "Keycloak manifests applied"
}

#===============================================================================
# Step 4: Wait for Keycloak
#===============================================================================
wait_for_keycloak() {
  log_step 4 "Waiting for Keycloak to be ready..."

  echo -n "  Starting"
  local max_attempts=60
  local attempt=0

  while [ $attempt -lt $max_attempts ]; do
    if kubectl get pods -n ${NAMESPACE} -l app=keycloak -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null | grep -q true; then
      echo ""
      log_ok "Keycloak ready"
      return 0
    fi
    echo -n "."
    sleep 5
    ((attempt++))
  done

  echo ""
  log_error "Keycloak failed to start within timeout"
}

#===============================================================================
# Step 5: Configure Keycloak - Get Admin Token
#===============================================================================
KEYCLOAK_URL=""
ACCESS_TOKEN=""

get_admin_token() {
  log_step 5 "Configuring Keycloak - Creating realm '${REALM}'..."

  # Start port-forward in background
  kubectl port-forward -n ${NAMESPACE} svc/keycloak 18080:8080 > /dev/null 2>&1 &
  local pf_pid=$!
  trap "kill $pf_pid 2>/dev/null" EXIT
  sleep 3

  KEYCLOAK_URL="http://localhost:18080"

  # Get admin token with retries
  local max_attempts=30
  local attempt=0

  while [ $attempt -lt $max_attempts ]; do
    ACCESS_TOKEN=$(curl -s -X POST "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
      -H "Content-Type: application/x-www-form-urlencoded" \
      -d "username=${ADMIN_USER}" \
      -d "password=${ADMIN_PASS}" \
      -d "grant_type=password" \
      -d "client_id=admin-cli" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

    if [ -n "$ACCESS_TOKEN" ]; then
      break
    fi
    sleep 2
    ((attempt++))
  done

  if [ -z "$ACCESS_TOKEN" ]; then
    log_error "Failed to get admin token"
  fi
}

#===============================================================================
# Step 5b: Create Realm
#===============================================================================
create_realm() {
  # Check if realm exists
  local exists=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "${KEYCLOAK_URL}/admin/realms/${REALM}")

  if [ "$exists" = "200" ]; then
    log_warn "Realm '${REALM}' already exists, skipping creation"
    return 0
  fi

  # Create realm
  curl -s -X POST "${KEYCLOAK_URL}/admin/realms" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "realm": "'"${REALM}"'",
      "enabled": true,
      "loginWithEmailAllowed": true,
      "duplicateEmailsAllowed": false,
      "resetPasswordAllowed": true,
      "editUsernameAllowed": false,
      "bruteForceProtected": false
    }'

  # Configure user profile attributes
  curl -s -X PUT "${KEYCLOAK_URL}/admin/realms/${REALM}/users/profile" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "attributes": [
        {"name": "username", "displayName": "Username", "required": {"roles": ["user"]}},
        {"name": "email", "displayName": "Email", "required": {"roles": ["user"]}},
        {"name": "firstName", "displayName": "First name"},
        {"name": "lastName", "displayName": "Last name"},
        {"name": "group", "displayName": "Group"},
        {"name": "org_id", "displayName": "Organization ID"},
        {"name": "team_id", "displayName": "Team ID"},
        {"name": "is_org", "displayName": "Is Organization"}
      ],
      "unmanagedAttributePolicy": "ENABLED"
    }'

  log_ok "Realm '${REALM}' created"
}

#===============================================================================
# Step 6: Create Clients
#===============================================================================
create_clients() {
  log_step 6 "Creating clients..."

  # Confidential client: agw-client
  create_client "agw-client" "agw-client-secret" "false" "true"

  # Public client: agw-client-public (PKCE)
  create_client "agw-client-public" "" "true" "false"

  # Budget management client
  create_client "budget-management" "budget-management-secret" "false" "true"

  log_ok "Clients created"
}

create_client() {
  local client_id=$1
  local client_secret=$2
  local is_public=$3
  local service_accounts=$4

  # Check if client exists
  local existing=$(curl -s -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "${KEYCLOAK_URL}/admin/realms/${REALM}/clients?clientId=${client_id}" | grep -o '"id"')

  if [ -n "$existing" ]; then
    echo "  Client '${client_id}' already exists, skipping"
    return 0
  fi

  local payload='{
    "clientId": "'"${client_id}"'",
    "enabled": true,
    "publicClient": '"${is_public}"',
    "standardFlowEnabled": true,
    "directAccessGrantsEnabled": true,
    "serviceAccountsEnabled": '"${service_accounts}"',
    "redirectUris": ["*"],
    "webOrigins": ["*"],
    "attributes": {
      "access.token.signed.response.alg": "RS256",
      "id.token.signed.response.alg": "RS256",
      "post.logout.redirect.uris": "*"'

  # Add PKCE for public clients
  if [ "$is_public" = "true" ]; then
    payload="${payload}"', "pkce.code.challenge.method": "S256"'
  fi

  payload="${payload}"'}}'

  # Add secret for confidential clients
  if [ -n "$client_secret" ]; then
    payload=$(echo "$payload" | sed 's/}$/,"secret": "'"${client_secret}"'"}/')
  fi

  # Create client
  curl -s -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/clients" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$payload" > /dev/null

  # Get client UUID for adding mappers
  local client_uuid=$(curl -s -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "${KEYCLOAK_URL}/admin/realms/${REALM}/clients?clientId=${client_id}" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

  # Add protocol mappers for custom claims
  for attr in group org_id team_id is_org; do
    curl -s -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/clients/${client_uuid}/protocol-mappers/models" \
      -H "Authorization: Bearer ${ACCESS_TOKEN}" \
      -H "Content-Type: application/json" \
      -d '{
        "name": "'"${attr}"'",
        "protocol": "openid-connect",
        "protocolMapper": "oidc-usermodel-attribute-mapper",
        "config": {
          "user.attribute": "'"${attr}"'",
          "claim.name": "'"${attr}"'",
          "id.token.claim": "true",
          "access.token.claim": "true",
          "userinfo.token.claim": "true",
          "jsonType.label": "String"
        }
      }' > /dev/null
  done

  echo "  Created client '${client_id}'"
}

#===============================================================================
# Step 7: Create Users
#===============================================================================
create_users() {
  log_step 7 "Creating users..."

  # org-admin user
  create_user "org-admin" "orgadmin@solo.io" "Org" "Admin" "admins" "acme-corp" "" "true"

  # user1
  create_user "user1" "user1@solo.io" "Joe" "Blogg" "users" "acme-corp" "team-alpha" "false"

  # user2
  create_user "user2" "user2@solo.io" "Bob" "Doe" "users" "acme-corp" "team-alpha" "false"

  # team-user
  create_user "team-user" "teamuser@solo.io" "Team" "User" "users" "acme-corp" "team-beta" "false"

  log_ok "Users created"
}

create_user() {
  local username=$1
  local email=$2
  local first_name=$3
  local last_name=$4
  local group=$5
  local org_id=$6
  local team_id=$7
  local is_org=$8
  local password="Passwd00"

  # Check if user exists
  local existing=$(curl -s -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "${KEYCLOAK_URL}/admin/realms/${REALM}/users?username=${username}&exact=true" | grep -o '"id"')

  if [ -n "$existing" ]; then
    echo "  User '${username}' already exists, skipping"
    return 0
  fi

  # Build attributes
  local attributes='"group": ["'"${group}"'"], "org_id": ["'"${org_id}"'"], "is_org": ["'"${is_org}"'"]'
  if [ -n "$team_id" ]; then
    attributes="${attributes}"', "team_id": ["'"${team_id}"'"]'
  fi

  # Create user
  curl -s -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/users" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "username": "'"${username}"'",
      "email": "'"${email}"'",
      "firstName": "'"${first_name}"'",
      "lastName": "'"${last_name}"'",
      "enabled": true,
      "emailVerified": true,
      "attributes": {'"${attributes}"'},
      "credentials": [{
        "type": "password",
        "value": "'"${password}"'",
        "temporary": false
      }]
    }' > /dev/null

  echo "  Created user '${username}'"
}

#===============================================================================
# Main
#===============================================================================
main() {
  echo ""
  echo "=========================================="
  echo "  Keycloak Installation"
  echo "=========================================="
  echo ""

  create_namespace
  deploy_postgres
  deploy_keycloak
  wait_for_keycloak
  get_admin_token
  create_realm
  create_clients
  create_users

  echo ""
  echo "=========================================="
  echo -e "${GREEN}  Keycloak installed successfully!${NC}"
  echo "=========================================="
  echo ""
  echo "  Access Keycloak:"
  echo "    kubectl port-forward -n ${NAMESPACE} svc/keycloak 8080:8080"
  echo ""
  echo "  Admin Console:"
  echo "    URL:      http://localhost:8080/admin"
  echo "    Username: ${ADMIN_USER}"
  echo "    Password: ${ADMIN_PASS}"
  echo ""
  echo "  Realm: ${REALM}"
  echo ""
  echo "  Clients:"
  echo "    - agw-client (confidential) / agw-client-secret"
  echo "    - agw-client-public (public, PKCE)"
  echo "    - budget-management / budget-management-secret"
  echo ""
  echo "  Users (password: Passwd00):"
  echo "    - org-admin (admins, org: acme-corp)"
  echo "    - user1 (users, org: acme-corp, team: team-alpha)"
  echo "    - user2 (users, org: acme-corp, team: team-alpha)"
  echo "    - team-user (users, org: acme-corp, team: team-beta)"
  echo ""
}

main "$@"
