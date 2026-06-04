#!/usr/bin/env python3
"""Pre-flight checks driven by providers.toml + openshell.toml.

Usage:
  python3 lib/providers.py check              # full pre-flight
  python3 lib/providers.py check --strict     # fail if required missing
  python3 lib/providers.py available          # available provider names
  python3 lib/providers.py names             # all enabled provider names
"""

import json
import os
import re
import shutil
import subprocess
import sys
import tomllib
from pathlib import Path

ROOT = Path(__file__).parent.parent
PROVIDERS_TOML = ROOT / "providers.toml"
CONFIG_TOML = ROOT / "openshell.toml"
CLI = os.environ.get("OPENSHELL_CLI", "openshell")


def load_providers():
    with open(PROVIDERS_TOML, "rb") as f:
        return tomllib.load(f).get("providers", [])


def load_config():
    if not CONFIG_TOML.exists():
        return {}
    with open(CONFIG_TOML, "rb") as f:
        return tomllib.load(f)


def expand_path(p):
    return Path(os.path.expanduser(os.path.expandvars(p)))


def run_quiet(cmd, timeout=5, env=None):
    try:
        r = subprocess.run(cmd, shell=isinstance(cmd, str), capture_output=True, timeout=timeout, env=env)
        return r.returncode == 0, r.stdout.decode().strip()
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return False, ""


def mask_value(val, show=4):
    if not val or len(val) <= show:
        return "***"
    return val[:show] + "***"


def file_metadata(path):
    p = expand_path(path)
    if not p.exists():
        return None
    try:
        with open(p) as f:
            data = json.load(f)
        meta = {}
        if "quota_project_id" in data:
            meta["project"] = data.get("quota_project_id", "")
            meta["type"] = data.get("type", "")
        if "installed" in data:
            meta["client_id"] = mask_value(data["installed"].get("client_id", ""))
        return meta or None
    except (json.JSONDecodeError, KeyError, OSError):
        return None


def strip_ansi(s):
    return re.sub(r'\x1b\[[0-9;]*m', '', s)


# ── Input checking ────────────────────────────────────────────────────

def check_input(inp):
    """Check a single input. Returns (ok, detail_line)."""
    key = inp["key"]
    kind = inp.get("kind", "env")
    secret = inp.get("secret", False)
    desc = inp.get("description", "")

    if kind == "env":
        val = os.environ.get(key)
        if val:
            display = mask_value(val) if secret else val
            return True, f"✓ local env: {key}={display}"
        else:
            return False, f"✗ local env: {key} not set  →  export {key}=..."

    elif kind == "file":
        ep = expand_path(key)
        if ep.exists():
            meta = file_metadata(key)
            if meta and secret:
                safe = {k: v for k, v in meta.items() if k in ("project", "type")}
                masked = {k: v for k, v in meta.items() if k not in ("project", "type")}
                parts = [f"{k}={v}" for k, v in safe.items() if v]
                parts += [f"{k}={v}" for k, v in masked.items() if v]
                return True, f"✓ local file: {key} ({', '.join(parts)})" if parts else f"✓ local file: {key}"
            elif meta:
                meta_str = ", ".join(f"{k}={v}" for k, v in meta.items() if v)
                return True, f"✓ local file: {key} ({meta_str})" if meta_str else f"✓ local file: {key}"
            return True, f"✓ local file: {key}"
        else:
            return False, f"✗ local file: {key} not found"

    elif kind == "check":
        expanded = os.path.expandvars(key)
        ok, _ = run_quiet(expanded)
        sym = "✓" if ok else "✗"
        # Show original key (with ${VAR} references), not expanded values
        return ok, f"{sym} check: {key}"

    return False, f"{key}: unknown kind '{kind}'"


def check_provider(provider):
    """Check all inputs for a provider. Returns (ok, details)."""
    issues = 0
    details = []
    for inp in provider.get("inputs", []):
        ok, detail = check_input(inp)
        if not ok:
            issues += 1
        details.append(detail)
    return issues == 0, details


# ── Enabled providers ─────────────────────────────────────────────────

def enabled_providers(all_providers, config):
    """Filter to enabled providers based on openshell.toml."""
    enabled_names = set()
    if config:
        enabled_names.update(config.get("providers", []))
        enabled_names.update(config.get("providers-custom", []))
    else:
        return all_providers  # no config = all enabled

    return [p for p in all_providers if p["name"] in enabled_names]


# ── Commands ──────────────────────────────────────────────────────────

def cmd_check(strict=False):
    all_providers = load_providers()
    config = load_config()
    providers = enabled_providers(all_providers, config)
    has_failures = False

    # OpenShell CLI
    print("=== OpenShell CLI ===")
    cli_exists = shutil.which(CLI) is not None
    if not cli_exists:
        print("  ✗ not found on PATH")
        has_failures = True
    else:
        _, ver = run_quiet([CLI, "--version"])
        print(f"  ✓ {ver or CLI}")
        cli_path = shutil.which(CLI)
        print(f"    {cli_path}")

    # Detect active gateway from CLI
    active_gw = ""
    if cli_exists:
        _, gw_list = run_quiet([CLI, "gateway", "list"])
        for line in gw_list.splitlines():
            if line.startswith("*"):
                active_gw = line.split()[1] if len(line.split()) > 1 else ""
                break
    is_k8s = "-remote-" in active_gw

    # Gateway — only check the relevant one
    gw_ok = False
    if is_k8s:
        print()
        print("=== K8s gateway ===")
        has_kubectl = shutil.which("kubectl") is not None

        if not has_kubectl:
            print("  ✗ kubectl not found")
            has_failures = True
        else:
            cluster_ok, ctx = run_quiet("kubectl config current-context")
            if cluster_ok:
                print(f"  ✓ Cluster: {ctx}")
            else:
                print("  ✗ No cluster (kubectl not configured)")
                has_failures = True

            if cluster_ok and cli_exists:
                ocp_ok, out = run_quiet([CLI, "inference", "get"])
                if ocp_ok:
                    gw_ok = True
                    model = ""
                    for line in out.splitlines():
                        if "Model:" in line:
                            model = strip_ansi(line.split("Model:")[1]).strip()
                    print(f"  ✓ Gateway reachable (model: {model})" if model else "  ✓ Gateway reachable")
                else:
                    print("  ✗ Gateway unreachable")
    else:
        print()
        print("=== Podman gateway ===")
        if cli_exists:
            local_ok, out = run_quiet([CLI, "inference", "get"])
            if local_ok:
                gw_ok = True
                model = ""
                for line in out.splitlines():
                    if "Model:" in line:
                        model = strip_ansi(line.split("Model:")[1]).strip()
                print(f"  ✓ Reachable (model: {model})" if model else "  ✓ Reachable")
            else:
                print("  - Not running")

            if shutil.which("podman"):
                _, ver = run_quiet(["podman", "--version"])
                print(f"  ✓ Podman: {ver}")
            else:
                print("  ✗ Podman not found")
                has_failures = True
        else:
            print("  - CLI not available")

    # Registered providers
    if cli_exists and gw_ok:
        print()
        gw_label = "k8s" if is_k8s else "podman"
        print(f"=== Registered providers ({gw_label}) ===")
        openshell_providers = [p for p in providers if p.get("type") == "openshell"]
        for p in openshell_providers:
            reg_ok, _ = run_quiet([CLI, "provider", "get", p["name"]])
            if reg_ok:
                print(f"  ✓ {p['name']}")
            else:
                print(f"  ✗ {p['name']}: not registered — run ./setup-providers.sh")
                has_failures = True

    # Provider inputs
    print()
    print("=== Provider inputs ===")
    for p in providers:
        ok, details = check_provider(p)
        name = p["name"]
        desc = p.get("description", "")
        required = p.get("required", False)

        if ok:
            print(f"  ✓ {name}")
        else:
            print(f"  ✗ {name}")
            if required:
                has_failures = True
        print(f"    {desc}")

        for d in details:
            print(f"      {d}")

        upstream = p.get("upstream")
        if upstream and not ok:
            print(f"      upstream: {upstream}")
        print()

    # Summary
    if has_failures:
        print("✗ Not ready — fix issues above")
        if strict:
            sys.exit(1)
    else:
        print("✓ Ready to launch")


def cmd_available():
    providers = enabled_providers(load_providers(), load_config())
    available = []
    for p in providers:
        if p.get("type") != "openshell":
            continue
        ok, _ = check_provider(p)
        if ok:
            available.append(p["name"])
    print(" ".join(available))


def cmd_names():
    providers = enabled_providers(load_providers(), load_config())
    names = [p["name"] for p in providers if p.get("type") == "openshell"]
    print(" ".join(names))


def cmd_sandbox_env():
    """Print sandbox env vars from openshell.toml [sandbox.env] as export statements."""
    config = load_config()
    env = config.get("sandbox", {}).get("env", {})
    for key, val in env.items():
        print(f"export {key}={val}")


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else "check"
    strict = "--strict" in sys.argv

    if cmd == "check":
        cmd_check(strict=strict)
    elif cmd == "available":
        cmd_available()
    elif cmd == "names":
        cmd_names()
    elif cmd == "sandbox-env":
        cmd_sandbox_env()
    else:
        print(f"Unknown command: {cmd}", file=sys.stderr)
        sys.exit(1)
