# Providers Configuration Spec

Two files define the harness configuration:

- **`providers.toml`** — catalog of provider definitions with their inputs (checked into repo)
- **`openshell.toml`** — deployment config selecting which providers to enable (per-user)

## providers.toml

### Provider entry

Each provider is a `[[providers]]` array entry with these fields:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Unique identifier (used in openshell.toml to enable) |
| `type` | yes | `"openshell"` (registered with gateway) or `"custom"` (harness workaround) |
| `description` | yes | Human-readable description shown in preflight |
| `required` | no | If `true`, preflight `--strict` fails when inputs are missing. Default: `false` |
| `method` | no | Registration method (e.g., `"from-gcloud-adc"`) |
| `upstream` | custom only | Link to upstream issue that would make this provider native |

### Input entry

Each provider has an `[[providers.inputs]]` sub-array. Each input has:

| Field | Required | Description |
|-------|----------|-------------|
| `key` | yes | Env var name, file path, or shell command |
| `kind` | no | `"env"` (default), `"file"`, or `"check"` |
| `secret` | no | If `true`, value is masked in preflight output and delivered via the provider system. If `false` (default), env vars are written to sandbox.env for the sandbox to source. |

### Input kinds

- **`env`** — environment variable. Preflight checks if set, shows value (or masked if `secret = true`)
- **`file`** — filesystem path. Preflight checks existence, extracts metadata from known formats (ADC project, GWS client_id)
- **`check`** — shell command. Preflight runs it and reports pass/fail

### Example

```toml
[[providers]]
name = "github"
type = "openshell"

description = "GitHub API and git operations (read-only)"
required = true
inputs = [
  { key = "GITHUB_TOKEN", kind = "env", secret = true, description = "GitHub PAT" },
]

[[providers]]
name = "vertex-local"
type = "openshell"

method = "from-gcloud-adc"
description = "Google Vertex AI inference via gateway-managed OAuth"
required = true
inputs = [
  { key = "ANTHROPIC_VERTEX_PROJECT_ID", kind = "env", description = "GCP project ID" },
  { key = "CLOUD_ML_REGION", kind = "env", description = "Vertex AI region" },
  { key = "~/.config/gcloud/application_default_credentials.json", kind = "file", description = "GCP ADC" },
]

[[providers]]
name = "atlassian"
type = "openshell"

description = "Jira and Confluence (read-only, Basic auth resolved by proxy)"
inputs = [
  { key = "JIRA_API_TOKEN", kind = "env", secret = true, description = "Atlassian API token" },
]

[[providers]]
name = "atlassian-config"
type = "custom"
description = "Atlassian URL and username (non-secret, uploaded to sandbox)"
inputs = [
  { key = "JIRA_URL", kind = "env", description = "Atlassian site URL" },
  { key = "JIRA_USERNAME", kind = "env", description = "Atlassian username" },
]

[[providers]]
name = "gws"
type = "custom"
description = "Google Workspace (Gmail, Calendar, Drive)"
upstream = "https://github.com/NVIDIA/OpenShell/issues/1268"
inputs = [
  { key = "~/.config/gws/client_secret.json", kind = "file", description = "GWS OAuth client config" },
  { key = "gws auth status", kind = "check", description = "GWS authentication" },
]
```

## openshell.toml

Selects which providers from the catalog to enable:

```toml
providers = ["github", "vertex-local", "atlassian"]
providers-custom = ["gws", "atlassian-config"]

[sandbox]
image = "quay.io/rcochran/openshell:sandbox"
command = "claude --bare"

[inference]
model = "claude-sonnet-4-6"
```

If `openshell.toml` is absent, all providers are enabled.

## Preflight behavior

`openshell-harness-preflight.sh` reads both files and for each enabled provider:

1. Checks all inputs (env vars set? files exist? commands pass?)
2. Prints status per input with details:
   - `env` + `secret = true` → masked value (`ghp_***`)
   - `env` + `secret = false` → full value
   - `file` → existence + extracted metadata (project ID, client ID)
   - `check` → pass/fail
3. Reports provider as ✓ (all inputs present), ✗ (required + missing), or - (optional + missing)
4. With `--strict`, exits non-zero if any required provider has missing inputs
