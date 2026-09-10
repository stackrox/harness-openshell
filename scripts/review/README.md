# Review execution components

These scripts contain behavior that can be reused by multiple workflow
archetypes. They accept explicit paths and environment inputs; they do not own
provider credentials, workflow policy, model selection, or GitHub permissions.

Current component:

- `validate-agent-output.sh REVIEW_DIR` validates the bounded JSON event stream
  emitted by an agent run.

The PR-specific wrapper remains in `scripts/pr-review.sh` until a second
workflow demonstrates a stable context or lifecycle contract. Future
extractions should preserve this boundary: reusable components validate and
guard execution, while each workflow selects its agent, policy, providers, and
publication behavior.
