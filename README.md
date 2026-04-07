# Customer POC Runbook

This is a proof-of-concept for a customer POC. It is not intended to be a production-ready solution.

## POC Scenarios

| # | Scenario | Key Feature(s) | Status | Directory |
|---|----------|----------------|--------|-----------|
| 1 | [LLM Body-Based Routing](#1-llm-body-based-routing) | Body-based routing, multi-provider dispatch | ✅ Ready | [`01-llm-routing/`](./01-llm-routing/) |
| 2 | [Prompt Guarding](#2-prompt-guarding) | Webhook guard + Opik evaluation & tracing | ✅ Ready | [`02-prompt-guard/`](./02-prompt-guard/) |
| 3 | [Cost Control & Rate Limits](#3-cost-control--rate-limits) | Cost control, budget management, ext-proc enforcement | ✅ Ready | [`03-cost-control/`](./03-cost-control/) |
| 4 | [Authentication & RBAC](#4-authentication--rbac) | OIDC/Keycloak, JWT auth, workload identity, CEL RBAC | ✅ Ready | [`04-auth/`](./04-auth/) |
| 5 | [Microsoft Entra ID](#5-microsoft-entra-id) | Azure AD / Entra ID integration | 🔜 Planned | [`05-entra-id/`](./05-entra-id/) |
| 6 | [Observability](#6-observability) | Grafana dashboards, cost estimation, budget monitoring | ✅ Ready | [`06-observability/`](./06-observability/) |

---

## Prerequisites

All scenarios run on **Kubernetes** with **Enterprise Agentgateway** installed.

- Kubernetes cluster with the `enterprise-agentgateway` `GatewayClass` available
- `kubectl` configured against the target cluster
- Provider credentials (OpenAI, Vertex AI, etc.) available as environment variables for secret creation

---

## Scenario Summaries

### 1. LLM Body-Based Routing

Route requests to different LLM backends based on the `model` field in the request body. An `EnterpriseAgentgatewayPolicy` promotes the model name into a header at `PreRouting`; standard `HTTPRoute` header-match rules dispatch to the correct backend. The following configs are provided under [`01-llm-routing/config/`](./01-llm-routing/config/):

- **`bbr.yaml`** — Chat completions: Vertex AI (Gemini 2.5 Flash) vs local Ollama (Llama 3) on `/chat`
- **`bbr-embeddings.yaml`** — Embeddings: OpenAI (`text-embedding-3-small`) vs local Ollama (`bge-m3`) on `/embeddings`
- **`bbr-single-endpoint.yaml`** — Single-endpoint routing: chat and embeddings backends (Vertex AI, Ollama, OpenAI) unified under a single `/common` endpoint with body-based model dispatch
- **`routing-stt.yaml`** — OpenAI speech-to-text (`whisper-1`): `/v1/audio/transcriptions` passthrough; clients call `/openai` on the gateway (prefix rewrite to OpenAI)
- **`routing-tts.yaml`** — OpenAI text-to-speech (`gpt-4o-mini`): `/v1/audio/speech` passthrough; clients call `/openai` on the gateway (prefix rewrite to OpenAI)

→ [`01-llm-routing/`](./01-llm-routing/)

### 2. Prompt Guarding

Inspect every prompt before it reaches the LLM and every response before it returns to the client. A custom FastAPI webhook server (backed by [Opik](https://www.comet.com/site/products/opik/) for evaluation and tracing) implements the Solo.io Guardrail Webhook API. Checks applied in order:

**Request:** toxic phrases → banned words → Opik Sentiment → Opik Tone → PII masking

**Response:** PII masking → Opik Sentiment masking

→ [`02-prompt-guard/`](./02-prompt-guard/)

### 3. Cost Control & Rate Limits

Budget management service with ext-proc enforcement at the gateway. Components include:

- **Budget Management Service** — ext-proc that calculates costs and enforces budgets in real-time
- **PostgreSQL** — Stores budget definitions, usage records, and model pricing
- **OIDC-Protected UI** — Web interface for managing budgets and viewing usage

**Features:**
- Real-time cost tracking (token usage → USD)
- Budget enforcement with request blocking when exhausted
- Hierarchical budgets (org-level, team-level)
- Warning thresholds (default: 80%)
- Prometheus metrics for monitoring

→ [`03-cost-control/`](./03-cost-control/)

### 4. Authentication & RBAC

Demonstrates OIDC-based authentication using **Keycloak** as the identity provider, with a two-hop workload identity chain where every agent authenticates **as itself** at each gateway boundary. Each workload exchanges its auto-mounted Kubernetes ServiceAccount JWT for a Keycloak access token via RFC 8693 token exchange — **no long-lived secrets**, no token delegation.

**Architecture:**
- **Hop 1:** Caller Agent → AGW → Stock Agent (`azp=chain-caller-agent`)
- **Hop 2:** Stock Agent → AGW → MCP (`azp=chain-stock-agent`)

**Key Features:**
- OIDC integration with Keycloak for token issuance and validation
- JWT authentication enforced at each HTTPRoute boundary
- CEL-based tool-level RBAC on MCP backend (`jwt.azp == "chain-stock-agent" && mcp.tool.name == "get_stock_price"`)
- Kubernetes ServiceAccount → OIDC token exchange (RFC 8693)
- Blast radius isolation — compromised caller cannot reach MCP directly
- Independent audit trails per hop

→ [`04-auth/`](./04-auth/)

### 5. Microsoft Entra ID

> 🔜 **Planned** — directory is empty.

Intended coverage: gateway-level authentication using **Microsoft Entra ID** (formerly Azure AD) as the enterprise identity provider. Demonstrates JWT validation with Entra ID-issued tokens, Azure Workload Identity integration, and role-based access control using Entra ID groups/roles.

→ [`05-entra-id/`](./05-entra-id/)

### 6. Observability

Pre-built Grafana dashboards for monitoring Agent Gateway:

| Dashboard | Description |
|-----------|-------------|
| **Overview** | Request rates, latency percentiles, error rates, token usage by model |
| **Performance** | CPU, memory, connections, Tokio runtime stats, network bandwidth |
| **Control Plane** | Replicas, restarts, XDS auth success rate, reconciliation status |
| **Cost Estimation** | Token-based cost analysis, projected monthly costs by model |
| **Budget Enforcement** | Denials, utilization, remaining budget, decisions by entity |

→ [`06-observability/`](./06-observability/)

---

## Reference Links

- [Agentgateway documentation](https://agentgateway.dev/docs/standalone/main/)
- [Configuration schema](https://agentgateway.dev/schema/config)
- [CEL expression reference](https://agentgateway.dev/docs/standalone/main/reference/cel)
- [Supported LLM providers](https://agentgateway.dev/docs/standalone/main/)
