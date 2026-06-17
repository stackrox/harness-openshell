# Configuration Test Suite

Tests the harness CLI with different agent configs, provider combinations, credential styles, and output formats. Complements `test-flow.sh` (full deploy/sandbox lifecycle) by focusing on config resolution and validation.

## Usage

```bash
make test-suite              # config tests (most need no gateway)
make test-suite-live         # includes live sandbox create/delete
./test/suite/run.sh --filter parse   # run only tests matching "parse"
```

## Test categories

| Category | Tests | Needs gateway |
|----------|-------|---------------|
| Config parsing | 10 configs across minimal, multi-provider, multi-doc, task | --dry-run only |
| Output formats | -o yaml/json on apply and get commands | get needs gateway |
| Env resolution | static vars, host expansion, provider env | No |
| CLI flags | --agent, --name, mutually exclusive flags | --dry-run only |
| Describe/delete | error cases | No |
| Live sandbox | create, exec, env check, delete | Yes |
| Free API providers | dry-run with Groq, OpenRouter, NVIDIA NIM keys | No |

## Free LLM API setup (optional)

These providers offer free API keys with no credit card. Set the env vars to enable those test cases.

### Groq (fastest, 30k tokens/min)

1. Go to https://console.groq.com/keys
2. Sign up with email (no credit card)
3. Create an API key

```bash
export GROQ_API_KEY=gsk_...
```

### OpenRouter (20+ free models)

1. Go to https://openrouter.ai/keys
2. Sign up with email
3. Create an API key

```bash
export OPENROUTER_API_KEY=sk-or-...
```

### NVIDIA NIM (no daily token cap)

1. Go to https://build.nvidia.com
2. Sign in with NVIDIA account
3. Get an API key from any model page

```bash
export NVIDIA_API_KEY=nvapi-...
```

### Run with free API keys

```bash
export GROQ_API_KEY=gsk_...
export OPENROUTER_API_KEY=sk-or-...
export NVIDIA_API_KEY=nvapi-...
make test-suite
```

## Adding tests

Add new agent configs to `test/configs/`. The naming convention:
- `agent-*.yaml` -- single-doc agent configs
- `harness-*.yaml` -- multi-doc harness configs with inline providers/gateways

Add test cases to `test/suite/run.sh` using:
- `run_test "name" command args` -- expects success
- `run_test_expect_fail "name" command args` -- expects failure
- `skip_test "name" "reason"` -- skipped with message
