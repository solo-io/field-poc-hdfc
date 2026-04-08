# 06 - Observability & Telemetry Storage

This module covers how AgentGateway Enterprise routes telemetry (traces, metrics, access logs) into storage backends, and how to branch that telemetry to additional destinations like BigQuery for reporting.

## What's here

```
06-observability/
  collector-values.yaml     # OTel Collector Helm values with multi-exporter pipeline
  tracing-policy.yaml       # EnterpriseAgentgatewayPolicy: tracing with AI span attributes
  access-log-policy.yaml    # EnterpriseAgentgatewayPolicy: CEL access log enrichment
  bigquery-sink/
    README.md               # BigQuery-specific setup (log sink + schema)
```

## How it fits together

The Solo Enterprise UI Helm chart (`management`) ships with two pods that handle all telemetry:

- `solo-enterprise-telemetry-collector-0` — the OTel Collector
- `management-clickhouse-shard0-0` — columnar storage for the Solo UI's Tracing and Usage views

The proxy doesn't emit OTel data by default. Everything in this module assumes you've already installed the management chart:

```bash
helm upgrade -i management oci://us-docker.pkg.dev/solo-public/solo-enterprise-helm/charts/management \
  --namespace agentgateway-system \
  --create-namespace \
  --version 0.3.12 \
  --set cluster="mgmt-cluster" \
  --set products.agentgateway.enabled=true
```

Docs: [UI setup guide](https://docs.solo.io/agentgateway/2.2.x/install/ui/setup/)

---

## Step 1: Turn on tracing

Apply `tracing-policy.yaml` to wire the proxy to the telemetry collector:

```bash
kubectl apply -f tracing-policy.yaml
```

This creates an `EnterpriseAgentgatewayPolicy` that sends traces from `agentgateway-proxy` to `solo-enterprise-telemetry-collector:4317`. Once applied, traces start flowing into ClickHouse and appear in the Solo UI.

The policy also adds AI-specific span attributes (`llm.request_model`, `llm.input_tokens`, `llm.output_tokens`, `team`, `org`) needed for per-team queries and downstream routing.

Docs: [Configure tracing](https://docs.solo.io/agentgateway/2.2.x/install/ui/setup/#configure-tracing) | [Tracing guide](https://docs.solo.io/agentgateway/2.2.x/observability/tracing/)

---

## Step 2: Access logs (optional)

Apply `access-log-policy.yaml` to emit structured per-request logs via CEL expressions.

> **Note:** The proxy writes access logs to stdout only as of v2.2.x — no native OTLP push yet ([issue #228](https://github.com/solo-io/agentgateway-enterprise/issues/228)). To collect them you'll need an OTel Collector filelog DaemonSet scraping container stdout.

Docs: [Log CEL variables in access logs](https://docs.solo.io/agentgateway/2.2.x/traffic-management/transformations/access-logs/)

---

## Step 3: Adding exporters (the generic pattern)

The OTel Collector pipeline supports multiple exporters per signal. Adding a new destination is a matter of declaring an exporter and listing it alongside the existing ones in the pipeline. The collector fans out to all listed exporters concurrently — a failure or slowness in one doesn't block the others.

The general shape:

```yaml
config:
  exporters:
    # existing exporter (Solo UI / ClickHouse)
    otlp/solo-collector:
      endpoint: solo-enterprise-telemetry-collector:4317
      tls:
        insecure: true

    # new exporter — swap this block for any OTLP-compatible destination
    otlphttp/your-destination:
      endpoint: https://your-backend/otlp
      headers:
        Authorization: Bearer ${env:YOUR_API_KEY}
      sending_queue:
        enabled: true
      retry_on_failure:
        enabled: true

  service:
    pipelines:
      traces:
        receivers: [otlp]
        processors: [batch, memory_limiter]
        exporters: [otlp/solo-collector, otlphttp/your-destination]
      logs:
        receivers: [otlp]
        processors: [batch, memory_limiter]
        exporters: [otlp/solo-collector, otlphttp/your-destination]
```

`collector-values.yaml` shows this applied for the Opik routing use case. The BigQuery pattern below is the same idea with a different exporter.

---

## Step 4: BigQuery exporter

HDFC wants to run reporting directly in BigQuery. The recommended path on GKE is:

```
AgentGateway Proxy
    | OTLP
Solo Enterprise OTel Collector
    +-- ClickHouse --> Solo UI          (traces, usage, policy views)
    +-- googlecloud exporter
            +-- Cloud Logging           (structured logs)
            +-- Cloud Trace             (distributed traces)
            |
            v
        BigQuery Log Sink               (SQL reporting on AI gateway data)
```

### Why this path

The [`googlecloud` exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/googlecloudexporter) is part of `otel/opentelemetry-collector-contrib` (the image already used by the Solo Enterprise collector). It sends:

- Logs → Cloud Logging
- Traces → Cloud Trace
- Metrics → Cloud Monitoring

From Cloud Logging, you create a [BigQuery log sink](https://cloud.google.com/logging/docs/export/configure_export_v2) that routes log entries matching a filter into a BigQuery dataset. This gives a queryable, append-only table of every AI gateway request.

### Collector config

Add the `googlecloud` exporter to the collector Helm values:

```yaml
config:
  exporters:
    otlp/solo-collector:
      endpoint: solo-enterprise-telemetry-collector:4317
      tls:
        insecure: true

    googlecloud:
      project: hdfc-ai-platform          # your GCP project ID
      log:
        default_log_name: agentgateway   # shows up as logName in Cloud Logging
      retry_on_failure:
        enabled: true

  processors:
    batch: {}
    memory_limiter:
      check_interval: 5s
      limit_mib: 400

  service:
    pipelines:
      traces:
        receivers: [otlp]
        processors: [memory_limiter, batch]
        exporters: [otlp/solo-collector, googlecloud]
      logs:
        receivers: [otlp]
        processors: [memory_limiter, batch]
        exporters: [otlp/solo-collector, googlecloud]
      metrics:
        receivers: [otlp]
        processors: [memory_limiter, batch]
        exporters: [otlp/solo-collector, googlecloud]
```

### Auth

The collector needs access to write to Cloud Logging / Cloud Trace. On GKE the simplest approach is Workload Identity:

```bash
# Create a GCP service account with the right roles
gcloud iam service-accounts create agentgateway-otel \
  --project=hdfc-ai-platform

gcloud projects add-iam-policy-binding hdfc-ai-platform \
  --member="serviceAccount:agentgateway-otel@hdfc-ai-platform.iam.gserviceaccount.com" \
  --role="roles/logging.logWriter"

gcloud projects add-iam-policy-binding hdfc-ai-platform \
  --member="serviceAccount:agentgateway-otel@hdfc-ai-platform.iam.gserviceaccount.com" \
  --role="roles/cloudtrace.agent"

# Bind to the Kubernetes service account used by the collector
gcloud iam service-accounts add-iam-policy-binding \
  agentgateway-otel@hdfc-ai-platform.iam.gserviceaccount.com \
  --role="roles/iam.workloadIdentityUser" \
  --member="serviceAccount:hdfc-ai-platform.svc.id.goog[agentgateway-system/solo-enterprise-telemetry-collector]"
```

Then annotate the collector's Kubernetes service account:

```bash
kubectl annotate serviceaccount solo-enterprise-telemetry-collector \
  -n agentgateway-system \
  iam.gke.io/gcp-service-account=agentgateway-otel@hdfc-ai-platform.iam.gserviceaccount.com
```

### BigQuery log sink

Once logs are in Cloud Logging, create a sink to route them to BigQuery:

```bash
# Create the BigQuery dataset
bq --project_id=hdfc-ai-platform mk --dataset ai_gateway_logs

# Create the log sink
gcloud logging sinks create agentgateway-bq \
  bigquery.googleapis.com/projects/hdfc-ai-platform/datasets/ai_gateway_logs \
  --project=hdfc-ai-platform \
  --log-filter='logName="projects/hdfc-ai-platform/logs/agentgateway"' \
  --use-partitioned-tables
```

BigQuery will auto-create tables from the log schema. The `--use-partitioned-tables` flag partitions by day, which keeps query costs low.

### Querying in BigQuery

Once data is flowing, the AI gateway logs are queryable as a standard BigQuery table:

```sql
-- Token usage by model and team, last 7 days
SELECT
  DATE(timestamp)                                   AS date,
  JSON_VALUE(jsonPayload.llm_request_model)         AS model,
  JSON_VALUE(jsonPayload.org)                       AS org,
  JSON_VALUE(jsonPayload.team)                      AS team,
  SUM(CAST(JSON_VALUE(jsonPayload.llm_input_tokens)  AS INT64)) AS input_tokens,
  SUM(CAST(JSON_VALUE(jsonPayload.llm_output_tokens) AS INT64)) AS output_tokens,
  COUNT(*)                                          AS requests
FROM `hdfc-ai-platform.ai_gateway_logs.agentgateway_*`
WHERE DATE(timestamp) >= DATE_SUB(CURRENT_DATE(), INTERVAL 7 DAY)
GROUP BY date, model, org, team
ORDER BY date DESC, input_tokens DESC
```

The `team` and `org` fields come from the span attributes set in `tracing-policy.yaml`. They only appear if those attributes are configured — check Step 1.

### Alternative: direct OTLP to BigQuery

There's an experimental [`googlemanagedprometheus`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/googlemanagedprometheusexporter) exporter for metrics, but for logs and traces the Cloud Logging → BigQuery sink path is the production-recommended approach on GKE. A direct OTel-to-BigQuery exporter doesn't exist in `otelcol-contrib` at this time.

---

## Span attributes reference

These attributes are set by `tracing-policy.yaml` and flow through to ClickHouse, Cloud Logging, and BigQuery:

| Attribute | Source | Notes |
|---|---|---|
| `llm.request_model` | `llm.requestModel` CEL | Model name from the request body |
| `llm.response_model` | `llm.responseModel` CEL | Model that actually responded |
| `llm.input_tokens` | `llm.inputTokens` CEL | Input token count |
| `llm.output_tokens` | `llm.outputTokens` CEL | Output token count |
| `org` | `x-org-id` header | Set by your auth layer or JWT extraction |
| `team` | `x-team-id` header | Set by your auth layer or JWT extraction |
| `http.status_code` | `response.code` CEL | HTTP response status |

Docs: [Cost tracking and token attributes](https://docs.solo.io/agentgateway/2.2.x/llm/cost-tracking/)

---

## Related modules

- [`03-cost-control`](../03-cost-control/) — rate limiting and budget enforcement using token counts
- [`04-auth`](../04-auth/) — setting `x-org-id` / `x-team-id` headers from JWT claims (needed for the `org` / `team` span attributes above)
- [`02-prompt-guard`](../02-prompt-guard/) — ExtProc webhook approach for Opik routing (alternative to OTel Collector routing)
