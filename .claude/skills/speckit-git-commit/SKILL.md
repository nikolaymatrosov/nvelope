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
the script. The script's heuristic only counts files (`add 8, update 15
files`) and the config message is generic (`chore: spec kit implementation
progress`) — neither tells a future reader what the commit actually does.

**The single rule: the message describes what is in the staged diff. Nothing
more, nothing less.** A future reader running `git show <sha>` should be able
to verify every claim in the message against the hunks. If you can't point to
a hunk that supports a bullet, the bullet does not belong.

### The diff is the only source of truth

- `spec.md`, `tasks.md`, `plan.md`, and conversation context are **off-limits
  as content sources**. The spec describes what the feature *should eventually*
  do; the diff is what was *actually committed*. These differ — an
  `/speckit-implement` run rarely lands a whole feature in one commit, and the
  staged set may include unrelated WIP that got swept up by `git add .`.
- You may consult those files only as a **vocabulary aid** — e.g., to learn
  that an entity is called `TenantBranding` or that a permission is named
  `branding:manage` — but every concept you mention must also appear in the
  diff. If the spec talks about an "RSS feed" and the diff has no RSS handler,
  do not mention RSS.
- Past Claude sessions have failed this rule by copying the spec's
  whole-feature framing into the message even when only a slice was
  implemented, and by inventing details ("rejects draft via
  `ErrCampaignNotSent`; idempotent") that sounded plausible but were not
  actually in the diff. Don't do this.

### Steps

1. **Stage** so the diff is observable: `git add -A`
2. **Survey breadth:** `git diff --cached --stat`
3. **Read the diff itself**, not just the stat. For each non-trivial hunk,
   note in one short phrase what the code now does that it didn't before
   (added handler, new column, new validation rule, fixed off-by-one, etc.).
   Use the file's package / function names as anchors.
4. **Group the notes by behavior**, not by file. Several files implementing
   one capability become one bullet.
5. **Compose:**
   - **Subject** (≤ 72 chars): `<type>(<scope>): <imperative summary>`
     - `type`: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`
     - `scope`: the dominant package/area in the diff (e.g. `audience`,
       `campaign`, `frontend`, `billing`). One scope; two only when the
       change is genuinely split across two equal areas.
     - Summary: imperative, names the **behavior change** the diff produces.
       - GOOD: `feat(audience): import contacts from CSV uploads`
       - GOOD: `fix(campaign): prevent double-send on retry`
       - BAD: `feat(audience,campaign): add 8, update 15 files` *(file counts, not behavior)*
       - BAD: `chore: spec kit implementation progress` *(says nothing)*
       - BAD: `feat(campaign,tenant): campaign archive, RSS, and per-tenant branding` *when the diff only contains the archive slice* *(spec framing, not diff reality)*
   - **Body** (recommended for non-trivial diffs): 1–4 short bullets, each
     traceable to specific hunks. Name the symbols (`SetArchiveVisible`,
     `tenant_branding`, `migration 000018`) so a reader can grep. Mention
     migrations, new env vars, and breaking changes when present. Skip file
     counts, line counts, and `git log --stat` will-show-this kind of detail.

### Verification before invoking the script

Before calling `auto-commit.sh ... --message ...`, re-read your draft and
check each bullet against `git diff --cached`. If a bullet cannot be tied to
a hunk, delete it or rewrite it. It is better to have a short, true message
than a long, partially-fabricated one.

## Execution

Determine the event name from the hook that triggered this command.

For `after_implement`, draft the message per the section above, then run:

- **Bash**: `.specify/extensions/git/scripts/bash/auto-commit.sh after_implement --message "<subject>" --body "<body>"`
- **PowerShell**: `.specify/extensions/git/scripts/powershell/auto-commit.ps1 after_implement -Message "<subject>" -Body "<body>"`

For all other events, run the script without overrides — the per-event message
from `git-config.yml` is used:

- **Bash**: `.specify/extensions/git/scripts/bash/auto-commit.sh <event_name>`
- **PowerShell**: `.specify/extensions/git/scripts/powershell/auto-commit.ps1 <event_name>`

If the diff is truly opaque (e.g., a generated-file bump where you cannot say
what behavior changed), invoke the script without `--message` and let the
heuristic fallback handle it rather than fabricating a subject.

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
    # message is drafted by the skill from the staged diff (see SKILL.md);
    # the value here is only a fallback if the skill omits --message AND the
    # script's heuristic yields nothing.
    message: "chore: spec kit implementation progress"
```

## Graceful Degradation

- If Git is not available or the current directory is not a repository: skips with a warning
- If no config file exists: skips (disabled by default)
- If no changes to commit: skips with a message
- If `--message` is omitted on `after_implement`: falls back to the staged-diff heuristic, then to the config message
