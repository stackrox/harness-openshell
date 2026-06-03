# Sandbox Agent Instructions

You are running inside an OpenShell sandbox on an OpenShift cluster. Credentials are injected via the OpenShell provider system — they appear as environment variables automatically.

## Tools Available

### GitHub — `gh` CLI
- Pre-authenticated. Use `gh` for all GitHub operations.
- Examples: `gh repo clone`, `gh pr create`, `gh issue list`, `gh api`

### Jira & Confluence — mcp-atlassian MCP server
- Connected via the `atlassian` MCP server (credentials injected by provider).
- Use MCP tools for Jira searches, issue creation, comments, and Confluence page reads.

### Google Workspace — `gws` CLI
- Pre-authenticated for Gmail, Calendar, Drive, Docs, Sheets.
- Path: `/usr/local/bin/gws`
- Examples:
  - `gws gmail users messages list --params '{"userId": "me", "maxResults": 5}'`
  - `gws calendar events list --params '{"calendarId": "primary", "maxResults": 5}'`
  - `gws drive files list --params '{"pageSize": 10}'`
- Use `gws schema <service.resource.method>` to discover API parameters.

### Kubernetes — `kubectl`
- A deploy kubeconfig may be available at `/tmp/deploy-kubeconfig` for deploying to test namespaces.
- Do NOT modify the `openshell` or `agent-sandbox-system` namespaces.

### General Tools
- `python3`, `pip`, `uv` — Python 3.14 with a virtualenv at `/sandbox/.venv`
- `node`, `npm` — Node.js 22
- `git`, `curl` — pre-installed
- `cargo` — NOT available (no Rust toolchain in sandbox)

## Configuration
- Running via **Vertex AI** through `inference.local` gateway routing.
- Model selection is configured at the gateway level via `openshell inference set`.

## Conventions
- Working directory: `/sandbox`
- Writable paths: `/sandbox`, `/tmp`
- Network: Outbound allowed to Google APIs, GitHub, Atlassian, npm/pypi.
- Credentials are managed by the OpenShell provider system and cleaned up on sandbox exit.
