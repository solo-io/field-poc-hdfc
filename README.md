# Customer POC Runbook

This is a proof-of-concept for a customer POC. It is not intended to be a production-ready solution.

## POC Scenarios

| # | Scenario | Key Feature(s) | Status | Directory |
|---|----------|----------------|--------|-----------|
| 1 | [LLM Body-Based Routing](#1-llm-body-based-routing) | Body-based routing, multi-provider dispatch | ✅ Ready | [`01-llm-routing/`](./01-llm-routing/) |
| 2 | [Prompt Guarding](#2-prompt-guarding) | Webhook guard + Opik evaluation & tracing | ✅ Ready | [`02-prompt-guard/`](./02-prompt-guard/) |
| 3 | [Cost Control & Rate Limits](#3-cost-control--rate-limits) | Token-based local rate limits | 🔜 Planned | [`03-cost-control/`](./03-cost-control/) |
| 4 | [RBAC](#4-rbac) | JWT claims + CEL authorization rules | 🔜 Planned | [`04-rbac/`](./04-rbac/) |
| 5 | [Access Policies](#5-access-policies) | OIDC / Microsoft Entra ID | 🔜 Planned | [`05-access-policies/`](./05-access-policies/) |
| 6 | [Observability](#6-observability) | OpenTelemetry, Prometheus, access logging | 🔜 Planned | [`06-observability/`](./06-observability/) |

---

## Prerequisites

All scenarios run on **Kubernetes** with **Enterprise Agentgateway** installed.

- Kubernetes cluster with the `enterprise-agentgateway` `GatewayClass` available
- `kubectl` configured against the target cluster
- Provider credentials (OpenAI, Vertex AI, etc.) available as environment variables for secret creation

---

## Scenario Summaries

### 1. LLM Body-Based Routing

Route requests to different LLM backends based on the `model` field in the request body. An `EnterpriseAgentgatewayPolicy` promotes the model name into a header at `PreRouting`; standard `HTTPRoute` header-match rules dispatch to the correct backend. Two configs are provided:

- **`bbr.yaml`** — Chat completions: Vertex AI (Gemini 2.5 Flash) vs local Ollama (Llama 3) on `/chat`
- **`bbr-embeddings.yaml`** — Embeddings: OpenAI (`text-embedding-3-small`) vs local Ollama (`bge-m3`) on `/embeddings`

→ [`01-llm-routing/`](./01-llm-routing/)

### 2. Prompt Guarding

Inspect every prompt before it reaches the LLM and every response before it returns to the client. A custom FastAPI webhook server (backed by [Opik](https://www.comet.com/site/products/opik/) for evaluation and tracing) implements the Solo.io Guardrail Webhook API. Checks applied in order:

**Request:** toxic phrases → banned words → Opik Sentiment → Opik Tone → PII masking

**Response:** PII masking → Opik Sentiment masking

→ [`02-prompt-guard/`](./02-prompt-guard/)

### 3. Cost Control & Rate Limits

> 🔜 **Planned** — directory is empty.

Intended coverage: token-based rate limiting enforced at the gateway to prevent runaway billing and denial-of-wallet attacks.

→ [`03-cost-control/`](./03-cost-control/)

### 4. RBAC

> 🔜 **Planned** — directory is empty.

Intended coverage: fine-grained access control using JWT claims evaluated with CEL (Common Expression Language) to gate access to routes and models based on caller identity and role.

→ [`04-rbac/`](./04-rbac/)

### 5. Access Policies

> 🔜 **Planned** — directory is empty.

Intended coverage: gateway-level authentication using OIDC / JWT and Microsoft Entra ID (Azure AD) for both downstream caller validation and upstream provider auth.

→ [`05-access-policies/`](./05-access-policies/)

### 6. Observability

> 🔜 **Planned** — directory is empty.

Intended coverage: OpenTelemetry distributed traces with LLM-specific spans, Prometheus metrics, and structured access logging enriched with model and identity context.

→ [`06-observability/`](./06-observability/)

---

## Reference Links

- [Agentgateway documentation](https://agentgateway.dev/docs/standalone/main/)
- [Configuration schema](https://agentgateway.dev/schema/config)
- [CEL expression reference](https://agentgateway.dev/docs/standalone/main/reference/cel)
- [Supported LLM providers](https://agentgateway.dev/docs/standalone/main/)
