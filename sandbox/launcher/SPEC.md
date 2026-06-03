# Sandbox Launcher

In-cluster Go binary that creates OpenShell sandboxes from a YAML config.
Runs as a Kubernetes Job — `kubectl apply -f sandbox.yaml` triggers it.

## What it does

1. Read `config.yaml` from a mounted ConfigMap
2. Register the in-cluster gateway using mounted mTLS certs
3. Verify providers exist, skip any that aren't registered
4. Call `openshell sandbox create` with providers and skills config
5. Print connection instructions and exit

## Config format

Mounted at `/etc/openshell/sandbox/config.yaml`:

```yaml
name: agent
command: claude --bare
keep: true

providers:
  - github
  - vertex-local
  - atlassian

skills:
  - repo: https://github.com/stackrox/skills
  - repo: https://github.com/robbycochran/skills
    ref: v1.0
    path: claude/skills
```

## Mounted volumes

| Mount path | Source | Purpose |
|------------|--------|---------|
| `/etc/openshell/sandbox/` | ConfigMap | Sandbox config (config.yaml) |
| `/secrets/mtls/` | Secret `openshell-client-tls` | mTLS certs for gateway connection |
| `/secrets/gws/` | Secret `openshell-gws` (optional) | GWS OAuth credentials |

## Environment variables (from Job spec)

| Var | Source | Purpose |
|-----|--------|---------|
| `GATEWAY_ENDPOINT` | Job env | Gateway in-cluster address |
| `HOME` | Job env | Writable home for gateway config |
| `JIRA_URL` | Secret `openshell-atlassian` (optional) | Atlassian site URL |
| `JIRA_USERNAME` | Secret `openshell-atlassian` (optional) | Atlassian username |

## Behavior

### Gateway registration

If `/secrets/mtls/tls.crt` exists, configures the `openshell` CLI with
mTLS certs and registers the in-cluster gateway. Otherwise falls back
to insecure mode (for local dev).

### Provider detection

For each provider in the config, checks if it's registered with the
gateway (`openshell provider get <name>`). Skips unregistered providers
with a warning — doesn't fail.

### Sandbox creation

Calls `openshell sandbox create` with:
- `--name` from config
- `--no-tty` (no interactive terminal in a Job)
- `--provider` flags for each registered provider
- `--no-keep` if `keep: false`
- `-- bash -c "export SANDBOX_SKILLS_JSON=<base64>; . /sandbox/startup.sh"`

Skills are serialized as base64-encoded JSON and passed as an env var
to the sandbox's startup script. The startup script decodes it, clones
repos, and auto-detects marketplace plugins vs raw skill directories.

### Retry

Retries up to 3 times on supervisor race conditions (common on
Kubernetes where the sandbox pod isn't fully ready when the SSH
connection is attempted). Deletes and recreates the sandbox between
retries. 5-second backoff.

### Exit

Prints connection instructions and exits 0. The sandbox pod lives
independently — the Job completing doesn't affect it.

## Build

```
# Cross-compile for linux/amd64
GOOS=linux GOARCH=amd64 go build -o launcher .

# Or via Makefile
make cli-launcher
```

## Image

Minimal image with two static binaries:

```dockerfile
FROM scratch
COPY launcher /usr/local/bin/launcher
COPY openshell /usr/local/bin/openshell
ENTRYPOINT ["/usr/local/bin/launcher"]
```

No bash, no Python, no pip, no package manager. ~50MB total.

## Future

- Read skills config directly instead of passing via env var
- Support `openshell sandbox connect` for interactive sessions (needs TTY)
- Watch sandbox status and stream logs
- Support for non-GitHub skill repos (GitLab, Bitbucket)
