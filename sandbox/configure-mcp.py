#!/usr/bin/env python3
"""Generate .claude.json MCP server config from environment variables.

Writes /sandbox/.claude.json with configured MCP servers based on
which credentials are available. Skips servers whose credentials are
not set.

JIRA_URL and JIRA_USERNAME are read from an uploaded config file
(not secrets — no need for provider placeholders). JIRA_API_TOKEN
comes from the atlassian provider as a placeholder resolved by the proxy.
"""

import json
import os
import stat

config = {
    "autoUpdates": False,
    "hasCompletedOnboarding": True,
    "projects": {
        "/sandbox": {
            "hasTrustDialogAccepted": True,
            "allowedTools": [],
        },
    },
    "mcpServers": {},
}

# ── Atlassian (sooperset/mcp-atlassian) ────────────────────────────────
# JIRA_URL and JIRA_USERNAME come from either:
#   - An uploaded atlassian.json file (sandbox.sh / local workflow)
#   - Environment variables injected via secretKeyRef (in-cluster launcher)
atlassian_config = "/sandbox/.harness/creds/atlassian.json"
jira_url = ""
jira_username = ""
if os.path.isfile(atlassian_config):
    try:
        with open(atlassian_config) as f:
            atlassian = json.load(f)
        jira_url = atlassian.get("jira_url", "")
        jira_username = atlassian.get("jira_username", "")
    except (json.JSONDecodeError, OSError) as e:
        print(f"WARNING: could not read atlassian config: {e}", flush=True)
# Fall back to env vars (set by in-cluster launcher via secretKeyRef)
if not jira_url:
    jira_url = os.environ.get("JIRA_URL", "")
if not jira_username:
    jira_username = os.environ.get("JIRA_USERNAME", "")

if jira_url:
    jira_api_token = os.environ.get("JIRA_API_TOKEN", "")
    config["mcpServers"]["atlassian"] = {
        "type": "stdio",
        "command": "/sandbox/.venv/bin/mcp-atlassian",
        "args": [],
        "env": {
            "JIRA_URL": jira_url,
            "JIRA_USERNAME": jira_username,
            "JIRA_API_TOKEN": jira_api_token,
            "CONFLUENCE_URL": jira_url.rstrip("/") + "/wiki",
            "CONFLUENCE_USERNAME": jira_username,
            "CONFLUENCE_API_TOKEN": jira_api_token,
            "READ_ONLY_MODE": os.environ.get("READ_ONLY_MODE", "true"),
        },
    }

path = "/sandbox/.claude.json"
with open(path, "w") as f:
    json.dump(config, f, indent=2)
os.chmod(path, stat.S_IRUSR | stat.S_IWUSR)
