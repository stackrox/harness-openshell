# Sandbox Environment

You are running inside an OpenShell sandbox based on the StackRox
`rox-ci-image` build image. Credentials are injected by OpenShell providers and
are not part of the image.

## Environment

- Working directory: `/sandbox`
- Writable paths: `/sandbox`, `/tmp`
- Inference routes through the gateway proxy at `inference.local`
- Repository build tools from the `rox-ci-image` build are available, including
  Go, compilers, make, git, jq, and the StackRox CI toolchain.

## Tools

- `gh` — GitHub CLI. Use the bundled GitHub skill for REST-only API access.
- `gws` — Google Workspace CLI when the provider is attached.
- `python3`, `uv`, `node`, `npm`, `go`, `gopls`, `git`, `curl`
- `claude`, `opencode`, `codex`, and `copilot` coding agents
- Atlassian MCP through `.mcp.json` when its provider is attached
