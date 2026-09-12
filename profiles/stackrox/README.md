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

### `sandbox-collector-builder`

An amd64 image based on the StackRox Collector builder image. The `master`
builder manifest is pinned to
`sha256:52a3cca5253abb0cd5c3606769d5fd5b8be4cdb4671396f38a792695f07d7696`.
It keeps the Collector compiler and build toolchain and adds the same
OpenShell contract, coding agents, GitHub skill, Atlassian MCP, Google
Workspace CLI, and `gopls` support as the StackRox CI profile. It is separate
from `sandbox-stackrox-ci` so workflows can choose the Collector-specific
toolchain without changing the Apollo/rox-ci-image profile.

The base image is multi-architecture, but several bundled third-party
binaries are currently amd64-only, so this profile is published for
`linux/amd64` only.

Build it locally with:

```bash
docker build --platform linux/amd64 \
  -t quay.io/rcochran/openshell:sandbox-collector-builder \
  profiles/stackrox/image/sandbox-collector-builder
```
