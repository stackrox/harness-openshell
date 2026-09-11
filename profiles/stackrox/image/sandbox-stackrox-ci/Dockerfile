# syntax=docker/dockerfile:1.4

# StackRox CI sandbox image for OpenShell repository workflows.
#
# This profile deliberately starts from the StackRox Apollo CI image so that
# repository build tools (Go, compilers, make, and the scanner toolchain) are
# available to the agent. It adds the OpenShell sandbox contract and the
# coding-agent tools supplied by the NVIDIA community base image.
#
# The Apollo image currently publishes an amd64 toolchain. Keep this profile
# amd64-only until the upstream CI image provides a multi-architecture build.
#
# Base image source:
#   https://github.com/stackrox/rox-ci-image/blob/main/images/scanner-build.Dockerfile
# OpenShell base contract:
#   https://github.com/NVIDIA/OpenShell-Community/tree/main/sandboxes/base

ARG OPENSHELL_BASE_IMAGE=ghcr.io/nvidia/openshell-community/sandboxes/base@sha256:aeef1c63f00e2913ea002ccb3aaf925f338b5c5d70e63576f0d95c16a138044e
ARG BASE_IMAGE=quay.io/stackrox-io/apollo-ci:stackrox-build-0.5.14-1-g9bed4c4911
FROM ${OPENSHELL_BASE_IMAGE} AS openshell-base
FROM ${BASE_IMAGE}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

USER root

# Runtime tools used by the OpenShell sandbox contract and by agent skills.
# Apollo already supplies the compiler toolchain, Go, Node, npm, git, jq, and
# curl; install only the missing runtime pieces here.
RUN dnf install -y --setopt=install_weak_deps=False \
        ca-certificates \
        findutils \
        gzip \
        iproute \
        iputils \
        nano \
        net-tools \
        nmap-ncat \
        openssh-clients \
        perl-Digest-SHA \
        procps-ng \
        shadow-utils \
        tar \
        vim-minimal \
        wget \
        which \
    && dnf clean all \
    && rm -rf /var/cache/dnf

# OpenShell expects an unprivileged sandbox user with a writable home and a
# supervisor account available for images that need privileged setup.
RUN groupadd -r supervisor \
    && useradd -r -g supervisor -s /sbin/nologin supervisor \
    && groupadd -r sandbox \
    && useradd -r -g sandbox -d /sandbox -s /bin/bash sandbox \
    && mkdir -p /sandbox \
    && chown sandbox:sandbox /sandbox

# GitHub CLI (Apollo does not include it). Pin the release and verify it
# before installing so the image remains reproducible.
ARG GH_VERSION=2.100.0
ARG GH_SHA256_AMD64=e4d4bb4498e8d007abe545b6568926793ace1b6447da598294a610018cb164be
RUN curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_amd64.tar.gz" -o /tmp/gh.tgz \
    && echo "${GH_SHA256_AMD64}  /tmp/gh.tgz" | sha256sum -c - \
    && tar -xzf /tmp/gh.tgz --strip-components=2 -C /usr/bin "gh_${GH_VERSION}_linux_amd64/bin/gh" \
    && rm -f /tmp/gh.tgz

# Use the same uv-managed Python contract as the community base image. The
# Apollo system Python is 3.9, while current MCP integrations require 3.10+.
COPY --from=ghcr.io/astral-sh/uv:0.10.8 /uv /usr/local/bin/uv
ARG PYTHON_VERSION=3.14.3
ENV UV_PYTHON_INSTALL_DIR=/sandbox/.uv/python
RUN uv python install "${PYTHON_VERSION}" \
    && ln -s "$(uv python find "${PYTHON_VERSION}")" /usr/local/bin/python3 \
    && ln -s "$(uv python find "${PYTHON_VERSION}")" /usr/local/bin/python \
    && uv cache clean

# Coding agents and the JSON-schema utility used by StackRox workflows. These
# versions mirror the current StackRox sandbox profile where applicable.
RUN npm install -g \
        tar@7.5.11 \
        @hono/node-server@1.19.11 \
        opencode-ai@1.18.30 \
        @openai/codex@0.117.0 \
        @github/copilot@1.0.16 \
        ajv-cli@5.0.0 \
    && npm cache clean --force

# Keep the policy path used by the community image even though npm puts the
# executable in /usr/bin on this UBI-based image.
RUN ln -sf /usr/bin/opencode /usr/local/bin/opencode

# Claude Code uses its native installer rather than the deprecated npm path.
RUN curl -fsSL https://claude.ai/install.sh | bash \
    && cp /root/.local/bin/claude /usr/local/bin/claude \
    && chmod 755 /usr/local/bin/claude

# Go-language MCP support for repository analysis. Apollo supplies the Go
# toolchain; gopls is the only additional Go binary needed here.
ARG GOPLS_VERSION=0.20.0
RUN GOBIN=/usr/local/bin /usr/local/go/bin/go install "golang.org/x/tools/gopls@v${GOPLS_VERSION}" \
    && rm -rf /root/go /root/.cache/go-build

# Atlassian MCP is part of the StackRox workflow image contract. The gateway
# still owns credentials; no credential values are baked into this image.
RUN uv venv --python "${PYTHON_VERSION}" --seed /sandbox/.venv \
    && uv pip install --python /sandbox/.venv/bin/python \
        cloudpickle \
        mcp-atlassian==0.21.1 \
    && uv cache clean

# Google Workspace CLI is used by StackRox workflows when the corresponding
# OpenShell provider is attached.
RUN curl -fsSL "https://github.com/googleworkspace/cli/releases/download/v0.22.5/google-workspace-cli-x86_64-unknown-linux-gnu.tar.gz" \
    | tar xz -C /usr/local/bin

ENV PATH="/sandbox/.venv/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin" \
    VIRTUAL_ENV=/sandbox/.venv

# Copy the OpenShell-owned contract from the pinned community image rather than
# duplicating it in this profile. Provider credentials remain gateway-managed
# and are not copied here.
COPY --from=openshell-base /etc/openshell/policy.yaml /etc/openshell/policy.yaml
COPY --from=openshell-base /sandbox/.agents/skills/ /sandbox/.agents/skills/
COPY --from=openshell-base /sandbox/.bashrc /sandbox/.bashrc
COPY --from=openshell-base /sandbox/.profile /sandbox/.profile

# Default agent instructions and configuration. Workflows can override these
# files with payloads for repository-specific skills and instructions.
COPY CLAUDE.md /sandbox/.claude/CLAUDE.md
COPY settings.json /sandbox/.claude/settings.json
COPY claude.json /sandbox/.claude.json
COPY mcp.json /sandbox/.mcp.json
COPY opencode.json /sandbox/opencode.json

RUN mkdir -p /sandbox/.claude/skills /sandbox/.config/openshell \
    && for skill in /sandbox/.agents/skills/*/; do \
         [ -d "${skill}" ] || continue; \
         ln -sf "${skill}" "/sandbox/.claude/skills/$(basename "${skill}")"; \
       done \
    && printf '%s\n' \
         'export PATH="/sandbox/.venv/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"' \
         'export VIRTUAL_ENV="/sandbox/.venv"' \
         'export UV_PYTHON_INSTALL_DIR="/sandbox/.uv/python"' \
         >> /sandbox/.bashrc \
    && chown -R sandbox:sandbox /sandbox

WORKDIR /sandbox
USER sandbox

ENTRYPOINT ["/bin/bash"]
