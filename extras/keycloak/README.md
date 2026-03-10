# Keycloak Installation

Lightweight Keycloak setup for demo and development purposes. Deploys Keycloak with PostgreSQL to a Kubernetes cluster and configures a realm with pre-defined clients and users.

## Quick Start

```bash
# Install Keycloak
./install.sh

# Access Keycloak Admin Console
kubectl port-forward -n keycloak svc/keycloak 8080:8080
# Open http://localhost:8080/admin (admin/admin)

# Uninstall
./uninstall.sh
```

## Configuration

Override defaults via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KEYCLOAK_NAMESPACE` | `keycloak` | Kubernetes namespace |
| `KEYCLOAK_REALM` | `agw-dev` | Realm name |
| `KEYCLOAK_ADMIN_USER` | `admin` | Admin username |
| `KEYCLOAK_ADMIN_PASS` | `admin` | Admin password |

Example:
```bash
KEYCLOAK_REALM=my-realm ./install.sh
```

## Pre-configured Resources

### Clients

| Client ID | Type | Secret | Notes |
|-----------|------|--------|-------|
| `agw-client` | Confidential | `agw-client-secret` | For server-side apps |
| `agw-client-public` | Public | - | PKCE enabled, for SPAs |
| `budget-management` | Confidential | `budget-management-secret` | For budget service |

All clients include custom claim mappers for: `group`, `org_id`, `team_id`, `is_org`

### Users

| Username | Email | Group | org_id | team_id | is_org |
|----------|-------|-------|--------|---------|--------|
| `org-admin` | orgadmin@solo.io | admins | acme-corp | - | true |
| `user1` | user1@solo.io | users | acme-corp | team-alpha | false |
| `user2` | user2@solo.io | users | acme-corp | team-alpha | false |
| `team-user` | teamuser@solo.io | users | acme-corp | team-beta | false |

**Password for all users:** `Passwd00`

## Getting Tokens

### Password Grant (for testing)

```bash
# Get token for user1
curl -s -X POST "http://localhost:8080/realms/agw-dev/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password" \
  -d "client_id=agw-client" \
  -d "client_secret=agw-client-secret" \
  -d "username=user1" \
  -d "password=Passwd00" | jq -r '.access_token'
```

### Client Credentials Grant

```bash
curl -s -X POST "http://localhost:8080/realms/agw-dev/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=agw-client" \
  -d "client_secret=agw-client-secret" | jq -r '.access_token'
```

## OIDC Endpoints

| Endpoint | URL |
|----------|-----|
| Issuer | `http://localhost:8080/realms/agw-dev` |
| Authorization | `http://localhost:8080/realms/agw-dev/protocol/openid-connect/auth` |
| Token | `http://localhost:8080/realms/agw-dev/protocol/openid-connect/token` |
| JWKS | `http://localhost:8080/realms/agw-dev/protocol/openid-connect/certs` |
| UserInfo | `http://localhost:8080/realms/agw-dev/protocol/openid-connect/userinfo` |

## What Gets Deployed

- **Namespace:** `keycloak`
- **PostgreSQL:** Single replica with in-memory storage (data lost on restart)
- **Keycloak:** Development mode (`start-dev`), no TLS
- **Service:** LoadBalancer on port 8080
