# Sandbox Agent Instructions

You are running inside an OpenShell sandbox. Credentials are injected via the OpenShell provider system — they appear as environment variables automatically.

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

### AI Coding Agents
- `claude` — Claude Code
- `opencode` — OpenCode (MCP config at `/sandbox/opencode.json`)

### General Tools
- `python3`, `pip`, `uv` — Python with a virtualenv at `/sandbox/.venv`
- `node`, `npm` — Node.js 22
- `git`, `curl` — pre-installed

## Configuration
- Inference routes through the gateway proxy at `inference.local`.
- Model and provider configuration are set by the harness during sandbox creation.

## Conventions
- Working directory: `/sandbox`
- Writable paths: `/sandbox`, `/tmp`
- Network: Outbound allowed to Google APIs, GitHub, Atlassian, npm/pypi.
- Credentials are managed by the OpenShell provider system and cleaned up on sandbox exit.
