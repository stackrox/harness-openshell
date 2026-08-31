# Archived Documentation

This directory contains historical design documents that are no longer current but may have valuable context.

## design-v1-2026-06-08.md

Original design proposal from early development. **Outdated** — uses old naming (`harness.toml` vs `openshell.toml`, `profiles/*.toml` vs `agents/*.yaml`). Current implementation diverged from this design on 2026-06-08.

**Still valuable sections:**
- Provider health two-level model (lines 208-244)
- OAuth-proxy auth roadmap for OCP (lines 435-476)
- OCP vs local k8s comparison (lines 397-421)

For current architecture, see: README.md, SPEC.md, and AGENTS.md in the repo root.

## release-plan-2026-06.md

Original CI, embedding, and GoReleaser rollout plan. Phase 0 shipped, but later
implementation and filesystem details diverged. Retained for provenance; the
active roadmap is [`../../TODO.md`](../../TODO.md).
