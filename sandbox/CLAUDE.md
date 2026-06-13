# Sandbox Environment

You are running inside an OpenShell sandbox. Credentials are injected via the OpenShell provider system and appear as environment variables automatically.

## Environment

- Working directory: `/sandbox`
- Writable paths: `/sandbox`, `/tmp`
- Inference routes through the gateway proxy at `inference.local`
- Credentials are managed by OpenShell and cleaned up on sandbox exit

## Tools

- `gh` — GitHub CLI (pre-authenticated). Run `gh auth setup-git` before any git clone/push/pull to configure git credential helper.
- `gws` — Google Workspace CLI (when available). Use `gws schema <service.resource.method>` to discover API parameters.
- MCP servers (Jira, Confluence) are configured in `.mcp.json` and connected automatically.
- `python3`, `uv`, `node`, `npm`, `git`, `curl`
