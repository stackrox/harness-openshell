# syntax=docker/dockerfile:1.4

# OpenShell sandbox image for RHACS Collector team
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
#
# Base image tools (inherited, pinned upstream):
#   claude         latest   https://claude.ai/install.sh
#   gh             latest   https://cli.github.com/packages
#   node           22.x     https://deb.nodesource.com
#   python         3.14.x   https://github.com/astral-sh/uv
#   uv             0.10.x   https://github.com/astral-sh/uv
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
ARG BASE_IMAGE=ghcr.io/nvidia/openshell-community/sandboxes/base:latest
FROM ${BASE_IMAGE}

USER root

# mcp-atlassian for Jira/Confluence MCP server
# https://github.com/sooperset/mcp-atlassian
RUN pip install --no-cache-dir mcp-atlassian==0.21.1

# Google Workspace CLI (Gmail, Calendar, Drive, Docs, Sheets)
# https://github.com/googleworkspace/cli
RUN ARCH=$(uname -m | sed 's/arm64/aarch64/') && \
    curl -fsSL "https://github.com/googleworkspace/cli/releases/download/v0.22.5/google-workspace-cli-${ARCH}-unknown-linux-gnu.tar.gz" \
    | tar xz -C /usr/local/bin

# OpenCode AI coding agent
# https://github.com/opencode-ai/opencode
RUN npm install -g opencode-ai@latest

# Sandbox policy (network egress rules — provider profiles contribute
# additional endpoints at runtime via providers v2 composition)
COPY policy.yaml /etc/openshell/policy.yaml

# Agent instructions + config (Claude Code + OpenCode)
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
