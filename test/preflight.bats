#!/usr/bin/env bats
# Tests for lib/providers.py preflight check.
#
# Stubs external CLIs (openshell, kubectl, gcloud, podman, curl, gws)
# and uses temp TOML files to test every configuration path.

REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)"
PROVIDERS_PY="$REPO_ROOT/lib/providers.py"

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export STUB_DIR="$TEST_TMPDIR/stubs"
  mkdir -p "$STUB_DIR"

  # Isolate from real environment
  unset GITHUB_TOKEN JIRA_API_TOKEN JIRA_URL JIRA_USERNAME
  unset ANTHROPIC_VERTEX_PROJECT_ID CLOUD_ML_REGION
  unset OPENSHELL_GATEWAY OPENSHELL_NAMESPACE OPENSHELL_CLI
  unset GOOGLE_APPLICATION_CREDENTIALS

  # Point providers.py at temp TOML files
  export PROVIDERS_TOML="$TEST_TMPDIR/providers.toml"
  export CONFIG_TOML="$TEST_TMPDIR/openshell.toml"

  # Put stubs first on PATH, strip real openshell/kubectl/podman/docker
  # Keep /opt/homebrew for python3 with tomllib (3.11+)
  export PATH="$STUB_DIR:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ── Helpers ──────────────────────────────────────────────────────────

write_providers_toml() {
  cat > "$PROVIDERS_TOML" "$@"
}

write_config_toml() {
  cat > "$CONFIG_TOML" "$@"
}

make_stub() {
  local name="$1"; shift
  cat > "$STUB_DIR/$name" <<STUB
#!/usr/bin/env bash
$@
STUB
  chmod +x "$STUB_DIR/$name"
}

run_preflight() {
  # Override the TOML paths via env — providers.py reads ROOT-relative,
  # so we patch it with a wrapper
  python3 -c "
import sys, os
sys.path.insert(0, '$REPO_ROOT/lib')
os.chdir('$TEST_TMPDIR')

from pathlib import Path
import providers
providers.PROVIDERS_TOML = Path('$PROVIDERS_TOML')
providers.CONFIG_TOML = Path('$CONFIG_TOML')
providers.CLI = os.environ.get('OPENSHELL_CLI', 'openshell')

cmd = sys.argv[1] if len(sys.argv) > 1 else 'check'
strict = '--strict' in sys.argv
if cmd == 'check':
    providers.cmd_check(strict=strict)
elif cmd == 'available':
    providers.cmd_available()
elif cmd == 'names':
    providers.cmd_names()
" "$@"
}

# ── ENV input tests ──────────────────────────────────────────────────

@test "env input: set non-secret shows full value" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "MY_VAR", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF

  export MY_VAR="hello-world"
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✓ local env: MY_VAR=hello-world"* ]]
}

@test "env input: set secret shows masked value" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "MY_SECRET", kind = "env", secret = true },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF

  export MY_SECRET="super-secret-token-12345"
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✓ local env: MY_SECRET=supe***"* ]]
  [[ "$output" != *"super-secret-token-12345"* ]]
}

@test "env input: missing shows error with export hint" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "MISSING_VAR", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✗ local env: MISSING_VAR not set"* ]]
  [[ "$output" == *"export MISSING_VAR="* ]]
}

@test "env input: short secret fully masked" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "SHORT", kind = "env", secret = true },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF

  export SHORT="abc"
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✓ local env: SHORT=***"* ]]
  [[ "$output" != *"abc"* ]]
}

# ── FILE input tests ─────────────────────────────────────────────────

@test "file input: existing file shows check" {
  local fakefile="$TEST_TMPDIR/somefile.txt"
  echo "data" > "$fakefile"

  write_providers_toml <<EOF
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "$fakefile", kind = "file" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✓ local file: $fakefile"* ]]
}

@test "file input: missing file shows error" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "/nonexistent/file.json", kind = "file" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✗ local file: /nonexistent/file.json not found"* ]]
}

@test "file input: ADC json extracts project metadata" {
  local adc="$TEST_TMPDIR/adc.json"
  cat > "$adc" <<'JSON'
{"quota_project_id": "my-project", "type": "authorized_user"}
JSON

  write_providers_toml <<EOF
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "$adc", kind = "file", secret = true },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"project=my-project"* ]]
  [[ "$output" == *"type=authorized_user"* ]]
}

@test "file input: GWS client_secret extracts masked client_id" {
  local gws="$TEST_TMPDIR/client_secret.json"
  cat > "$gws" <<'JSON'
{"installed": {"client_id": "1715999888.apps.googleusercontent.com"}}
JSON

  write_providers_toml <<EOF
[[providers]]
name = "test"
type = "custom"
description = "test provider"
inputs = [
  { key = "$gws", kind = "file", secret = true },
]
EOF
  write_config_toml <<'EOF'
providers-custom = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"client_id=1715***"* ]]
  [[ "$output" != *"1715999888"* ]]
}

# ── CHECK input tests ────────────────────────────────────────────────

@test "check input: passing command shows checkmark" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "true", kind = "check" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✓ check: true"* ]]
}

@test "check input: failing command shows x" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "false", kind = "check" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✗ check: false"* ]]
}

@test "check input: env vars expanded in command" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test provider"
inputs = [
  { key = "test -n ${MY_CHECK_VAR}", kind = "check" },
]
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  export MY_CHECK_VAR="yes"
  run run_preflight check
  [[ "$output" == *"✓ check:"* ]]
  # Output shows original template, not expanded
  [[ "$output" == *'${MY_CHECK_VAR}'* ]]
}

# ── Provider status tests ────────────────────────────────────────────

@test "provider: all inputs pass shows checkmark" {
  write_providers_toml <<'EOF'
[[providers]]
name = "good"
type = "openshell"
description = "all good"
inputs = [
  { key = "GOOD_VAR", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["good"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'
  export GOOD_VAR="yes"

  run run_preflight check
  [[ "$output" == *"✓ good"* ]]
}

@test "provider: any input fails shows x" {
  write_providers_toml <<'EOF'
[[providers]]
name = "bad"
type = "openshell"
description = "missing input"
inputs = [
  { key = "SET_VAR", kind = "env" },
  { key = "MISSING_VAR", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["bad"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'
  export SET_VAR="yes"

  run run_preflight check
  [[ "$output" == *"✗ bad"* ]]
}

@test "provider: required failure with --strict exits non-zero" {
  write_providers_toml <<'EOF'
[[providers]]
name = "critical"
type = "openshell"
description = "must have"
required = true
inputs = [
  { key = "MISSING", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["critical"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check --strict
  [ "$status" -ne 0 ]
  [[ "$output" == *"✗ Not ready"* ]]
}

@test "provider: optional failure without --strict exits zero" {
  write_providers_toml <<'EOF'
[[providers]]
name = "optional"
type = "openshell"
description = "nice to have"
inputs = [
  { key = "MISSING", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["optional"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [ "$status" -eq 0 ]
  [[ "$output" == *"✓ Ready to launch"* ]]
}

@test "provider: custom type with upstream shows link on failure" {
  write_providers_toml <<'EOF'
[[providers]]
name = "future"
type = "custom"
description = "not yet native"
upstream = "https://github.com/NVIDIA/OpenShell/issues/1268"
inputs = [
  { key = "MISSING", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers-custom = ["future"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"upstream: https://github.com/NVIDIA/OpenShell/issues/1268"* ]]
}

@test "provider: custom type with upstream hides link on success" {
  write_providers_toml <<'EOF'
[[providers]]
name = "future"
type = "custom"
description = "not yet native"
upstream = "https://github.com/NVIDIA/OpenShell/issues/1268"
inputs = [
  { key = "PRESENT", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers-custom = ["future"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'
  export PRESENT="yes"

  run run_preflight check
  [[ "$output" != *"upstream:"* ]]
}

# ── Config filtering tests ───────────────────────────────────────────

@test "config: only enabled providers are checked" {
  write_providers_toml <<'EOF'
[[providers]]
name = "enabled"
type = "openshell"
description = "this one is on"
inputs = [
  { key = "ENABLED_VAR", kind = "env" },
]

[[providers]]
name = "disabled"
type = "openshell"
description = "this one is off"
inputs = [
  { key = "DISABLED_VAR", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["enabled"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'
  export ENABLED_VAR="yes"

  run run_preflight check
  [[ "$output" == *"✓ enabled"* ]]
  [[ "$output" != *"disabled"* ]]
}

@test "config: no config file enables all providers" {
  write_providers_toml <<'EOF'
[[providers]]
name = "alpha"
type = "openshell"
description = "first"
inputs = [
  { key = "A", kind = "env" },
]

[[providers]]
name = "beta"
type = "openshell"
description = "second"
inputs = [
  { key = "B", kind = "env" },
]
EOF
  rm -f "$CONFIG_TOML"
  make_stub openshell 'echo "openshell v0.0.54"'
  export A="yes" B="yes"

  run run_preflight check
  [[ "$output" == *"✓ alpha"* ]]
  [[ "$output" == *"✓ beta"* ]]
}

# ── CLI detection tests ──────────────────────────────────────────────

@test "cli: missing openshell shows error" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  # Symlink python3 into stubs but don't add openshell
  ln -sf "$(which python3)" "$STUB_DIR/python3"
  export PATH="$STUB_DIR:/usr/bin:/bin"

  run run_preflight check
  [[ "$output" == *"=== OpenShell CLI ==="* ]]
  [[ "$output" == *"✗ not found on PATH"* ]]
}

@test "cli: found openshell shows version and path" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'

  run run_preflight check
  [[ "$output" == *"✓ openshell v0.0.54"* ]]
  [[ "$output" == *"$STUB_DIR/openshell"* ]]
}

# ── Gateway tests ────────────────────────────────────────────────────

@test "podman gateway: not running shows dash" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell '
if [[ "$1" == "--version" ]]; then echo "openshell v0.0.54"; exit 0; fi
if [[ "$1" == "gateway" ]]; then echo "* openshell-local-podman  https://127.0.0.1:17670  local  mtls"; exit 0; fi
if [[ "$1" == "inference" ]]; then exit 1; fi
exit 1
'
  make_stub podman 'echo "podman version 5.0.0"'

  run run_preflight check
  [[ "$output" == *"=== Podman gateway ==="* ]]
  [[ "$output" == *"- Not running"* ]]
  [[ "$output" != *"=== K8s gateway ==="* ]]
}

@test "podman gateway: running shows model" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell '
if [[ "$1" == "--version" ]]; then echo "openshell v0.0.54"; exit 0; fi
if [[ "$1" == "gateway" ]]; then echo "* openshell-local-podman  https://127.0.0.1:17670  local  mtls"; exit 0; fi
if [[ "$1" == "inference" ]]; then echo "Model: claude-sonnet-4-6"; exit 0; fi
if [[ "$1" == "provider" ]]; then exit 0; fi
exit 1
'
  make_stub podman 'echo "podman version 5.0.0"'

  run run_preflight check
  [[ "$output" == *"✓ Reachable (model: claude-sonnet-4-6)"* ]]
  [[ "$output" == *"✓ Podman:"* ]]
  [[ "$output" != *"=== K8s gateway ==="* ]]
}

@test "k8s gateway: no cluster shows error" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell '
if [[ "$1" == "--version" ]]; then echo "openshell v0.0.54"; exit 0; fi
if [[ "$1" == "gateway" ]]; then echo "* openshell-remote-ocp  https://gw.example.com  remote  mtls"; exit 0; fi
exit 1
'
  make_stub kubectl 'exit 1'

  run run_preflight check
  [[ "$output" == *"=== K8s gateway ==="* ]]
  [[ "$output" == *"✗ No cluster"* ]]
  [[ "$output" != *"=== Podman gateway ==="* ]]
}

@test "k8s gateway: cluster with gateway shows reachable" {
  write_providers_toml <<'EOF'
[[providers]]
name = "test"
type = "openshell"
description = "test"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["test"]
EOF
  make_stub openshell '
if [[ "$1" == "--version" ]]; then echo "openshell v0.0.54"; exit 0; fi
if [[ "$1" == "gateway" ]]; then echo "* openshell-remote-ocp  https://gw.example.com  remote  mtls"; exit 0; fi
if [[ "$1" == "inference" ]]; then echo "Model: claude-sonnet-4-6"; exit 0; fi
if [[ "$1" == "provider" ]]; then exit 0; fi
exit 1
'
  make_stub kubectl '
if [[ "$1" == "config" ]]; then echo "my-ocp-cluster"; exit 0; fi
exit 0
'

  run run_preflight check
  [[ "$output" == *"✓ Cluster: my-ocp-cluster"* ]]
  [[ "$output" == *"✓ Gateway reachable"* ]]
  [[ "$output" != *"=== Podman gateway ==="* ]]
}

# ── available/names command tests ────────────────────────────────────

@test "available: returns only providers with all inputs satisfied" {
  write_providers_toml <<'EOF'
[[providers]]
name = "good"
type = "openshell"
description = "has inputs"
inputs = [
  { key = "GOOD", kind = "env" },
]

[[providers]]
name = "bad"
type = "openshell"
description = "missing inputs"
inputs = [
  { key = "MISSING", kind = "env" },
]

[[providers]]
name = "custom"
type = "custom"
description = "custom type"
inputs = [
  { key = "CUSTOM", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["good", "bad"]
providers-custom = ["custom"]
EOF
  export GOOD="yes" CUSTOM="yes"

  run run_preflight available
  [ "$status" -eq 0 ]
  [[ "$output" == "good" ]]
}

@test "names: returns all enabled openshell provider names" {
  write_providers_toml <<'EOF'
[[providers]]
name = "alpha"
type = "openshell"
description = "first"
inputs = []

[[providers]]
name = "beta"
type = "openshell"
description = "second"
inputs = []

[[providers]]
name = "gamma"
type = "custom"
description = "custom"
inputs = []
EOF
  write_config_toml <<'EOF'
providers = ["alpha", "beta"]
providers-custom = ["gamma"]
EOF

  run run_preflight names
  [ "$status" -eq 0 ]
  [[ "$output" == "alpha beta" ]]
}

# ── Full scenario tests ──────────────────────────────────────────────

@test "full: all providers configured shows ready" {
  write_providers_toml <<'EOF'
[[providers]]
name = "github"
type = "openshell"
description = "GitHub"
required = true
inputs = [
  { key = "GITHUB_TOKEN", kind = "env", secret = true },
]

[[providers]]
name = "atlassian"
type = "openshell"
description = "Jira"
inputs = [
  { key = "JIRA_API_TOKEN", kind = "env", secret = true },
  { key = "JIRA_URL", kind = "env" },
  { key = "JIRA_USERNAME", kind = "env" },
]
EOF
  write_config_toml <<'EOF'
providers = ["github", "atlassian"]
EOF
  make_stub openshell '
if [[ "$1" == "--version" ]]; then echo "openshell v0.0.54"; exit 0; fi
if [[ "$1" == "inference" ]]; then echo "Model: claude-sonnet-4-6"; exit 0; fi
if [[ "$1" == "provider" && "$2" == "get" ]]; then exit 0; fi
exit 1
'

  export GITHUB_TOKEN="ghp_abc123"
  export JIRA_API_TOKEN="ATATT_secret_token"
  export JIRA_URL="https://mysite.atlassian.net"
  export JIRA_USERNAME="user@example.com"

  run run_preflight check
  [ "$status" -eq 0 ]
  [[ "$output" == *"✓ github"* ]]
  [[ "$output" == *"✓ atlassian"* ]]
  [[ "$output" == *"✓ Ready to launch"* ]]
  # Secrets are masked
  [[ "$output" == *"ghp_***"* ]]
  [[ "$output" == *"ATAT***"* ]]
  [[ "$output" != *"ghp_abc123"* ]]
}

@test "full: required provider missing with --strict fails" {
  write_providers_toml <<'EOF'
[[providers]]
name = "github"
type = "openshell"
description = "GitHub"
required = true
inputs = [
  { key = "GITHUB_TOKEN", kind = "env", secret = true },
]

[[providers]]
name = "atlassian"
type = "openshell"
description = "Jira"
inputs = [
  { key = "JIRA_API_TOKEN", kind = "env", secret = true },
]
EOF
  write_config_toml <<'EOF'
providers = ["github", "atlassian"]
EOF
  make_stub openshell 'echo "openshell v0.0.54"'
  # GITHUB_TOKEN not set (required), JIRA_API_TOKEN not set (optional)

  run run_preflight check --strict
  [ "$status" -ne 0 ]
  [[ "$output" == *"✗ github"* ]]
  [[ "$output" == *"✗ atlassian"* ]]
  [[ "$output" == *"✗ Not ready"* ]]
}
