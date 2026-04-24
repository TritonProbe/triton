param(
  [switch]$Race,
  [switch]$SkipSmoke,
  [switch]$SkipBenchGuard,
  [switch]$SkipCheckGuard
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Require-Command {
  param(
    [string]$Name,
    [string]$InstallHint
  )

  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "[local-ci] required command '$Name' was not found. $InstallHint"
  }
}

function Invoke-Step {
  param(
    [string]$Name,
    [scriptblock]$Action
  )

  Write-Host "[local-ci] $Name"
  & $Action
  Write-Host "[local-ci] ok: $Name"
}

Require-Command "go" "Install Go and ensure it is available in PATH."
Require-Command "staticcheck" "Install with: go install honnef.co/go/tools/cmd/staticcheck@latest"
Require-Command "gosec" "Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"
Require-Command "rg" "Install ripgrep and ensure 'rg' is available in PATH."

$goFiles = @(& rg --files -g '*.go' -g '!vendor/**')
if (-not $goFiles -or $goFiles.Count -eq 0) {
  throw "[local-ci] no Go files found for formatting verification"
}

Invoke-Step "verify formatting" {
  $unformatted = @(& gofmt -l $goFiles | Where-Object { $_ -and $_.Trim() -ne "" })
  if ($unformatted.Count -gt 0) {
    throw "[local-ci] gofmt required for:`n$($unformatted -join "`n")"
  }
}

Invoke-Step "run tests" {
  & go test ./... -count=1
  if ($LASTEXITCODE -ne 0) {
    throw "[local-ci] go test failed"
  }
}

Invoke-Step "run vet" {
  & go vet ./...
  if ($LASTEXITCODE -ne 0) {
    throw "[local-ci] go vet failed"
  }
}

Invoke-Step "run staticcheck" {
  & staticcheck ./...
  if ($LASTEXITCODE -ne 0) {
    throw "[local-ci] staticcheck failed"
  }
}

Invoke-Step "run gosec" {
  & gosec ./...
  if ($LASTEXITCODE -ne 0) {
    throw "[local-ci] gosec failed"
  }
}

$bin = ".\bin\triton.exe"
Invoke-Step "build binary" {
  $binDir = Split-Path -Parent $bin
  if ($binDir -and -not (Test-Path $binDir)) {
    $null = New-Item -ItemType Directory -Path $binDir -Force
  }
  & go build -o $bin ./cmd/triton
  if ($LASTEXITCODE -ne 0 -or -not (Test-Path $bin)) {
    throw "[local-ci] build failed"
  }
}

$env:BIN = $bin
try {
  if (-not $SkipSmoke) {
    Invoke-Step "run smoke flow" {
      & powershell -NoProfile -File .\scripts\ci-smoke.ps1
      if ($LASTEXITCODE -ne 0) {
        throw "[local-ci] smoke flow failed"
      }
    }
  } else {
    Write-Host "[local-ci] skip: smoke flow"
  }

  if (-not $SkipBenchGuard) {
    Invoke-Step "run benchmark regression guard" {
      & powershell -NoProfile -File .\scripts\ci-bench-guard.ps1
      if ($LASTEXITCODE -ne 0) {
        throw "[local-ci] benchmark regression guard failed"
      }
    }
  } else {
    Write-Host "[local-ci] skip: benchmark regression guard"
  }

  if (-not $SkipCheckGuard) {
    Invoke-Step "run combined check guard" {
      & powershell -NoProfile -File .\scripts\ci-check-guard.ps1
      if ($LASTEXITCODE -ne 0) {
        throw "[local-ci] combined check guard failed"
      }
    }
  } else {
    Write-Host "[local-ci] skip: combined check guard"
  }

  if ($Race) {
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
      Write-Host "[local-ci] skip: race tests require gcc in PATH on this machine"
    } else {
      Invoke-Step "run race tests" {
        $previousCGO = $env:CGO_ENABLED
        try {
          $env:CGO_ENABLED = "1"
          & go test -race ./... -count=1
          if ($LASTEXITCODE -ne 0) {
            throw "[local-ci] race tests failed"
          }
        } finally {
          $env:CGO_ENABLED = $previousCGO
        }
      }
    }
  } else {
    Write-Host "[local-ci] skip: race tests (pass -Race to enable)"
  }
} finally {
  if (Test-Path Env:\BIN) {
    Remove-Item Env:\BIN
  }
}

Write-Host "[local-ci] all requested checks passed"
