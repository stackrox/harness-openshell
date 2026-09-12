# syntax=docker/dockerfile:1.4

# StackRox CI sandbox image for OpenShell repository workflows.
#
# This profile deliberately starts from the StackRox rox-ci-image build so that
# repository build tools (Go, compilers, make, and the scanner toolchain) are
# available to the agent. It adds the OpenShell sandbox contract and the
# coding-agent tools plus selected vendored assets from the NVIDIA community
# base image.
#
# The rox-ci-image build currently publishes an amd64 toolchain. Keep this
# profile amd64-only until the upstream CI image provides a multi-architecture
# build.
#
# Base image source:
#   https://github.com/stackrox/rox-ci-image/blob/main/images/scanner-build.Dockerfile
# OpenShell base contract:
#   https://github.com/NVIDIA/OpenShell-Community/tree/main/sandboxes/base

# The tag is stackrox-build-0.5.14-1-g9bed4c4911; pin its immutable digest
# so a retag cannot silently change the toolchain.
ARG BASE_IMAGE=quay.io/stackrox-io/apollo-ci@sha256:fe0e38d8d7792fbe61b593d9fca98906985404a083d1e761f344a8c7e49b873b
FROM ${BASE_IMAGE}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

USER root

# Runtime tools used by the OpenShell sandbox contract and by agent skills.
# rox-ci-image already supplies the compiler toolchain, Go, Node, npm, git, jq,
# and curl; install only the missing runtime pieces here.
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

# GitHub CLI (rox-ci-image does not include it). Pin the release and verify it
# before installing so the image remains reproducible.
ARG GH_VERSION=2.100.0
ARG GH_SHA256_AMD64=e4d4bb4498e8d007abe545b6568926793ace1b6447da598294a610018cb164be
RUN curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_amd64.tar.gz" -o /tmp/gh.tgz \
    && echo "${GH_SHA256_AMD64}  /tmp/gh.tgz" | sha256sum -c - \
    && mkdir -p /tmp/gh-extract \
    && tar -xzf /tmp/gh.tgz --strip-components=2 -C /tmp/gh-extract "gh_${GH_VERSION}_linux_amd64/bin/gh" \
    && test -f /tmp/gh-extract/gh \
    && install -m 0755 /tmp/gh-extract/gh /usr/bin/gh \
    && rm -rf /tmp/gh-extract /tmp/gh.tgz

# Use the same uv-managed Python contract as the community base image. The
# rox-ci-image system Python is 3.9, while current MCP integrations require
# 3.10+.
COPY --from=ghcr.io/astral-sh/uv:0.10.8 /uv /usr/local/bin/uv
ARG PYTHON_VERSION=3.14.3
ENV UV_PYTHON_INSTALL_DIR=/sandbox/.uv/python
RUN uv python install "${PYTHON_VERSION}" \
    && uv cache clean

# Coding agents and the JSON-schema utility used by StackRox workflows. These
# versions mirror the current StackRox sandbox profile where applicable.
ARG NPM_HONO_VERSION=1.19.11
ARG NPM_OPENCODE_VERSION=1.18.30
ARG NPM_CODEX_VERSION=0.117.0
ARG NPM_COPILOT_VERSION=1.0.16
ARG NPM_AJV_VERSION=5.0.0
RUN mkdir -p /tmp/npm-global \
    && chown sandbox:sandbox /tmp/npm-global
USER sandbox
RUN NPM_CONFIG_PREFIX=/tmp/npm-global npm install -g \
        "@hono/node-server@${NPM_HONO_VERSION}" \
        "opencode-ai@${NPM_OPENCODE_VERSION}" \
        "@openai/codex@${NPM_CODEX_VERSION}" \
        "@github/copilot@${NPM_COPILOT_VERSION}" \
        "ajv-cli@${NPM_AJV_VERSION}" \
    && npm cache clean --force
USER root
RUN cp -a /tmp/npm-global/lib/node_modules/. /usr/lib/node_modules/ \
    && for name in opencode codex copilot ajv; do \
         test -e "/tmp/npm-global/bin/${name}"; \
         cp -a "/tmp/npm-global/bin/${name}" "/usr/bin/${name}"; \
         chown -h root:root "/usr/bin/${name}"; \
       done \
    && chown -R root:root /usr/lib/node_modules \
    && rm -rf /tmp/npm-global

# Claude Code's native binary. Pin both the release URL and its release
# manifest checksum instead of executing a downloaded installer as root.
ARG CLAUDE_VERSION=2.1.269
ARG CLAUDE_SHA256_AMD64=25e44883f54419569a3d739f38cbbdaebe83b09895da0f343e1b003710a4775b
RUN curl -fsSL "https://downloads.claude.ai/claude-code-releases/${CLAUDE_VERSION}/linux-x64/claude" -o /tmp/claude \
    && echo "${CLAUDE_SHA256_AMD64}  /tmp/claude" | sha256sum -c - \
    && install -m 0755 /tmp/claude /usr/local/bin/claude \
    && rm -f /tmp/claude

# Go-language MCP support for repository analysis. rox-ci-image supplies the Go
# toolchain; gopls is the only additional Go binary needed here.
ARG GOPLS_VERSION=0.20.0
RUN GOPATH=/tmp/gopath GOCACHE=/tmp/gocache GOBIN=/usr/local/bin \
        /usr/local/go/bin/go install "golang.org/x/tools/gopls@v${GOPLS_VERSION}" \
    && rm -rf /tmp/gopath /tmp/gocache

# Atlassian MCP is part of the StackRox workflow image contract. The gateway
# still owns credentials; no credential values are baked into this image.
RUN uv venv --python "${PYTHON_VERSION}" --seed /sandbox/.venv \
    && uv pip install --python /sandbox/.venv/bin/python \
        cloudpickle==3.1.2 \
        mcp-atlassian==0.21.1 \
    && uv cache clean

# Google Workspace CLI is used by StackRox workflows when the corresponding
# OpenShell provider is attached. Use the static musl build so it runs on the
# UBI glibc version supplied by rox-ci-image.
ARG GWS_VERSION=0.22.5
ARG GWS_SHA256_AMD64=4db473dde4b1ab872e4ff35d769b0d4af1f1a6441a605e79d5cf8ada9c87e920
RUN curl -fsSL "https://github.com/googleworkspace/cli/releases/download/v${GWS_VERSION}/google-workspace-cli-x86_64-unknown-linux-musl.tar.gz" -o /tmp/gws.tgz \
    && echo "${GWS_SHA256_AMD64}  /tmp/gws.tgz" | sha256sum -c - \
    && mkdir -p /tmp/gws-extract \
    && tar xzf /tmp/gws.tgz --no-same-owner --no-same-permissions -C /tmp/gws-extract ./gws \
    && test -f /tmp/gws-extract/gws \
    && install -m 0755 /tmp/gws-extract/gws /usr/local/bin/gws \
    && rm -rf /tmp/gws-extract \
    && rm -f /tmp/gws.tgz

ENV PATH="/sandbox/.venv/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin" \
    VIRTUAL_ENV=/sandbox/.venv \
    GOPATH=/sandbox/.cache/go \
    GOCACHE=/sandbox/.cache/go-build \
    GOMODCACHE=/sandbox/.cache/go-mod

# Vendor only the small OpenShell assets this profile uses. The upstream
# network policy is intentionally not inherited; this profile owns its small
# StackRox policy and provider profiles supply integration egress at runtime.
COPY openshell/skills/ /sandbox/.agents/skills/
COPY openshell/.bashrc /sandbox/.bashrc
COPY openshell/.profile /sandbox/.profile
COPY policy.yaml /etc/openshell/policy.yaml

# Default agent instructions and configuration. Workflows can override these
# files with payloads for repository-specific skills and instructions.
COPY CLAUDE.md /sandbox/.claude/CLAUDE.md
COPY settings.json /sandbox/.claude/settings.json
COPY claude.json /sandbox/.claude.json
COPY mcp.json /sandbox/.mcp.json
COPY opencode.json /sandbox/opencode.json

RUN mkdir -p /sandbox/.claude/skills /sandbox/.config/openshell \
    && chmod 0644 /etc/openshell/policy.yaml \
         /sandbox/.claude/CLAUDE.md \
         /sandbox/.claude/settings.json \
         /sandbox/.claude.json \
         /sandbox/.mcp.json \
         /sandbox/opencode.json \
    && for skill in /sandbox/.agents/skills/*/; do \
         [ -d "${skill}" ] || continue; \
         ln -sf "${skill}" "/sandbox/.claude/skills/$(basename "${skill}")"; \
       done \
    && chown -R sandbox:sandbox /sandbox

WORKDIR /sandbox
USER sandbox

ENTRYPOINT ["/bin/bash"]
