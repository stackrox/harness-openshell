# Review execution components

These scripts contain behavior that can be reused by multiple workflow
archetypes. They accept explicit paths and environment inputs; they do not own
provider credentials, workflow policy, model selection, or GitHub permissions.

Current component:

- `validate-agent-output.sh REVIEW_DIR` validates the bounded OpenCode event
  stream emitted by the PR reviewer. It rejects malformed event-looking lines,
  requires text and a terminal stop event, and recognizes narrow exceptions
  for comment-position and shell parser tool failures. It checks event-stream
  completion, not finding correctness or whether every external action was
  appropriate. Comments can already have been posted when this check runs.

This validator is intentionally scoped to the PR-review workflow until a second
workflow demonstrates a stable event and publication contract. It is not a
generic agent-result protocol.

The PR-specific wrapper remains in `scripts/pr-review.sh` until a second
workflow demonstrates a stable context or lifecycle contract. Future
extractions should preserve this boundary: reusable components validate and
guard execution, while each workflow selects its agent, policy, providers, and
publication behavior.
