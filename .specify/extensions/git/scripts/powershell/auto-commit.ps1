#!/usr/bin/env pwsh
# Git extension: auto-commit.ps1
# Automatically commit changes after a Spec Kit command completes.
# Checks per-command config keys in git-config.yml before committing.
#
# Usage: auto-commit.ps1 <event_name>
#   e.g.: auto-commit.ps1 after_specify
param(
    [Parameter(Position = 0, Mandatory = $true)]
    [string]$EventName
)
$ErrorActionPreference = 'Stop'

function Find-ProjectRoot {
    param([string]$StartDir)
    $current = Resolve-Path $StartDir
    while ($true) {
        foreach ($marker in @('.specify', '.git')) {
            if (Test-Path (Join-Path $current $marker)) {
                return $current
            }
        }
        $parent = Split-Path $current -Parent
        if ($parent -eq $current) { return $null }
        $current = $parent
    }
}

# Build a Conventional Commits message from the currently staged changes.
# Returns a hashtable with Subject and Body. Heuristic only: it summarizes
# what changed (type, scopes, file counts), not the intent.
function Get-GeneratedCommitMessage {
    $statusLines = @(git diff --cached --name-status 2>$null)
    if (-not $statusLines) { return $null }

    $files = $statusLines | ForEach-Object { ($_ -split '\s+')[-1] }
    $fileCount = @($files).Count

    # Commit type: docs-only, test-only, otherwise feature work
    $nonDoc = $files | Where-Object { $_ -notmatch '(^docs/|\.md$)' }
    $nonTest = $files | Where-Object { $_ -notmatch '(_test\.go$|\.test\.[jt]sx?$|/tests?/)' }
    if (-not $nonDoc) { $ctype = 'docs' }
    elseif (-not $nonTest) { $ctype = 'test' }
    else { $ctype = 'feat' }

    # Map each file to a scope, then rank scopes by file count
    $scopes = $files | ForEach-Object {
        $p = $_
        if ($p -notmatch '/') { 'repo' }
        elseif ($p -match '^internal/([^/]+)/') { $matches[1] }
        elseif ($p -match '^cmd/([^/]+)/') { $matches[1] }
        elseif ($p -match '^frontend/') { 'frontend' }
        elseif ($p -match '^specs?/') { 'spec' }
        elseif ($p -match '^\.specify/') { 'specify' }
        elseif ($p -match '^([^/]+)/') { $matches[1] }
        else { 'repo' }
    }
    $ranking = @($scopes | Group-Object | Sort-Object Count -Descending)
    $scopeLabel = if ($ranking.Count -le 1) { $ranking[0].Name } else { "$($ranking[0].Name),$($ranking[1].Name)" }

    # Added / modified / deleted counts for the subject
    $nAdd = @($statusLines | Where-Object { $_ -match '^A' }).Count
    $nMod = @($statusLines | Where-Object { $_ -match '^M' }).Count
    $nDel = @($statusLines | Where-Object { $_ -match '^D' }).Count
    $parts = @()
    if ($nAdd -gt 0) { $parts += "add $nAdd" }
    if ($nMod -gt 0) { $parts += "update $nMod" }
    if ($nDel -gt 0) { $parts += "remove $nDel" }
    if (-not $parts) { $parts = @("change $fileCount") }
    $noun = if ($fileCount -eq 1) { 'file' } else { 'files' }
    $partStr = ($parts -join ', ')
    if ($scopeLabel -eq $ctype) {
        $subject = "${ctype}: $partStr $noun"
    } else {
        $subject = "${ctype}($scopeLabel): $partStr $noun"
    }

    # Body: per-scope file counts plus git's own shortstat
    $scopeBody = ($ranking | ForEach-Object { "- $($_.Name): $($_.Count) file(s)" }) -join "`n"
    $shortstat = (@(git diff --cached --shortstat 2>$null) -join '').Trim()

    return @{ Subject = $subject; Body = "$scopeBody`n`n$shortstat" }
}

$repoRoot = Find-ProjectRoot -StartDir $PSScriptRoot
if (-not $repoRoot) { $repoRoot = Get-Location }
Set-Location $repoRoot

# Check if git is available
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Warning "[specify] Warning: Git not found; skipped auto-commit"
    exit 0
}

# Temporarily relax ErrorActionPreference so git stderr warnings
# (e.g. CRLF notices on Windows) do not become terminating errors.
$savedEAP = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
try {
    git rev-parse --is-inside-work-tree 2>$null | Out-Null
    $isRepo = $LASTEXITCODE -eq 0
} finally {
    $ErrorActionPreference = $savedEAP
}
if (-not $isRepo) {
    Write-Warning "[specify] Warning: Not a Git repository; skipped auto-commit"
    exit 0
}

# Read per-command config from git-config.yml
$configFile = Join-Path $repoRoot ".specify/extensions/git/git-config.yml"
$enabled = $false
$commitMsg = ""

if (Test-Path $configFile) {
    # Parse YAML to find auto_commit section
    $inAutoCommit = $false
    $inEvent = $false
    $defaultEnabled = $false

    foreach ($line in Get-Content $configFile) {
        # Detect auto_commit: section
        if ($line -match '^auto_commit:') {
            $inAutoCommit = $true
            $inEvent = $false
            continue
        }

        # Exit auto_commit section on next top-level key
        if ($inAutoCommit -and $line -match '^[a-z]') {
            break
        }

        if ($inAutoCommit) {
            # Check default key
            if ($line -match '^\s+default:\s*(.+)$') {
                $val = $matches[1].Trim().ToLower()
                if ($val -eq 'true') { $defaultEnabled = $true }
            }

            # Detect our event subsection
            if ($line -match "^\s+${EventName}:") {
                $inEvent = $true
                continue
            }

            # Inside our event subsection
            if ($inEvent) {
                # Exit on next sibling key (2-space indent, not 4+)
                if ($line -match '^\s{2}[a-z]' -and $line -notmatch '^\s{4}') {
                    $inEvent = $false
                    continue
                }
                if ($line -match '\s+enabled:\s*(.+)$') {
                    $val = $matches[1].Trim().ToLower()
                    if ($val -eq 'true') { $enabled = $true }
                    if ($val -eq 'false') { $enabled = $false }
                }
                if ($line -match '\s+message:\s*(.+)$') {
                    $commitMsg = $matches[1].Trim() -replace '^["'']' -replace '["'']$'
                }
            }
        }
    }

    # If event-specific key not found, use default
    if (-not $enabled -and $defaultEnabled) {
        $hasEventKey = Select-String -Path $configFile -Pattern "^\s*${EventName}:" -Quiet
        if (-not $hasEventKey) {
            $enabled = $true
        }
    }
} else {
    # No config file — auto-commit disabled by default
    exit 0
}

if (-not $enabled) {
    exit 0
}

# Check if there are changes to commit
# Relax ErrorActionPreference so CRLF warnings on stderr do not terminate.
$savedEAP = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
try {
    git diff --quiet HEAD 2>$null; $d1 = $LASTEXITCODE
    git diff --cached --quiet 2>$null; $d2 = $LASTEXITCODE
    $untracked = git ls-files --others --exclude-standard 2>$null
} finally {
    $ErrorActionPreference = $savedEAP
}

if ($d1 -eq 0 -and $d2 -eq 0 -and -not $untracked) {
    Write-Host "[specify] No changes to commit after $EventName" -ForegroundColor DarkGray
    exit 0
}

# Derive a human-readable command name from the event
$commandName = $EventName -replace '^after_', '' -replace '^before_', ''
$phase = if ($EventName -match '^before_') { 'before' } else { 'after' }

# Stage and commit
# Relax ErrorActionPreference so CRLF warnings on stderr do not terminate,
# while still allowing redirected error output to be captured for diagnostics.
$savedEAP = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
try {
    $out = git add . 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) { throw "git add failed: $out" }

    # For implementation commits, generate a descriptive Conventional Commits
    # message from the staged diff instead of the static config string.
    $commitBody = ""
    if ($EventName -eq 'after_implement') {
        $generated = Get-GeneratedCommitMessage
        if ($generated) {
            $commitMsg = $generated.Subject
            $commitBody = $generated.Body
        }
    }

    # Use custom message if configured, otherwise default
    if (-not $commitMsg) {
        $commitMsg = "[Spec Kit] Auto-commit $phase $commandName"
    }

    if ($commitBody) {
        $out = git commit -q -m $commitMsg -m $commitBody 2>&1 | Out-String
    } else {
        $out = git commit -q -m $commitMsg 2>&1 | Out-String
    }
    if ($LASTEXITCODE -ne 0) { throw "git commit failed: $out" }
} catch {
    Write-Warning "[specify] Error: $_"
    exit 1
} finally {
    $ErrorActionPreference = $savedEAP
}

Write-Host "[OK] Changes committed $phase $commandName"
