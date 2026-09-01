# Provider profile examples

These files use the upstream OpenShell provider-profile format. They describe
credential discovery, proxy injection, refresh, endpoint policy, and allowed
sandbox binaries for integrations not fully covered by built-in profiles.

They are inputs to the platform bootstrap process, not to `harness apply`.
Import and create providers with OpenShell before applying a workflow. The
harness then verifies and reconciles the provider resources declared in
`spec.providers`; names in `spec.sandbox.providers` attach existing providers
without claiming ownership.

The checked-in examples are:

- `atlassian.yaml`: Jira and Confluence through `mcp-atlassian`, using
  `JIRA_API_TOKEN` plus non-secret endpoint/user configuration.
- `gws.yaml`: Google Workspace APIs through a gateway-managed OAuth refresh
  token. Refresh material stays in the gateway; the sandbox receives only a
  proxy-resolved placeholder.

OpenShell ships built-in profiles for providers such as GitHub and Vertex AI.
Keep custom profiles narrowly scoped and prefer an upstream profile when one
becomes available.
