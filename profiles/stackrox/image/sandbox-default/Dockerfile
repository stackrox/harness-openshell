# syntax=docker/dockerfile:1.4

# StackRox OpenShell sandbox image for repository workflows
#
# Extends the community base image with Atlassian MCP and Google Workspace
# CLI. All other tools (claude, gh, node, python, git, curl, uv) are
# provided by the base image.
#
# Community base image pattern:
#   https://github.com/NVIDIA/OpenShell-Community/tree/main/sandboxes
#
# Build:  docker build --platform linux/amd64 -t quay.io/rcochran/openshell:sandbox .
# Push:   docker push quay.io/rcochran/openshell:sandbox
#
# ── Tool versions ──────────────────────────────────────────────────────
#
# Pinned for reproducibility. Bump versions here and rebuild — tools
# are never installed at sandbox startup (deterministic image).
#
#   mcp-atlassian  0.21.1   https://github.com/sooperset/mcp-atlassian/releases
#   gws CLI        0.22.5   https://github.com/googleworkspace/cli/releases
#   gopls          0.20.0   https://pkg.go.dev/golang.org/x/tools/gopls
#   ajv-cli        5.0.0    https://github.com/ajv-validator/ajv-cli
#
# Base image tools (inherited, pinned upstream):
#   claude         latest   https://claude.ai/install.sh
#   gh             latest   https://cli.github.com/packages
#   node           22.x     https://deb.nodesource.com
#   python         3.14.x   https://github.com/astral-sh/uv
#   uv             0.10.x   https://github.com/astral-sh/uv
#   Go             1.24.2   https://go.dev/dl/
#
# ── Workarounds in this image ──────────────────────────────────────────
#
# mcp-atlassian: Installed because the base image doesn't include it.
#   Could become a community sandbox (like ollama, gemini) if NVIDIA
#   adds an Atlassian sandbox to OpenShell-Community.
#
# gws CLI: Installed because the base image doesn't include it.
#   When OpenShell adds file-based credential projection (#1268, #1423),
#   GWS config could be provider-managed and the CLI could move to a
#   community sandbox.
#
ARG BASE_IMAGE=ghcr.io/nvidia/openshell-community/sandboxes/base@sha256:aeef1c63f00e2913ea002ccb3aaf925f338b5c5d70e63576f0d95c16a138044e
FROM ${BASE_IMAGE}

USER root

# mcp-atlassian for Jira/Confluence MCP server
# https://github.com/sooperset/mcp-atlassian
RUN pip install --no-cache-dir mcp-atlassian==0.21.1

# Go and gopls for the Go-analysis MCP server used by ACS triage. Keep the Go
# toolchain in the runtime image because gopls invokes `go list` and related
# commands while analyzing a checked-out repository.
ARG GO_VERSION=1.24.2
ARG GO_SHA256_AMD64=68097bd680839cbc9d464a0edce4f7c333975e27a90246890e9f1078c7e702ad
ARG GO_SHA256_ARM64=756274ea4b68fa5535eb9fe2559889287d725a8da63c6aae4d5f23778c229f4b
ARG GOPLS_VERSION=0.20.0
RUN set -eux; \
    case "$(dpkg --print-architecture)" in \
      amd64) go_arch=amd64; go_sha256="$GO_SHA256_AMD64" ;; \
      arm64) go_arch=arm64; go_sha256="$GO_SHA256_ARM64" ;; \
      *) echo "unsupported architecture: $(dpkg --print-architecture)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" -o /tmp/go.tgz; \
    echo "${go_sha256}  /tmp/go.tgz" | sha256sum -c -; \
    tar -C /usr/local -xzf /tmp/go.tgz; \
    GOBIN=/usr/local/bin /usr/local/go/bin/go install "golang.org/x/tools/gopls@v${GOPLS_VERSION}"; \
    rm -rf /tmp/go.tgz /root/go /root/.cache/go-build

# Google Workspace CLI (Gmail, Calendar, Drive, Docs, Sheets)
# https://github.com/googleworkspace/cli
RUN ARCH=$(uname -m | sed 's/arm64/aarch64/') && \
    curl -fsSL "https://github.com/googleworkspace/cli/releases/download/v0.22.5/google-workspace-cli-${ARCH}-unknown-linux-gnu.tar.gz" \
    | tar xz -C /usr/local/bin

# OpenCode AI coding agent
# https://github.com/opencode-ai/opencode
# JSON Schema validation used by the ACS triage workflow.
RUN npm install -g opencode-ai@1.18.30 ajv-cli@5.0.0 && \
    npm cache clean --force

ENV PATH="/sandbox/.venv/bin:/usr/local/go/bin:${PATH}"

# Sandbox policy (network egress rules — provider profiles contribute
# additional endpoints at runtime via providers v2 composition)
COPY policy.yaml /etc/openshell/policy.yaml

# Agent instructions + config (Claude Code + OpenCode)
# These are defaults. Override any file via payloads: in the agent YAML:
#   payloads:
#     - destination: /sandbox/.claude/CLAUDE.md
#       content: |
#         Custom instructions here.
COPY CLAUDE.md /sandbox/.claude/CLAUDE.md
COPY settings.json /sandbox/.claude/settings.json
COPY claude.json /sandbox/.claude.json
COPY mcp.json /sandbox/.mcp.json
COPY opencode.json /sandbox/opencode.json

# Harness config directory (populated at runtime via upload)
RUN mkdir -p /sandbox/.config/openshell && chown sandbox:sandbox /sandbox/.config/openshell

RUN chown -R sandbox:sandbox /sandbox/.claude /sandbox/.claude.json /sandbox/.mcp.json /sandbox/opencode.json /sandbox/.config

USER sandbox

ENTRYPOINT ["/bin/bash"]
