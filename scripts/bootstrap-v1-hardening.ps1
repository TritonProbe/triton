[CmdletBinding()]
param(
  [string]$Milestone = "v1 Hardening",
  [string]$Repo,
  [string]$Assignee,
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Write-Step {
  param([string]$Message)
  Write-Host "[bootstrap-v1-hardening] $Message"
}

function Require-Command {
  param([string]$Name)
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "required command not found: $Name"
  }
}

function Resolve-Repo {
  param([string]$Value)
  if ($Value) {
    return $Value
  }

  $origin = (git remote get-url origin).Trim()
  if ($origin -match "github\.com[:/](?<repo>[^/]+/[^/.]+)(?:\.git)?$") {
    return $Matches["repo"]
  }

  throw "could not determine GitHub repo from origin remote: $origin"
}

function Join-CommandArgs {
  param([string[]]$CommandArgs)

  $quoted = foreach ($arg in $CommandArgs) {
    if ($null -eq $arg) {
      '""'
      continue
    }

    if ($arg -match '[\s"]') {
      '"' + ($arg -replace '"', '\"') + '"'
    } else {
      $arg
    }
  }

  return ($quoted -join " ")
}

function Invoke-GHJson {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$CommandArgs,
    [switch]$IgnoreNotFound
  )

  $stdoutPath = [System.IO.Path]::GetTempFileName()
  $stderrPath = [System.IO.Path]::GetTempFileName()
  try {
    $argumentString = Join-CommandArgs -CommandArgs $CommandArgs
    $process = Start-Process `
      -FilePath "gh" `
      -ArgumentList $argumentString `
      -NoNewWindow `
      -Wait `
      -PassThru `
      -RedirectStandardOutput $stdoutPath `
      -RedirectStandardError $stderrPath

    $exitCode = $process.ExitCode
    $output = if ((Get-Item $stdoutPath).Length -gt 0) {
      Get-Content $stdoutPath -Raw
    } else {
      ""
    }
    $errorOutput = if ((Get-Item $stderrPath).Length -gt 0) {
      Get-Content $stderrPath -Raw
    } else {
      ""
    }
  } finally {
    Remove-Item $stdoutPath, $stderrPath -ErrorAction SilentlyContinue
  }

  if ($exitCode -ne 0) {
    $message = @($output, $errorOutput) -join "`n"
    $message = $message.Trim()
    if ($IgnoreNotFound -and $message -match "404") {
      return $null
    }

    throw "gh command failed: gh $($CommandArgs -join ' ')`n$message"
  }

  if (-not $output) {
    return $null
  }

  return $output | ConvertFrom-Json
}

function Escape-DataString {
  param([string]$Value)
  return [System.Uri]::EscapeDataString($Value)
}

function Ensure-Label {
  param(
    [string]$RepoName,
    [hashtable]$Label
  )

  $encodedName = Escape-DataString -Value $Label.Name
  $existing = Invoke-GHJson -CommandArgs @(
    "api",
    "repos/$RepoName/labels/$encodedName"
  ) -IgnoreNotFound

  if ($existing) {
    Write-Step "label exists: $($Label.Name)"
    return
  }

  if ($DryRun) {
    Write-Step "dry-run create label: $($Label.Name)"
    return
  }

  & gh api "repos/$RepoName/labels" `
    --method POST `
    -f "name=$($Label.Name)" `
    -f "color=$($Label.Color)" `
    -f "description=$($Label.Description)" | Out-Null

  Write-Step "created label: $($Label.Name)"
}

function Ensure-Milestone {
  param(
    [string]$RepoName,
    [string]$Title
  )

  $milestones = Invoke-GHJson -CommandArgs @(
    "api",
    "repos/$RepoName/milestones?state=all&per_page=100"
  )

  foreach ($milestone in ($milestones | ForEach-Object { $_ })) {
    if ($milestone.title -eq $Title) {
      Write-Step "milestone exists: $Title"
      return
    }
  }

  if ($DryRun) {
    Write-Step "dry-run create milestone: $Title"
    return
  }

  & gh api "repos/$RepoName/milestones" `
    --method POST `
    -f "title=$Title" | Out-Null

  Write-Step "created milestone: $Title"
}

function Get-ExistingIssue {
  param(
    [string]$RepoName,
    [string]$Title
  )

  $issues = Invoke-GHJson -CommandArgs @(
    "issue",
    "list",
    "--repo", $RepoName,
    "--state", "all",
    "--search", "$Title in:title",
    "--json", "number,title"
  )

  foreach ($issue in ($issues | ForEach-Object { $_ })) {
    if ($issue.title -eq $Title) {
      return $issue
    }
  }

  return $null
}

function Ensure-Issue {
  param(
    [string]$RepoName,
    [hashtable]$Issue
  )

  $existing = Get-ExistingIssue -RepoName $RepoName -Title $Issue.Title
  if ($existing) {
    Write-Step "issue exists: #$($existing.number) $($Issue.Title)"
    return
  }

  if ($DryRun) {
    Write-Step "dry-run create issue: $($Issue.Title)"
    return
  }

  $args = @(
    "issue", "create",
    "--repo", $RepoName,
    "--title", $Issue.Title,
    "--milestone", $Milestone,
    "--body", $Issue.Body
  )

  foreach ($label in $Issue.Labels) {
    $args += @("--label", $label)
  }

  if ($Assignee) {
    $args += @("--assignee", $Assignee)
  }

  $url = (& gh @args).Trim()
  Write-Step "created issue: $url"
}

Require-Command git
Require-Command gh

$repoName = Resolve-Repo -Value $Repo
Write-Step "repo: $repoName"
Write-Step "milestone: $Milestone"
if ($Assignee) {
  Write-Step "assignee: $Assignee"
}
if ($DryRun) {
  Write-Step "dry-run enabled"
}

$labels = @(
  @{
    Name = "type/cli"
    Color = "0E8A16"
    Description = "CLI-facing changes"
  },
  @{
    Name = "type/product-boundary"
    Color = "BFD4F2"
    Description = "Clarifies supported vs experimental scope"
  },
  @{
    Name = "priority/p1"
    Color = "B60205"
    Description = "Highest priority work"
  },
  @{
    Name = "priority/p2"
    Color = "D93F0B"
    Description = "Important but not first"
  },
  @{
    Name = "area/server"
    Color = "1D76DB"
    Description = "Server and runtime surface"
  },
  @{
    Name = "area/probe"
    Color = "5319E7"
    Description = "Probe pipeline and outputs"
  },
  @{
    Name = "type/ux"
    Color = "FBCA04"
    Description = "User-facing experience changes"
  },
  @{
    Name = "type/docs"
    Color = "0075CA"
    Description = "Documentation changes"
  },
  @{
    Name = "type/dashboard"
    Color = "C5DEF5"
    Description = "Dashboard UI and API surface"
  },
  @{
    Name = "area/dashboard"
    Color = "0052CC"
    Description = "Dashboard area ownership"
  },
  @{
    Name = "type/ops"
    Color = "E99695"
    Description = "Operations and deployment work"
  },
  @{
    Name = "type/release"
    Color = "F9D0C4"
    Description = "Release process and packaging"
  }
)

$issues = @(
  @{
    Title = "Clarify supported vs experimental CLI surface"
    Labels = @("type/cli", "type/product-boundary", "priority/p1", "area/server")
    Body = @"
Clarify `server`, `probe`, `bench`, and `lab` help/output so the supported path is clearly separated from experimental and lab-only surfaces.

Definition of done:
- command help clearly distinguishes supported vs experimental
- `lab` is explicitly described as research/lab-only
- startup warnings make experimental transport obvious
- no CLI text implies in-repo QUIC/H3 is production-ready
"@
  },
  @{
    Title = "Unify probe fidelity language across CLI, JSON, and dashboard"
    Labels = @("area/probe", "type/ux", "type/product-boundary", "priority/p1")
    Body = @"
Make `full`, `observed`, and `partial` consistent across all user-facing probe surfaces.

Definition of done:
- fidelity terms mean the same thing everywhere
- advanced fields always carry fidelity context
- heuristic metrics are harder to misread as packet-level truth
- output-related tests cover the wording
"@
  },
  @{
    Title = "Align current-state docs around SUPPORTED.md"
    Labels = @("type/docs", "type/product-boundary", "priority/p1")
    Body = @"
Make `SUPPORTED.md` the clear source of truth for current supported behavior and align surrounding docs to it.

Definition of done:
- `SUPPORTED.md` is the canonical current-state document
- `README.md` and `ARCHITECTURE.md` defer to it for scope questions
- future-state language is separated from shipped behavior
- current docs do not overstate QUIC/H3 capabilities
"@
  },
  @{
    Title = "Improve dashboard detail views for probe and bench results"
    Labels = @("type/dashboard", "type/ux", "priority/p2", "area/dashboard")
    Body = @"
Improve dashboard detail views so summary, fidelity, and risk are easier to scan in probe and bench results.

Definition of done:
- probe and bench detail views surface summary prominently
- fidelity and risk are visible without digging
- detail navigation is easier to scan
- dashboard tests still pass
"@
  },
  @{
    Title = "Publish operations and release checklist for supported deployment path"
    Labels = @("type/ops", "type/release", "type/docs", "priority/p2")
    Body = @"
Document the supported deployment path with a concise checklist for safe operations and repeatable release handling.

Definition of done:
- checklist covers TLS, auth, remote bind safety, retention, and trace/log storage
- local and CI quality gates are documented together
- release/build expectations are explicit
- a new operator can follow the checklist without tribal knowledge
"@
  }
)

foreach ($label in $labels) {
  Ensure-Label -RepoName $repoName -Label $label
}

Ensure-Milestone -RepoName $repoName -Title $Milestone

foreach ($issue in $issues) {
  Ensure-Issue -RepoName $repoName -Issue $issue
}

Write-Step "done"
