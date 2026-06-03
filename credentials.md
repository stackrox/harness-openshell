# Credential Flows

How credentials are configured, stored, and consumed in sandboxes.

## Providers v2

This harness uses **providers v2** (`providers_v2_enabled = true`), where
providers are profile-backed access bundles that carry both credentials and
network policy.

### How credentials flow

1. **Registration** — `openshell provider create` stores credentials in the
   gateway database, keyed by provider name and type.
2. **Attachment** — `sandbox.sh` passes `--provider` flags at sandbox creation.
3. **Placeholder injection** — the sandbox process sees opaque placeholder
   tokens, never real values.
4. **Proxy resolution** — when the sandbox makes an HTTP request, the proxy
   replaces placeholders with real values in headers, query params, and
   URL path segments.
5. **Policy composition** — each attached provider's profile contributes
   network policy entries to the sandbox at runtime.

### Credential refresh

The gateway supports automated refresh for OAuth2 tokens. The `vertex-local`
provider uses this: `--from-gcloud-adc` configures the gateway to mint and
rotate GCP access tokens from your ADC refresh token automatically.

```shell
openshell provider refresh status vertex-local   # check refresh state
openshell provider refresh rotate vertex-local \
  --credential-key GOOGLE_VERTEX_AI_TOKEN        # force immediate refresh
```

## Credentials

### GitHub (`github` provider)

| | |
|---|---|
| **Source** | `GITHUB_TOKEN` env var on host |
| **Registration** | `openshell provider create --name github --type github --credential GITHUB_TOKEN` |
| **Sandbox delivery** | Provider placeholder in env |
| **Consumption** | `gh` CLI sends `Authorization: Bearer <placeholder>` → proxy resolves |
| **Access control** | Read-only at proxy level (`access: read-only` in github profile) |

#### Multi-organization access

A single Classic PAT (`ghp_...`) covers all organizations the user belongs
to. This is the simplest approach.

**When to switch to fine-grained PATs:** If an org admin mandates them, or
you need least-privilege access scoped to specific repos. Fine-grained PATs
are scoped to a single owner/org, so multi-org access requires multiple
providers:

```shell
openshell provider create --name github-stackrox --type github \
  --credential "GITHUB_TOKEN=github_pat_stackrox_..."
openshell provider create --name github-personal --type github \
  --credential "GITHUB_TOKEN=github_pat_personal_..."
```

**When GitHub Apps make sense:** When the sandbox operates on behalf of a
team (not a person). The gateway-managed token refresh feature supports
this — GitHub App installation tokens (1-hour expiry) can be rotated
automatically.

| Approach | Scope | Expiry | OpenShell compat |
|----------|-------|--------|------------------|
| Classic PAT | All orgs user belongs to | 90 days (org-configurable) | Native via `github` provider |
| Fine-grained PAT | Single org, specific repos | Configurable | Native (same Bearer auth) |
| GitHub App | Per-org installation | 1 hour (auto-refresh) | Via gateway token refresh |

### Vertex AI (`vertex-local` provider)

Uses the native `google-vertex-ai` provider type with gateway-managed
inference routing.

| | |
|---|---|
| **Source** | `~/.config/gcloud/application_default_credentials.json` |
| **Registration** | `openshell provider create --name vertex-local --type google-vertex-ai --from-gcloud-adc --config VERTEX_AI_PROJECT_ID=<project> --config VERTEX_AI_REGION=<region>` |
| **Inference route** | `openshell inference set --provider vertex-local --model claude-sonnet-4-6-20250514 --no-verify` |
| **Sandbox delivery** | Gateway mints access tokens via OAuth refresh; sandbox sees placeholder |
| **Consumption** | Claude Code sends requests to `https://inference.local` with `ANTHROPIC_API_KEY=unused`; gateway proxies to Vertex AI with real credentials |

The sandbox never sees ADC secrets. The gateway reads the ADC file once
during provider creation, configures OAuth2 refresh, and mints short-lived
access tokens automatically.

**Credential rotation:**
```shell
gcloud auth application-default login   # refresh local ADC
openshell provider delete vertex-local  # remove old provider
openshell provider create --name vertex-local --type google-vertex-ai \
  --from-gcloud-adc --config VERTEX_AI_PROJECT_ID=<project> \
  --config VERTEX_AI_REGION=<region>
```

### Atlassian (`atlassian` provider)

Atlassian uses a split credential model:

- **`JIRA_API_TOKEN`** — provider-managed. The OpenShell proxy resolves it in Basic auth headers: it decodes `base64("username:token")`, replaces the token placeholder with the real value, and re-encodes.
- **`JIRA_URL` and `JIRA_USERNAME`** — not secrets, uploaded as plain JSON by `sandbox.sh`. Read from `atlassian.json` by `configure-mcp.py` and set as literal values in the MCP config.

| | |
|---|---|
| **Source** | `JIRA_URL`, `JIRA_USERNAME`, `JIRA_API_TOKEN` env vars on host |
| **Registration** | `setup-providers.sh` registers only `JIRA_API_TOKEN`; URL/username uploaded at sandbox launch |
| **Sandbox delivery** | Token as provider placeholder; URL/username as literals in `.claude.json` |
| **Consumption** | mcp-atlassian constructs Basic auth → proxy resolves token placeholder |
| **Access control** | Read-only via `READ_ONLY_MODE=true` in mcp-atlassian config |

### Google Workspace (file upload)

GWS credentials are encrypted files consumed locally by the `gws` CLI.
No HTTP request carries the credential.

| | |
|---|---|
| **Source** | `$GWS_CONFIG_DIR` (default: `~/.config/gws/`) |
| **Registration** | None — file upload only |
| **Sandbox delivery** | Uploaded via `--upload` at sandbox creation, copied to `/tmp/gws-config/` |
| **Consumption** | `gws` CLI reads files from `$GOOGLE_WORKSPACE_CLI_CONFIG_DIR` |

**Upstream:** When OpenShell adds file-based credential projection
([#1268](https://github.com/NVIDIA/OpenShell/issues/1268),
[#1423](https://github.com/NVIDIA/OpenShell/issues/1423)), GWS files
can move to the provider system.

## Rotation

```shell
# GitHub — update provider credential
openshell provider update github --credential GITHUB_TOKEN="ghp_new_token"

# Vertex AI — re-authenticate and recreate provider
gcloud auth application-default login
openshell provider delete vertex-local
openshell provider create --name vertex-local --type google-vertex-ai \
  --from-gcloud-adc --config VERTEX_AI_PROJECT_ID=<project> ...

# Atlassian — set env vars, launch new sandbox
export JIRA_API_TOKEN="new_token"

# GWS — re-authenticate locally, new sandboxes upload fresh files
gws auth login
```

## Revocation

| Credential | Revoke at |
|------------|-----------|
| GCP ADC | `gcloud auth application-default revoke` |
| Atlassian API token | https://id.atlassian.com/manage-profile/security/api-tokens |
| GitHub PAT | https://github.com/settings/tokens |
| GWS OAuth | https://myaccount.google.com/permissions |
