# Proto-Based Profile Format Migration

Migrate harness-openshell's profile and provider profile formats from
hand-rolled TOML/YAML to OpenShell's proto-generated Go types. The proto
becomes the schema; the serialization format (textproto, JSON, or TOML
with a mapping layer) is a separate decision.

## Why

### Upstream alignment

OpenShell's `SandboxSpec`, `ProviderProfile`, and related messages are the
canonical types that the gateway operates on. Every field harness-openshell
cares about already exists in the proto:

| harness-openshell concept | Proto message |
|---------------------------|---------------|
| Sandbox profile (`profiles/*.toml`) | `CreateSandboxRequest` → `SandboxSpec` → `SandboxTemplate` |
| Provider profile (`sandbox/profiles/*.yaml`) | `ProviderProfile` |
| Provider credentials | `ProviderProfileCredential` |
| Network endpoints | `NetworkEndpoint` (from `sandbox.proto`) |
| Provider catalog (`providers.toml`) | Partially: `ProviderProfileDiscovery` + `ProviderProfileCredential.env_vars` |

When OpenShell adds fields (e.g., `user_namespaces`, `volume_claim_templates`,
credential refresh material), re-running `protoc` makes them available
immediately. No manual struct updates, no drift.

### Compile-time safety

Today, profile fields are parsed from TOML into a hand-written `Config`
struct. If OpenShell renames or restructures a field, the harness discovers
this at runtime when CLI flags fail. With proto-generated types, a field
change is a compile error.

### Format for the future

The proto types are the same ones a gRPC client would use. If/when
harness-openshell moves from CLI exec to direct gRPC, the profile structs
are already the request payloads — no translation step.

## What changes

### Sandbox profiles

**Today:** `profiles/default.toml` → parsed into `profile.Config` struct →
translated to `openshell sandbox create --name X --image Y --provider Z` flags.

**After:** `profiles/default.textproto` (or `.json`) → unmarshalled into
generated `openshellv1.CreateSandboxRequest` + a `HarnessConfig` wrapper →
same CLI flags, but sourced from the proto struct.

Harness-only fields (`command`, `keep`) have no proto equivalent. These
live in a wrapper:

```go
type Profile struct {
    Request *openshellv1.CreateSandboxRequest
    Command string
    Keep    bool
}
```

### Provider profiles

**Today:** `sandbox/profiles/atlassian.yaml` → hand-written YAML matching
OpenShell's profile schema → imported via `openshell provider profile import`.

**After:** `sandbox/profiles/atlassian.textproto` → unmarshalled into
generated `openshellv1.ProviderProfile` → serialized to YAML or JSON for
`openshell provider profile import`, or passed directly if/when using gRPC.

### Provider catalog (providers.toml)

`providers.toml` tracks preflight validation inputs and custom provider
workarounds. The proto doesn't cover preflight checks (`kind = "file"`,
`kind = "check"`) or the `upstream` tracking field. This file stays as-is —
it's harness-specific orchestration, not an upstream type.

Over time, `providers.toml` shrinks as:
- OpenShell's `ProviderProfileDiscovery` handles more credential discovery
- Credential verification on create ships upstream (#896 roadmap)
- Custom providers migrate to native OpenShell provider profiles

### Bash path

The bash path (`profile.sh` + `parse-profile.py`) currently reads TOML.
Options when the format changes:

| Option | Tradeoff |
|--------|----------|
| **Go shim** (`harness profile dump NAME` → shell vars) | One parser, two consumers. Bash calls Go. Cleanest. |
| **Swap Python parser** (protobuf or json module) | Keeps bash self-contained but adds protobuf pip dependency. |
| **Drop bash path** | Simplest, but premature if Go migration isn't complete. |

**Recommendation:** Go shim. Add a `harness profile dump` subcommand that
reads the proto-format profile and emits `SANDBOX_NAME=...` shell variable
assignments. `parse-profile.py` is replaced by a one-line call to the Go
binary. No new dependencies in the bash path, no duplicated parsing logic.

## Tradeoffs

### Serialization format

| Format | Pros | Cons |
|--------|------|------|
| **Textproto** | Comments. Snake_case field names match proto exactly. `prototext.Unmarshal` in Go. | Map entries are verbose (`key:` / `value:` pairs). Less familiar to most users. |
| **Proto JSON** | Universal. `protojson.Unmarshal` in Go. Easy to generate/consume from other tools. | No comments. camelCase field names (protobuf JSON convention). |
| **TOML (keep, marshal internally)** | Best authoring UX. Users don't need to learn a new format. | Manual mapping layer between TOML keys and proto fields. Defeats some of the alignment benefit. |

**Recommendation:** Textproto for provider profiles (they read cleanly,
comments are valuable for documenting credential vs. config distinctions).
For sandbox profiles, textproto works but the `environment` map is ugly —
proto JSON may be more readable there. Support both via file extension
detection (`.textproto` → `prototext`, `.json` → `protojson`).

### Proto stability

OpenShell is alpha (v0.0.58). The proto could change. However:

- `SandboxSpec` core fields (`image`, `environment`, `providers`) are stable
  — they map directly to the CLI's `sandbox create` flags which have been
  stable since v0.0.20+.
- `ProviderProfile` shipped with providers v2 and has a published schema in
  the docs. Breaking changes would affect all provider profile YAML users,
  not just harness-openshell.
- Proto changes produce compile errors, which is strictly better than
  discovering CLI output format changes at runtime.
- If a field is removed or renamed, the fix is: re-generate, update the one
  or two references in harness code, done.

### Complexity budget

Adding `protoc` to the build introduces:
- Proto files vendored (4 files: `openshell.proto`, `datamodel.proto`,
  `sandbox.proto`, `inference.proto`)
- `protoc-gen-go` and `protoc-gen-go-grpc` as build-time dependencies
- A `make proto` (or `buf generate`) target
- Generated `.pb.go` files checked in (or generated in CI)

This is standard Go infrastructure but it's not zero. The project currently
has no protobuf dependency.

## Implementation plan

### Phase 1: Vendor proto, generate types

1. Create `proto/` directory, vendor the 4 proto files from NVIDIA/OpenShell
2. Add `buf.yaml` + `buf.gen.yaml` (or a `make proto` target with `protoc`)
3. Generate Go types into `internal/openshell/` (or `pkg/openshell/`)
4. Add generated files to the repo (avoid requiring protoc for contributors)
5. Verify: generated `CreateSandboxRequest`, `SandboxSpec`, `ProviderProfile`
   types compile and match the upstream schema

### Phase 2: Migrate provider profiles

Start here because the provider profile format is the cleanest mapping —
almost 1:1 with the existing `atlassian.yaml`.

1. Convert `sandbox/profiles/atlassian.yaml` → `sandbox/profiles/atlassian.textproto`
2. Update `cmd/providers.go` to unmarshal via `prototext.Unmarshal` into
   `*openshellv1.ProviderProfile`
3. For `openshell provider profile import`: serialize the proto struct to
   YAML (or JSON) since the CLI expects that format, or write a temp file
4. Update tests
5. Verify: `harness providers` still registers the atlassian profile correctly

### Phase 3: Migrate sandbox profiles

1. Define the `Profile` wrapper struct (proto `CreateSandboxRequest` +
   harness-only `Command` and `Keep` fields)
2. Convert `profiles/default.toml` → `profiles/default.textproto` (or `.json`)
   with a sidecar section or separate file for harness-only fields
3. Update `internal/profile/` to parse the new format into the wrapper struct
4. Update `internal/profile/` to translate the proto struct into CLI flags
   (same logic as today, different source struct)
5. Add `harness profile dump NAME` subcommand for bash path compatibility
6. Replace `parse-profile.py` call in `profile.sh` with `harness profile dump`
7. Update tests (Go unit tests + bats integration)
8. Verify: `make validate` passes on all matrix combinations

### Phase 4: Cleanup

1. Remove `parse-profile.py`
2. Remove old `profile.Config` struct (replaced by wrapper + proto types)
3. Update docs and `profile-concepts.md` to reflect the new format
4. Document the format in a comment block or README section

## Decision: harness-only fields

Two fields exist in harness profiles that have no proto equivalent:

| Field | Why it's harness-only | Where it goes |
|-------|----------------------|---------------|
| `command` | Passed at `sandbox connect` / `sandbox exec` time, not at creation | Wrapper struct |
| `keep` | Client-side lifecycle decision (keep sandbox after command exits) | Wrapper struct |

Options for encoding these alongside the proto:

1. **Wrapper struct with separate parsing** — proto fields in `.textproto`,
   harness fields as Go defaults or a small sidecar `harness.toml`.
2. **Proto extensions** — technically possible but overkill and non-standard.
3. **Comments-as-config** — parse specially formatted comments in the
   textproto. Fragile, don't do this.
4. **Custom proto message** — define a `HarnessProfile` message that embeds
   `CreateSandboxRequest`. Cleanest if we're already running protoc.

**Recommendation:** Option 4. Define a small `harness.proto`:

```protobuf
syntax = "proto3";
package harness.v1;
import "openshell.proto";

message Profile {
  openshell.v1.CreateSandboxRequest request = 1;
  string command = 2;      // default: "claude --bare"
  bool keep = 3;            // default: true
  string description = 4;   // human-readable profile description
}
```

This gives you one file, one parse call, full type safety, and a natural
place for any future harness-only fields.

## References

- [OpenShell proto](https://github.com/NVIDIA/OpenShell/blob/main/proto/openshell.proto)
- [OpenShell providers v2 docs](https://github.com/NVIDIA/OpenShell/blob/main/docs/sandboxes/providers-v2.mdx)
- [OpenShell #896 — Enhanced Provider Management](https://github.com/NVIDIA/OpenShell/issues/896)
- [Kaiden #1272 — Projects management](https://github.com/openkaiden/kaiden/issues/1272)
- [profile-concepts.md](profile-concepts.md) — cross-project profile concept analysis
