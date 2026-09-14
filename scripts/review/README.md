# Review compatibility entry points

Review orchestration and OpenCode event validation now live in the
[Go GitHub adapter](../../integrations/github/review/), exposed through
`harness github review prepare|run|validate-output`.

`validate-agent-output.sh REVIEW_DIR` delegates to that validator and requires a
built `harness` binary. `../pr-review.sh prepare|run` preserves the old local
entry point. New callers should use the review CLI directly against existing
resources, or `../pr-review-local.sh` for temporary local provider setup.

These wrappers contain no separate parsing, task orchestration or sandbox
cleanup implementation. Provider provisioning remains outside the Go CLI.
