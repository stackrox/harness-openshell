# Credentials

How credentials flow from your machine into sandboxes.

## Overview

| Credential | Provider type | Mechanism | Setup |
|------------|--------------|-----------|-------|
| GitHub token | `github` (built-in) | Bearer auth, proxy-resolved | `setup-providers.sh` |
| Vertex AI | `google-vertex-ai` (built-in) | `inference.local` gateway routing, OAuth refresh | `setup-providers.sh` |
| Atlassian API token | `atlassian` (custom profile) | Basic auth, proxy decodes base64 + resolves | `setup-providers.sh` |
| Atlassian URL/username | — | Uploaded as `atlassian.json` (not secrets) | `sandbox-podman.sh` / `sandbox-ocp.sh` |
| GWS OAuth | — | Decrypted export uploaded as files | `sandbox-podman.sh` / `setup-creds.sh` |

## How it works

Providers v2 (`providers_v2_enabled = true`) manages API tokens. The gateway
stores credentials and injects opaque placeholder tokens into sandboxes. The
sandbox proxy resolves placeholders in HTTP headers at request time — the
sandbox process never sees real values.

**Inference routing:** Claude Code sends requests to `https://inference.local`.
The gateway proxies to Vertex AI using the `vertex-local` provider's
credentials. No Anthropic API key needed — `ANTHROPIC_API_KEY` is a dummy.

**Basic auth (Atlassian):** The proxy decodes the base64 Basic auth header,
resolves the placeholder token inside, and re-encodes.

## Access control

- **GitHub:** read-only at proxy level (`access: read-only` in profile). Push requires `openshell policy set` with explicit `git-receive-pack` rules.
- **Atlassian:** read-only via `READ_ONLY_MODE=true` in mcp-atlassian config.

## Rotation

```bash
# GitHub — update provider
openshell provider update github --credential GITHUB_TOKEN="ghp_new"

# Vertex AI — re-auth and recreate
gcloud auth application-default login
./setup-providers.sh --force

# Atlassian — update provider + re-export env vars
openshell provider update atlassian --credential JIRA_API_TOKEN="new"

# GWS — re-auth, new sandboxes get fresh export
gws auth login
```

## Revocation

| Credential | Revoke at |
|------------|-----------|
| GCP ADC | `gcloud auth application-default revoke` |
| Atlassian | https://id.atlassian.com/manage-profile/security/api-tokens |
| GitHub | https://github.com/settings/tokens |
| GWS OAuth | https://myaccount.google.com/permissions |
