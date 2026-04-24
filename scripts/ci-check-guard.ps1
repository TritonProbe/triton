$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if ($env:BIN) {
  $bin = $env:BIN
  $buildLocalBinary = $false
} else {
  $bin = ".\bin\triton.exe"
  $buildLocalBinary = $true
}

$tcpAddr = if ($env:TCP_ADDR) { $env:TCP_ADDR } else { "127.0.0.1:18443" }
$dashboardAddr = if ($env:DASHBOARD_ADDR) { $env:DASHBOARD_ADDR } else { "127.0.0.1:19090" }

if ($buildLocalBinary) {
  $binDir = Split-Path -Parent $bin
  if ($binDir -and -not (Test-Path $binDir)) {
    $null = New-Item -ItemType Directory -Path $binDir -Force
  }
  & go build -o $bin ./cmd/triton
  if (-not (Test-Path $bin)) {
    throw "[check-guard] binary not found at $bin after build"
  }
} elseif (-not (Test-Path $bin)) {
  throw "[check-guard] binary not found at $bin"
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("triton-check-" + [guid]::NewGuid().ToString("N"))
$null = New-Item -ItemType Directory -Path $tempDir -Force
$configPath = Join-Path $tempDir "triton-check.yaml"
$reportPath = Join-Path $tempDir "check-report.md"
$stdoutLog = Join-Path $tempDir "server.out.log"
$stderrLog = Join-Path $tempDir "server.err.log"

$config = @"
server:
  listen: ""
  listen_h3: ""
  listen_tcp: ":8443"
  dashboard: false
probe:
  timeout: 1s
  insecure: true
  allow_insecure_tls: true
  default_tests:
    - handshake
    - latency
  default_streams: 1
bench:
  default_duration: 200ms
  default_concurrency: 1
  default_protocols:
    - h1
  insecure: true
  allow_insecure_tls: true
probe_profiles:
  ci-local:
    target: https://$tcpAddr/ping
    report_name: CI Local Probe
    default_format: markdown
    thresholds:
      require_status_min: 200
      require_status_max: 299
      max_total_ms: 1000
bench_profiles:
  ci-local:
    target: https://$tcpAddr/ping
    report_name: CI Local Bench
    default_format: markdown
    default_protocols:
      - h1
    thresholds:
      require_all_healthy: true
      max_error_rate: 0.05
      min_req_per_sec: 1
      max_p95_ms: 1000
storage:
  results_dir: $($tempDir -replace '\\','/')
  max_results: 20
  retention: 24h
"@
Set-Content -Path $configPath -Value $config -NoNewline

$server = $null
try {
  & $bin version | Out-Null

  $server = Start-Process -FilePath $bin -ArgumentList @(
    "server",
    "--listen-tcp", $tcpAddr,
    "--dashboard=false",
    "--dashboard-listen", $dashboardAddr
  ) -PassThru -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog

  $healthUri = "https://$tcpAddr/healthz"
  $ready = $false
  for ($i = 0; $i -lt 40; $i++) {
    try {
      $health = & curl.exe -skf $healthUri | ConvertFrom-Json
      if ($health.status -eq "ok") {
        $ready = $true
        break
      }
    } catch {
      Start-Sleep -Milliseconds 250
    }
  }

  if (-not $ready) {
    throw "[check-guard] server did not become healthy in time"
  }

  $result = & $bin check --config $configPath --profile ci-local --report-out $reportPath --report-format markdown --format json | ConvertFrom-Json
  if (-not $result.passed) {
    throw "[check-guard] expected combined check to pass"
  }
  if (-not (Test-Path $reportPath)) {
    throw "[check-guard] report file was not created"
  }
  $reportText = Get-Content $reportPath -Raw
  if ($reportText -notmatch "Check Result") {
    throw "[check-guard] report output missing heading"
  }

  Write-Host "[check-guard] ok"
} finally {
  if ($server -and -not $server.HasExited) {
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
  }
  if (Test-Path $tempDir) {
    Remove-Item -LiteralPath $tempDir -Recurse -Force
  }
}
