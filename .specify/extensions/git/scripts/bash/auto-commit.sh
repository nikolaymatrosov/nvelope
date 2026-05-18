#!/usr/bin/env bash
# Git extension: auto-commit.sh
# Automatically commit changes after a Spec Kit command completes.
# Checks per-command config keys in git-config.yml before committing.
#
# Usage: auto-commit.sh <event_name>
#   e.g.: auto-commit.sh after_specify

set -e

EVENT_NAME="${1:-}"
if [ -z "$EVENT_NAME" ]; then
    echo "Usage: $0 <event_name>" >&2
    exit 1
fi

SCRIPT_DIR="$(CDPATH="" cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

_find_project_root() {
    local dir="$1"
    while [ "$dir" != "/" ]; do
        if [ -d "$dir/.specify" ] || [ -d "$dir/.git" ]; then
            echo "$dir"
            return 0
        fi
        dir="$(dirname "$dir")"
    done
    return 1
}

# Build a Conventional Commits message from the currently staged changes.
# Prints the subject on line 1, a blank line, then the body. Heuristic only:
# it summarizes what changed (type, scopes, file counts), not the intent.
_generate_commit_message() {
    local status_lines all_files file_count
    status_lines=$(git diff --cached --name-status 2>/dev/null)
    [ -z "$status_lines" ] && return 0

    all_files=$(printf '%s\n' "$status_lines" | awk '{print $NF}')
    file_count=$(printf '%s\n' "$all_files" | sed '/^$/d' | wc -l | tr -d ' ')

    # Commit type: docs-only, test-only, otherwise feature work
    local non_doc non_test ctype
    non_doc=$(printf '%s\n' "$all_files" | grep -Ev '(^docs/|\.md$)' || true)
    non_test=$(printf '%s\n' "$all_files" | grep -Ev '(_test\.go$|\.test\.[jt]sx?$|/tests?/)' || true)
    if [ -z "$non_doc" ]; then
        ctype="docs"
    elif [ -z "$non_test" ]; then
        ctype="test"
    else
        ctype="feat"
    fi

    # Map each file to a scope, then rank scopes by file count
    local scope_ranking scope_total scope1 scope2 scope_label
    scope_ranking=$(printf '%s\n' "$all_files" | sed -E \
        -e 's#^[^/]+$#repo#' \
        -e 's#^internal/([^/]+)/.*#\1#' \
        -e 's#^cmd/([^/]+)/.*#\1#' \
        -e 's#^frontend/.*#frontend#' \
        -e 's#^specs?/.*#spec#' \
        -e 's#^\.specify/.*#specify#' \
        -e 's#^([^/]+)/.*#\1#' \
        | sed '/^$/d' | sort | uniq -c | sort -rn)
    scope_total=$(printf '%s\n' "$scope_ranking" | sed '/^$/d' | wc -l | tr -d ' ')
    scope1=$(printf '%s\n' "$scope_ranking" | sed -n '1p' | awk '{print $2}')
    scope2=$(printf '%s\n' "$scope_ranking" | sed -n '2p' | awk '{print $2}')
    if [ "$scope_total" -le 1 ]; then
        scope_label="$scope1"
    else
        scope_label="${scope1},${scope2}"
    fi

    # Added / modified / deleted counts for the subject
    local n_add n_mod n_del parts subject
    n_add=$(printf '%s\n' "$status_lines" | grep -c '^A' || true)
    n_mod=$(printf '%s\n' "$status_lines" | grep -c '^M' || true)
    n_del=$(printf '%s\n' "$status_lines" | grep -c '^D' || true)
    parts=""
    [ "$n_add" -gt 0 ] && parts="add ${n_add}"
    [ "$n_mod" -gt 0 ] && parts="${parts:+$parts, }update ${n_mod}"
    [ "$n_del" -gt 0 ] && parts="${parts:+$parts, }remove ${n_del}"
    [ -z "$parts" ] && parts="change ${file_count}"
    local noun="files"
    [ "$file_count" = "1" ] && noun="file"
    if [ "$scope_label" = "$ctype" ]; then
        subject="${ctype}: ${parts} ${noun}"
    else
        subject="${ctype}(${scope_label}): ${parts} ${noun}"
    fi

    # Body: per-scope file counts plus git's own shortstat
    local scope_body shortstat
    scope_body=$(printf '%s\n' "$scope_ranking" | sed '/^$/d' \
        | awk '{print "- " $2 ": " $1 " file(s)"}')
    shortstat=$(git diff --cached --shortstat 2>/dev/null | sed 's/^ *//')

    printf '%s\n\n%s\n\n%s\n' "$subject" "$scope_body" "$shortstat"
}

REPO_ROOT=$(_find_project_root "$SCRIPT_DIR") || REPO_ROOT="$(pwd)"
cd "$REPO_ROOT"

# Check if git is available
if ! command -v git >/dev/null 2>&1; then
    echo "[specify] Warning: Git not found; skipped auto-commit" >&2
    exit 0
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "[specify] Warning: Not a Git repository; skipped auto-commit" >&2
    exit 0
fi

# Read per-command config from git-config.yml
_config_file="$REPO_ROOT/.specify/extensions/git/git-config.yml"
_enabled=false
_commit_msg=""

if [ -f "$_config_file" ]; then
    # Parse the auto_commit section for this event.
    # Look for auto_commit.<event_name>.enabled and .message
    # Also check auto_commit.default as fallback.
    _in_auto_commit=false
    _in_event=false
    _default_enabled=false

    while IFS= read -r _line; do
        # Detect auto_commit: section
        if echo "$_line" | grep -q '^auto_commit:'; then
            _in_auto_commit=true
            _in_event=false
            continue
        fi

        # Exit auto_commit section on next top-level key
        if $_in_auto_commit && echo "$_line" | grep -Eq '^[a-z]'; then
            break
        fi

        if $_in_auto_commit; then
            # Check default key
            if echo "$_line" | grep -Eq "^[[:space:]]+default:[[:space:]]"; then
                _val=$(echo "$_line" | sed 's/^[^:]*:[[:space:]]*//' | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
                [ "$_val" = "true" ] && _default_enabled=true
            fi

            # Detect our event subsection
            if echo "$_line" | grep -Eq "^[[:space:]]+${EVENT_NAME}:"; then
                _in_event=true
                continue
            fi

            # Inside our event subsection
            if $_in_event; then
                # Exit on next sibling key (same indent level as event name)
                if echo "$_line" | grep -Eq '^[[:space:]]{2}[a-z]' && ! echo "$_line" | grep -Eq '^[[:space:]]{4}'; then
                    _in_event=false
                    continue
                fi
                if echo "$_line" | grep -Eq '[[:space:]]+enabled:'; then
                    _val=$(echo "$_line" | sed 's/^[^:]*:[[:space:]]*//' | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
                    [ "$_val" = "true" ] && _enabled=true
                    [ "$_val" = "false" ] && _enabled=false
                fi
                if echo "$_line" | grep -Eq '[[:space:]]+message:'; then
                    _commit_msg=$(echo "$_line" | sed 's/^[^:]*:[[:space:]]*//' | sed 's/^["'\'']//' | sed 's/["'\'']*$//')
                fi
            fi
        fi
    done < "$_config_file"

    # If event-specific key not found, use default
    if [ "$_enabled" = "false" ] && [ "$_default_enabled" = "true" ]; then
        # Only use default if the event wasn't explicitly set to false
        # Check if event section existed at all
        if ! grep -q "^[[:space:]]*${EVENT_NAME}:" "$_config_file" 2>/dev/null; then
            _enabled=true
        fi
    fi
else
    # No config file — auto-commit disabled by default
    exit 0
fi

if [ "$_enabled" != "true" ]; then
    exit 0
fi

# Check if there are changes to commit
if git diff --quiet HEAD 2>/dev/null && git diff --cached --quiet 2>/dev/null && [ -z "$(git ls-files --others --exclude-standard 2>/dev/null)" ]; then
    echo "[specify] No changes to commit after $EVENT_NAME" >&2
    exit 0
fi

# Derive a human-readable command name from the event
# e.g., after_specify -> specify, before_plan -> plan
_command_name=$(echo "$EVENT_NAME" | sed 's/^after_//' | sed 's/^before_//')
_phase=$(echo "$EVENT_NAME" | grep -q '^before_' && echo 'before' || echo 'after')

# Stage all changes first so message generation sees the full diff
_git_out=$(git add . 2>&1) || { echo "[specify] Error: git add failed: $_git_out" >&2; exit 1; }

# For implementation commits, generate a descriptive Conventional Commits
# message from the staged diff instead of the static config string.
_commit_body=""
if [ "$EVENT_NAME" = "after_implement" ]; then
    _generated=$(_generate_commit_message)
    if [ -n "$_generated" ]; then
        _commit_msg=$(printf '%s\n' "$_generated" | sed -n '1p')
        _commit_body=$(printf '%s\n' "$_generated" | sed '1,2d')
    fi
fi

# Use custom message if configured, otherwise default
if [ -z "$_commit_msg" ]; then
    _commit_msg="[Spec Kit] Auto-commit ${_phase} ${_command_name}"
fi

# Commit
if [ -n "$_commit_body" ]; then
    _git_out=$(git commit -q -m "$_commit_msg" -m "$_commit_body" 2>&1) || { echo "[specify] Error: git commit failed: $_git_out" >&2; exit 1; }
else
    _git_out=$(git commit -q -m "$_commit_msg" 2>&1) || { echo "[specify] Error: git commit failed: $_git_out" >&2; exit 1; }
fi

echo "[OK] Changes committed ${_phase} ${_command_name}" >&2
