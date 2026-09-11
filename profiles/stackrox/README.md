# StackRox sandbox image

`image/sandbox-default` is the optional StackRox agent image. It extends the
NVIDIA OpenShell community base with integrations shared by StackRox workflows,
including Atlassian MCP and Google Workspace tooling.

It also includes the ACS triage toolchain: Go, `gopls` (the Go-analysis MCP
server), and `ajv-cli` for JSON Schema validation. `gcloud` is intentionally
not included; OpenShell provider credentials and inference routing replace the
runner-side service-account setup used by the original GitHub Actions workflow.

Generic Harness workflows use the NVIDIA base image directly. Select the
published StackRox image only when a workflow needs one of these additions;
providers, credentials, skills, and task-specific policy remain outside the
image.

Build it locally with:

```bash
make dev-sandbox
```
