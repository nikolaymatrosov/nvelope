#!/usr/bin/env bash
# Run frontend (pnpm) tests and report the real exit code.
#
# Wrapped in a script so Claude Code can allowlist a single command instead of a
# compound `cd ...; pnpm test ... | tail; echo ${PIPESTATUS[0]}` pipeline, which
# prefix-based permission rules cannot safely match (so it prompts every time).
#
# Usage:
#   .claude/scripts/test-frontend.sh                # run the whole suite
#   .claude/scripts/test-frontend.sh src/i18n       # run a path/pattern
#
# Always run from the repo root; this script cd's into frontend/ itself.

set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root/frontend" || { echo "frontend/ not found" >&2; exit 1; }

pnpm test "$@" 2>&1 | tail -25
status=${PIPESTATUS[0]}
echo "EXIT:${status}"
exit "$status"
