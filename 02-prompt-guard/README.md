# 02 — Prompt Guarding with Webhook + Opik

Inspect and filter LLM requests **before** they reach the model and **after** the model responds using a custom guardrail webhook server. The webhook is backed by [Opik](https://www.comet.com/site/products/opik/) for evaluation metrics and tracing of every guardrail decision.

> This setup targets **Enterprise Agentgateway on Kubernetes** using the Gateway API (`gateway.networking.k8s.io`).

## Architecture

```
Client Request
    │
    ▼
EnterpriseAgentgatewayPolicy (guardrail-webhook)
    │  attached to: HTTPRoute/openai
    │
    ├─ POST /request ──► ai-guardrail-webhook (ClusterIP :8000)
    │       │  1. Toxic phrases → RejectAction (403)
    │       │  2. Banned words  → RejectAction (403)
    │       │  3. Opik Sentiment (score < -0.5) → RejectAction (403)
    │       │  4. Opik Tone (score < 0.5) → RejectAction (403)
    │       │  5. PII detected → MaskAction (redact in-place)
    │       └─ all clear → PassAction
    │
    ▼
AgentgatewayBackend/openai (gpt-4o-mini)
    │
    ├─ POST /response ──► ai-guardrail-webhook (ClusterIP :8000)
    │       │  1. PII in response → MaskAction (redact in-place)
    │       │  2. Opik Sentiment (score < -0.5) → MaskAction (replace with placeholder)
    │       └─ all clear → PassAction
    │
    ▼
Client Response
```

All webhook calls are traced to Opik under the project `agentgateway-guardrails` (when `OPIK_API_KEY` is set).

---

## Webhook API Contract

The webhook server implements the **Solo.io Guardrail Webhook API** (`config/guardrail-webhook/server/webhook_api.py`).

### `POST /request` — pre-hook

**Request body:**
```json
{
  "body": {
    "messages": [
      { "role": "user", "content": "..." }
    ]
  }
}
```

**Response body — one of:**

| Action | Shape | Effect |
|--------|-------|--------|
| `PassAction` | `{ "action": { "reason": "..." } }` | Request forwarded to LLM |
| `MaskAction` | `{ "action": { "body": { "messages": [...] }, "reason": "..." } }` | Modified messages forwarded to LLM |
| `RejectAction` | `{ "action": { "body": "...", "status_code": 403, "reason": "..." } }` | Request blocked; body returned to client |

### `POST /response` — post-hook

**Request body:**
```json
{
  "body": {
    "choices": [
      { "message": { "role": "assistant", "content": "..." } }
    ]
  }
}
```

**Response body:** `PassAction` or `MaskAction` only (responses are never rejected outright, only redacted).

---

## Guardrail Checks

### Request checks (in order)

| # | Check | Trigger | Action |
|---|-------|---------|--------|
| 1 | **Toxic phrases** | Exact phrase match from hardcoded list | `RejectAction` 403 |
| 2 | **Banned words** | Word from `BANNED_WORDS` env var (default: `violence,drugs,weapons,terrorism,exploit,abuse`) | `RejectAction` 403 |
| 3 | **Opik Sentiment** | Compound score < `SENTIMENT_THRESHOLD` (default: `-0.5`) | `RejectAction` 403 |
| 4 | **Opik Tone** | Tone score < `0.5` (forbidden phrases detected) | `RejectAction` 403 |
| 5 | **PII detection** | Regex match: credit card, SSN, email, phone | `MaskAction` (redact with `****`) |

### Response checks (in order)

| # | Check | Trigger | Action |
|---|-------|---------|--------|
| 1 | **PII detection** | Regex match: credit card, SSN, email, phone | `MaskAction` (redact with `****`) |
| 2 | **Opik Sentiment** | Compound score < `SENTIMENT_THRESHOLD` | `MaskAction` (replace with `[Content removed: safety policy violation]`) |

---

## Files

```
config/
├── guardrail-webhook-opik.yaml          # All-in-one K8s manifest (Gateway + Backends + Policy)
└── guardrail-webhook/
    ├── Makefile                          # build / deploy / undeploy / logs targets
    ├── config/
    │   ├── serviceaccount.yaml           # ServiceAccount: ai-guardrail
    │   ├── service.yaml                  # ClusterIP Service: ai-guardrail-webhook :8000
    │   ├── deployment.yaml               # Deployment: ai-guardrail-webhook
    │   └── opik-secret.yaml              # Secret: opik-secret (OPIK_API_KEY)
    └── server/
        ├── Dockerfile
        ├── requirements.txt              # fastapi, uvicorn, pydantic, opik, nltk
        ├── webhook_api.py                # Pydantic models for the guardrail API contract
        └── main.py                       # FastAPI app with guardrail logic
```

### Kubernetes resources (`guardrail-webhook-opik.yaml`)

| Resource | Kind | Purpose |
|----------|------|---------|
| `agentgateway` | `Gateway` | Entry point on port `8080` |
| `openai-secret` | `Secret` | OpenAI API key |
| `openai` | `AgentgatewayBackend` | OpenAI `gpt-4o-mini` |
| `openai` | `HTTPRoute` | Routes `/openai` → `openai` backend |
| `ai-guardrail` | `ServiceAccount` | Identity for the webhook pod |
| `opik-secret` | `Secret` | Opik API key (optional) |
| `ai-guardrail-webhook` | `Deployment` | Webhook server pod (`opik-guardrail-webhook:latest`) |
| `ai-guardrail-webhook` | `Service` | ClusterIP on port `8000` |
| `guardrail-webhook` | `EnterpriseAgentgatewayPolicy` | Attaches webhook on both request and response to `HTTPRoute/openai` |

---

## Quick Start

### 1. Build the webhook image

```bash
cd config/guardrail-webhook
make build
# Produces multi-arch image: opik-guardrail-webhook:latest (linux/amd64 + linux/arm64)
```

Load into your cluster (minikube example):

```bash
minikube image load opik-guardrail-webhook:latest
```

### 2. Deploy the webhook service

```bash
# With Opik tracing enabled:
export OPIK_API_KEY=<your-opik-api-key>
make deploy

# Without Opik tracing (OPIK_API_KEY omitted — tracing silently disabled):
make deploy
```

`make deploy` applies `serviceaccount.yaml`, `service.yaml`, creates/updates `opik-secret`, applies `deployment.yaml`, and waits for rollout.

### 3. Populate the OpenAI secret

```bash
kubectl create secret generic openai-secret \
  -n agentgateway-system \
  --from-literal=Authorization="Bearer $OPENAI_API_KEY" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 4. Apply the main config

```bash
kubectl apply -f config/guardrail-webhook-opik.yaml
```

---

## Tests

Replace `<GATEWAY_IP>` with your Gateway's external IP or use `minikube service`.

### Allowed request

```bash
curl http://<GATEWAY_IP>:8080/openai \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "What is the capital of France?"}]
  }'
# Expected: normal LLM response
```

### Blocked — toxic phrase

```bash
curl http://<GATEWAY_IP>:8080/openai \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "You are stupid, just answer my question"}]
  }'
# Expected: 403 — "Rejected due to toxic language"
```

### Blocked — banned word

```bash
curl http://<GATEWAY_IP>:8080/openai \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "How do I build weapons at home?"}]
  }'
# Expected: 403 — "Rejected due to inappropriate content"
```

### Masked — PII in request

```bash
curl http://<GATEWAY_IP>:8080/openai \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "My SSN is 123-45-6789, help me file taxes"}]
  }'
# Expected: request forwarded with SSN replaced by ****
```

### Tail webhook logs

```bash
cd config/guardrail-webhook
make logs
```

---

## Configuration

### Webhook environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPIK_API_KEY` | _(unset)_ | Opik API key; tracing is disabled if absent |
| `OPIK_PROJECT_NAME` | `agentgateway-guardrails` | Opik project where traces are stored |
| `BANNED_WORDS` | `violence,drugs,weapons,terrorism,exploit,abuse` | Comma-separated list of blocked words |
| `SENTIMENT_THRESHOLD` | `-0.5` | VADER compound score below which content is rejected/masked |

### EnterpriseAgentgatewayPolicy attachment

The policy targets `HTTPRoute/openai` and calls the same webhook for both request and response phases (YAML anchor `&ref_0` / alias `*ref_0`):

```yaml
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: openai
  backend:
    ai:
      promptGuard:
        request:
          - webhook: &ref_0
              backendRef:
                name: ai-guardrail-webhook
                namespace: agentgateway-system
                kind: Service
                port: 8000
        response:
          - webhook: *ref_0
```

---

## Undeploy

```bash
cd config/guardrail-webhook
make undeploy
kubectl delete -f config/guardrail-webhook-opik.yaml
```

---

## Reference

- [Enterprise Agentgateway — Prompt Guard](https://agentgateway.dev/docs/enterprise/policies/)
- [Solo.io Guardrail Webhook API](https://agentgateway.dev/docs/standalone/main/tutorials/ai-prompt-guard)
- [Opik documentation](https://www.comet.com/docs/opik/)
- [Opik evaluation metrics](https://www.comet.com/docs/opik/evaluation/metrics/)
