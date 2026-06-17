# profiles/providers/

OpenShell provider profile YAMLs. These are imported to the gateway during `harness apply` via `openshell provider profile import`.

Provider profiles define how credentials are discovered, stored, and injected into sandboxes. The format is defined by [OpenShell](https://github.com/NVIDIA/OpenShell).

## Format

```yaml
id: provider-name               # unique ID, matches the profile name in agent configs
display_name: Human Name
description: What this provider does
category: knowledge              # knowledge, inference, or tools

credentials:
  - name: credential_key         # internal name
    description: What this credential is
    env_vars: [ENV_VAR_NAME]     # host env var(s) to discover from
    required: true
    auth_style: bearer           # bearer, basic, or header
    header_name: authorization   # HTTP header for credential injection

    refresh:                     # optional: gateway-managed token refresh
      strategy: oauth2_refresh_token
      token_url: https://oauth2.googleapis.com/token
      scopes: [...]
      refresh_before_seconds: 300
      max_lifetime_seconds: 3600
      material:                  # non-injectable refresh inputs
        - name: client_secret
          secret: true
        - name: refresh_token
          secret: true

discovery:
  credentials: [credential_key]  # which credentials to discover from host

endpoints:                       # network policy endpoints this provider needs
  - host: "api.example.com"
    port: 443
    protocol: rest
    access: read-only
    enforcement: enforce
    request_body_credential_rewrite: false  # true for OAuth token endpoints

binaries:                        # sandbox binaries this provider needs access to
  - /usr/local/bin/tool
```

## Supported providers

### First-class (OpenShell built-in profiles)

**`github`** -- GitHub API access via `gh` CLI and MCP server.
- Credential: `GITHUB_TOKEN` env var
- Registration: `--from-existing` (OpenShell discovers from host env)
- Sandbox access: `gh` CLI, GitHub MCP server

**`google-vertex-ai`** -- Inference routing through Vertex AI.
- Credential: Google Application Default Credentials (`gcloud auth application-default login`)
- Config: `ANTHROPIC_VERTEX_PROJECT_ID` (or auto-read from ADC), `CLOUD_ML_REGION` (default: `global`)
- Registration: `--from-gcloud-adc`
- The gateway routes inference through `inference.local` -- the sandbox never sees GCP credentials

### Custom profiles (this directory)

**`atlassian`** (`atlassian.yaml`) -- Jira and Confluence via `mcp-atlassian` MCP server.
- Credential: `JIRA_API_TOKEN` env var
- Config: `JIRA_URL` and `JIRA_USERNAME` passed as non-secret env vars in the agent config
- Registration: `--from-existing`
- Endpoints: `*.atlassian.net`, `*.atl-paas.net`, `*.atlassian.com`

**`google-workspace`** (`gws.yaml`) -- Gmail, Calendar, Drive, Tasks via `gws` CLI.
- Credential: OAuth2 access token, gateway-refreshed from a stored refresh token
- Registration: multi-step -- `gws auth export` extracts client credentials, the gateway manages token refresh. The sandbox never sees client_secret or refresh_token.
- Scopes are defined in the provider profile's `refresh.scopes` list. These control what the gateway-minted access token can do -- even though the underlying refresh token may have broader permissions. This is the access scope policy for Google Workspace.
- Endpoints: `gmail.googleapis.com`, `calendar-json.googleapis.com`, `drive.googleapis.com`, `docs.googleapis.com`, `sheets.googleapis.com`, `tasks.googleapis.com`, `oauth2.googleapis.com`

### Future

Additional providers can be added by creating a profile YAML in this directory and adding the registration logic to `cmd/providers.go`. Candidates include Google Cloud (gcloud CLI access), AWS, and Azure.

## Built-in vs custom profiles

OpenShell ships built-in profiles for common providers (`github`, `google-vertex-ai`). Profiles in this directory extend or override built-ins.

The harness imports all profiles from this directory before registering providers. Agent configs reference profiles by name:

```yaml
providers:
  - profile: github              # built-in
  - profile: atlassian           # from providers/atlassian.yaml
  - profile: google-workspace    # from providers/gws.yaml
```

## Registration flows

The harness uses three registration patterns depending on provider type:

| Pattern | Providers | How credentials are discovered |
|---------|-----------|-------------------------------|
| `--from-existing` | github, atlassian | Reads from host env vars |
| `--from-gcloud-adc` | google-vertex-ai | Reads Application Default Credentials |
| Custom OAuth | google-workspace | `gws auth export` + gateway-managed refresh |

Registration happens automatically during `harness apply`. Missing credentials are skipped with a warning.
