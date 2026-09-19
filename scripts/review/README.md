# Review execution components

These scripts contain behavior that can be reused by multiple workflow
archetypes. They accept explicit paths and environment inputs; they do not own
provider credentials, workflow policy, model selection, or GitHub permissions.

Current components:

- `agents/codex.sh` and `agents/opencode.sh` are the two PR-review agent
  profiles. Each profile owns its trusted task workflow, output validator,
  output extraction, and any provider/workspace setup. The shared wrapper does
  not need to know how an agent is provisioned.

- `validate-agent-output.sh REVIEW_DIR` validates the bounded OpenCode event
  stream emitted by the PR reviewer. It rejects malformed event-looking lines,
  requires text and a terminal stop event, and recognizes narrow exceptions
  for comment-position and shell parser tool failures. It checks event-stream
  completion, not finding correctness or whether every external action was
  appropriate. Comments can already have been posted when this check runs.

These validators are intentionally scoped to the PR-review workflow until a
second workflow demonstrates a stable event and publication contract. They are
not a generic agent-result protocol.

The PR-specific lifecycle remains in `scripts/pr-review.sh`; future agent
removal should delete the corresponding profile and task bundle without
changing PR eligibility, diff integrity, sandbox cleanup, or artifact handling.

The reusable CI workflow enables the Codex profile's ephemeral provider
bootstrap for each review run; local managed workspaces can leave that setup
disabled and use their pre-provisioned providers instead.

The workflow uploads the bounded review directory as an artifact so a failed
run can be diagnosed without exposing provider credentials.
