# Native OpenShell inputs

Import the provider profiles and create provider instances in the target
workspace before running the workflow. The instance names must match the
policy bindings:

```bash
openshell provider profile import \
  -f openshell/providers/github-readonly.yaml
openshell provider profile import \
  -f openshell/providers/atlassian-readonly.yaml
openshell provider create --name github-readonly \
  --type github-readonly --credential GITHUB_TOKEN
openshell provider create --name atlassian-readonly \
  --type atlassian-readonly --credential JIRA_API_TOKEN
```

Render `policy.yaml` with the selected repository and then create a sandbox
with the native OpenShell CLI. The policy is intentionally the authority for
the read-only boundary; do not replace it with unrestricted provider egress.
