---
name: running-frontend-tests
description: Use when running, re-running, or checking frontend (pnpm/vitest) tests in this repo — including a single file, path, or pattern (e.g. src/i18n), or the whole suite. Triggers on "run the frontend tests", "check if the i18n tests pass", flaky/failing vitest output, or any urge to type `cd frontend && pnpm test ...`.
---

# Running Frontend Tests

## Overview

Frontend tests run through a pre-approved wrapper script so they execute **without a permission prompt**. Always use the script — never hand-roll a `cd frontend ... | tail | echo` pipeline.

## The one command

```bash
.claude/scripts/test-frontend.sh            # whole suite
.claude/scripts/test-frontend.sh src/i18n   # a path, file, or vitest pattern
```

Run it from the repo root — the script `cd`s into `frontend/` itself, tails the output, and prints `EXIT:<code>` so the real pass/fail status is unambiguous.

## Why not the ad-hoc pipeline

`cd frontend 2>/dev/null; pnpm test … | tail -25; echo "EXIT:${PIPESTATUS[0]}"` prompts for permission **every time**. A compound command needs every segment (`cd … 2>/dev/null`, `tail`, `echo`) on the allowlist; they aren't. The script collapses it to a single invocation matched by the `Bash(.claude/scripts/test-frontend.sh:*)` allow rule in [.claude/settings.json](../../settings.json), so it never prompts.

## Common mistakes

- **Typing `cd frontend && pnpm test …` anyway** → prompts, and you lose the `EXIT:` line. Use the script.
- **Passing an absolute path** → pass a path relative to `frontend/` (e.g. `src/i18n`), since the script runs from there.
- **Trusting tailed output for pass/fail** → read the final `EXIT:0` line; `tail` can hide an earlier failure summary.
