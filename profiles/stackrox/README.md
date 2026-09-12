# StackRox sandbox images

StackRox profiles are optional OpenShell sandbox images for workflows that need
repository-specific tools. Providers, credentials, skills supplied by a
workflow, and task-specific policy remain outside the image.
The image does not create or attach providers; a workflow must name providers
that are already provisioned and attach them through `sandbox.providers` before
provider credentials or inference routes are available.

## Profiles

### `sandbox-default`

The general StackRox image, based on the NVIDIA OpenShell community image. It
adds the integrations shared by StackRox workflows, including Atlassian MCP,
Google Workspace, and the ACS triage toolchain (`gopls` and `ajv-cli`).

Build it locally with:

```bash
make dev-sandbox
```

### `sandbox-stackrox-ci`

An opt-in image based on the StackRox `rox-ci-image` build image
`quay.io/stackrox-io/apollo-ci:stackrox-build-0.5.14-1-g9bed4c4911`. It keeps
the StackRox CI toolchain (Go, compilers, make, and scanner build tools) and
adds the OpenShell sandbox contract, coding agents, `gh`, `uv`, `ajv-cli`, the
GitHub skill, Atlassian MCP, Google Workspace CLI, and the `gopls` MCP server.
Go module and build caches stay below `/sandbox`. It deliberately does not
install `gcloud` or copy service-account keys; OpenShell providers own those
credentials and inference routes.

The `rox-ci-image` build currently provides an amd64 toolchain, so this profile
is published for `linux/amd64` only. It is an experimental alternative to
`sandbox-default`, not a replacement for it.

Build it locally with:

```bash
docker build --platform linux/amd64 \
  -t quay.io/rcochran/openshell:sandbox-stackrox-ci \
  profiles/stackrox/image/sandbox-stackrox-ci
```
