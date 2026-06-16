# Langfuse Local Setup

## Prerequisites

- Langfuse running locally via `podman-compose up -d` from `/tmp/langfuse-spike/`
- API keys from Langfuse UI (Settings > API Keys)

## Environment Variables

```bash
export LANGFUSE_PUBLIC_KEY="pk-lf-5c833482-38cc-4ba9-badb-bcbd56e9be2e"
export LANGFUSE_SECRET_KEY="sk-lf-bb5f27ad-c72d-4def-8275-98e4ffecd9d9"
export LANGFUSE_BASE_URL="http://localhost:3000"

export CLAUDE_CODE_ENABLE_TELEMETRY=1
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1
export OTEL_TRACES_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=none
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:3000/api/public/otel
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic cGstbGYtNWM4MzM0ODItMzhjYy00YmE5LWJhZGItYmNiZDU2ZTliZTJlOnNrLWxmLWJiNWYyN2FkLWM3MmQtNGRlZi04Mjc1LTk4ZTRmZmVjZDlkOQ=="
export OTEL_LOG_USER_PROMPTS=1
export OTEL_LOG_TOOL_DETAILS=1
export OTEL_LOG_TOOL_CONTENT=1
export OTEL_LOG_RAW_API_BODIES=1
```

## What each variable does

| Variable | Purpose |
|---|---|
| `CLAUDE_CODE_ENABLE_TELEMETRY` | Master switch for Claude Code telemetry |
| `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA` | Required for trace export (not just metrics) |
| `OTEL_TRACES_EXPORTER=otlp` | Send span structure (interaction -> LLM request -> tool calls) |
| `OTEL_LOGS_EXPORTER=otlp` | Send full API request/response bodies (conversation content) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Langfuse OTel endpoint |
| `OTEL_EXPORTER_OTLP_HEADERS` | Basic auth (base64 of `public_key:secret_key`) |
| `OTEL_LOG_USER_PROMPTS` | Include prompt text in traces |
| `OTEL_LOG_TOOL_DETAILS` | Include tool call arguments |
| `OTEL_LOG_TOOL_CONTENT` | Include tool output |
| `OTEL_LOG_RAW_API_BODIES` | Include full API request/response JSON |

## Regenerating the auth header

```bash
echo -n "${LANGFUSE_PUBLIC_KEY}:${LANGFUSE_SECRET_KEY}" | base64
```

## Starting Langfuse

```bash
cd /tmp/langfuse-spike
podman-compose up -d
```

UI at http://localhost:3000

## Stopping Langfuse

```bash
cd /tmp/langfuse-spike
podman-compose down
```

Data persists in podman volumes across restarts.
