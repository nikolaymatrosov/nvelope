---
name: speckit-git-commit
description: Auto-commit changes after a Spec Kit command completes
compatibility: Requires spec-kit project structure with .specify/ directory
metadata:
  author: github-spec-kit
  source: git:commands/speckit.git.commit.md
---

# Auto-Commit Changes

Automatically stage and commit all changes after a Spec Kit command completes.

## Behavior

This command is invoked as a hook after (or before) core commands. It:

1. Determines the event name from the hook context (e.g., `after_specify`, `before_plan`, `after_implement`)
2. Checks `.specify/extensions/git/git-config.yml` for the `auto_commit` section
3. Looks up the specific event key to see if auto-commit is enabled
4. Falls back to `auto_commit.default` if no event-specific key exists
5. For `after_implement`, **drafts a Conventional Commits message that describes intent** (see below) and passes it to the script
6. For other events, uses the per-command `message` from config (or a default)
7. If enabled and there are uncommitted changes, runs `git add .` + `git commit`

## Drafting the message for `after_implement`

For `after_implement` only, draft the commit message yourself before invoking
the script. Do **not** delegate this to the script's heuristic — that produces
useless summaries like `feat(audience,campaign): add 8, update 15 files` that
hide what actually changed.

Steps:

1. **Stage first so you can see the full diff:** `git add -A`
2. **Look at the change set:**
   - `git diff --cached --stat` for scope/breadth
   - `git diff --cached` (or spot-check the largest files) for what actually changed
3. **Read the active feature's intent** from `specs/<current-branch>/spec.md`
   and `specs/<current-branch>/tasks.md` — the subject should match the
   user-facing goal, not the file layout. If the implement run only completed
   a slice of tasks, describe that slice, not the whole feature.
4. **Compose the message:**
   - **Subject** (≤ 72 chars): `<type>(<scope>): <imperative summary>`
     - `type`: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`
     - `scope`: the dominant area touched (e.g. `audience`, `campaign`, `frontend`, `billing`). Use one scope, two only when the change is genuinely split.
     - Summary: imperative mood, describes the *behavior change* — not file counts, not "add N files". Examples:
       - GOOD: `feat(audience): import contacts from CSV uploads`
       - GOOD: `fix(campaign): prevent double-send on retry`
       - BAD: `feat(audience,campaign): add 8, update 15 files`
       - BAD: `chore: spec kit implementation progress`
   - **Body** (optional, recommended for non-trivial changes): 1–4 short bullets describing the user-visible or behavioral changes. Skip file lists and stats — `git log --stat` already shows those. Mention migrations, breaking changes, or new env vars if any.

## Execution

Determine the event name from the hook that triggered this command.

For `after_implement`, draft the message per the section above, then run:

- **Bash**: `.specify/extensions/git/scripts/bash/auto-commit.sh after_implement --message "<subject>" --body "<body>"`
- **PowerShell**: `.specify/extensions/git/scripts/powershell/auto-commit.ps1 after_implement -Message "<subject>" -Body "<body>"`

For all other events, run the script without overrides — the per-event message
from `git-config.yml` is used:

- **Bash**: `.specify/extensions/git/scripts/bash/auto-commit.sh <event_name>`
- **PowerShell**: `.specify/extensions/git/scripts/powershell/auto-commit.ps1 <event_name>`

If you cannot determine intent (no spec, no tasks file, opaque diff), invoke
the script without `--message` and let the heuristic fallback handle it rather
than fabricating a subject.

## Configuration

In `.specify/extensions/git/git-config.yml`:

```yaml
auto_commit:
  default: false          # Global toggle — set true to enable for all commands
  after_specify:
    enabled: true          # Override per-command
    message: "[Spec Kit] Add specification"
  after_plan:
    enabled: false
    message: "[Spec Kit] Add implementation plan"
  after_implement:
    enabled: true
    # message is drafted by the skill from spec/tasks/diff; the value here is
    # only a fallback if the skill omits --message and the heuristic yields
    # nothing.
    message: "chore: spec kit implementation progress"
```

## Graceful Degradation

- If Git is not available or the current directory is not a repository: skips with a warning
- If no config file exists: skips (disabled by default)
- If no changes to commit: skips with a message
- If `--message` is omitted on `after_implement`: falls back to the staged-diff heuristic, then to the config message
