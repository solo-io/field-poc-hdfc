# Native Cost Control, Budgets & Fine-Grained Authorization

Demonstrates cost and access control built entirely on **native Enterprise Agentgateway CRDs** — no custom ext-proc service or database required for budgets, model pricing, or rate limits. It combines model cost tracking, hierarchical budget enforcement, virtual API keys, global burst rate limiting, and OpenFGA-based fine-grained authorization on a single set of routes.

> This is a different approach from [`03-cost-control/`](../03-cost-control/), which uses a custom ext-proc microservice backed by PostgreSQL for budgets and pricing. This scenario uses the built-in `EnterpriseAgentgatewayBudget` and `EnterpriseAgentgatewayParameters` CRDs instead — simpler to operate, at the cost of the custom service's approval workflows, audit log, and management UI.

## Components

| Component | Description |
|-----------|--------------|
| **agentgateway** | Routes chat requests, evaluates budgets, applies rate limits, and calls ext_authz |
| **Keycloak** | Optional JWT issuer — auth is `Optional` in this scenario, not `Strict` |
| **openfga-ext-authz** | gRPC `ext_authz` service bridging to OpenFGA `Check` API ([source](https://github.com/day0ops/openfga-ext-authz)) |
| **OpenFGA** | ReBAC authorization server |

No PostgreSQL, no additional management service — budgets, pricing, virtual keys, and rate limits all live in Kubernetes CRDs and Secrets.

## Features Demonstrated

### Model Cost Catalog

A `ConfigMap` (`model-costs-catalog`) holds per-model input/output/cache token rates, referenced by an `EnterpriseAgentgatewayParameters` resource (`model-costs-params`). The catalog must then be attached to the Gateway:

```bash
kubectl patch gateway agentgateway-gw -n agentgateway-system --type=merge -p \
  '{"spec":{"infrastructure":{"parametersRef":{"name":"model-costs-params","group":"enterpriseagentgateway.solo.io","kind":"EnterpriseAgentgatewayParameters"}}}}'
```

This step can't be expressed as a plain manifest apply since it patches the existing `Gateway` object's `spec.infrastructure.parametersRef`.

### Budget Enforcement

`EnterpriseAgentgatewayBudget/human-budgets` defines hierarchical, subject-scoped budgets evaluated on every request:

| Name | Subject | Limit | Window | On Exceeded |
|------|---------|-------|--------|-------------|
| `acme-org-monthly-ceiling` | `org_id: acme` | 200 USD | Month | Audit |
| `acme-ml-team-monthly-cap` | `team_id: acme-ml` | 50 USD | Month | Block |
| `alice-daily-cap` | `username: alice` | 5 USD | Day | Block |
| `bob-daily-cap` | `username: bob` | 10 USD | Day | Block |
| `team-ci-daily-cap` | `virtualKey: team-ci` | 1 Token | Day | Block |

`entBudgetEnforcement: {}` on the route policy is what activates enforcement against these budgets.

### Virtual Keys & Per-Key Rate Limiting

A `virtual-keys` Secret defines a `team-ci` API key. `apiKeyAuthentication` (mode `Optional`) accepts it via the `X-Api-Key` header, and `virtual-keys-ratelimit` caps `team-ci` at 5000 requests/hour via `entRateLimit`.

### Optional JWT + API Key Auth with Fallback

Unlike [`07-openfga-authz/`](../07-openfga-authz/), JWT authentication here is `Optional`, not `Strict`. The identity header is derived with a fallback:

```yaml
value: coalesce(jwt['preferred_username'], apiKey['user_id'])
```

A caller may authenticate with either a Keycloak JWT or a virtual API key; `x-user-id` resolves to whichever is present.

### Global Burst Rate Limiting

`chat-burst-limit` caps the entire `providers-chat-route` at 20,000 requests/minute, independent of the per-virtual-key limit above.

### OpenFGA Fine-Grained Authorization

Same `openfga-ext-authz` integration as [`07-openfga-authz/`](../07-openfga-authz/) — see that scenario for OpenFGA store/model/tuple setup instructions. `OPENFGA_STORE_ID` and `OPENFGA_MODEL_ID` must be set in [`config/config.yaml`](./config/config.yaml) before deploying.

## Prerequisites

- Keycloak running with the `agw-dev` realm configured — see [extras/keycloak/README.md](../extras/keycloak/README.md) (optional, only needed to test the JWT auth path)
- An OpenFGA server, store, model, and tuples — see [`07-openfga-authz/README.md`](../07-openfga-authz/README.md#setting-up-openfga)
- Provider credentials for OpenAI and Anthropic

## Deployment

```bash
# 1. Set provider credentials in config/config.yaml
# Replace: <set OPENAI_API_KEY> and <set ANTHROPIC_API_KEY>

# 2. Set OPENFGA_STORE_ID and OPENFGA_MODEL_ID in config/config.yaml

# 3. Apply the configuration
kubectl apply -f config/config.yaml

# 4. Attach the model cost catalog to the Gateway
kubectl patch gateway agentgateway-gw -n agentgateway-system --type=merge -p \
  '{"spec":{"infrastructure":{"parametersRef":{"name":"model-costs-params","group":"enterpriseagentgateway.solo.io","kind":"EnterpriseAgentgatewayParameters"}}}}'
```

## Testing

```bash
# Port-forward the gateway
kubectl port-forward -n agentgateway-system svc/agentgateway 8080:8080

# Request using a virtual key (team-ci) instead of a JWT
curl -X POST http://localhost:8080/chat \
  -H "X-Api-Key: sk-team-ci-demo-9f3c7a1e" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "Hello!"}]}'

# team-ci's daily budget is capped at 1 token -> the next request is Blocked
curl -i -X POST http://localhost:8080/chat \
  -H "X-Api-Key: sk-team-ci-demo-9f3c7a1e" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "Hello again!"}]}'
```

## Key Policies

### Optional Auth with Identity Fallback (Gateway-wide)

```yaml
transformation:
  request:
    set:
      - name: x-user-id
        value: coalesce(jwt['preferred_username'], apiKey['user_id'])
jwtAuthentication:
  mode: Optional
  providers: [...]
apiKeyAuthentication:
  mode: Optional
  secretRef:
    name: virtual-keys
  location:
    header:
      name: X-Api-Key
entRateLimit:
  global:
    rateLimitConfigRefs:
      - name: virtual-keys-ratelimit
```

### Budget + Rate Limit + ext_authz (Route-scoped)

```yaml
extAuth:
  backendRef:
    name: openfga-ext-authz
    port: 9001
  grpc: {}
entBudgetEnforcement: {}
entRateLimit:
  global:
    rateLimitConfigRefs:
      - name: chat-burst-limit
```

## Reference Links

- [openfga-ext-authz](https://github.com/day0ops/openfga-ext-authz) — the ext_authz gRPC service used in this scenario
- [OpenFGA documentation](https://openfga.dev/docs)
- [Agentgateway documentation](https://agentgateway.dev/docs/standalone/main/)
