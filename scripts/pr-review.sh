#!/usr/bin/env bash
# Compatibility entry point. Managed targets should call harness github review.
set -euo pipefail
cd "$(dirname "$0")/.."
phase="${1:?usage: pr-review.sh prepare|run}"
shift
case "$phase" in
  prepare) exec ./harness github review prepare "$@" ;;
  run)
    args=()
    [[ -z "${REVIEW_SKILL:-}" ]] || args+=(--skill "$REVIEW_SKILL")
    [[ -z "${REVIEW_POLICY_TEMPLATE:-}" ]] || args+=(--policy-template "$REVIEW_POLICY_TEMPLATE")
    exec bash scripts/pr-review-local.sh "${args[@]}" "$@" ;;
  *) echo 'usage: pr-review.sh prepare|run' >&2; exit 1 ;;
esac
