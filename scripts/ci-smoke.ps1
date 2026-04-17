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
$udpAddr = if ($env:UDP_ADDR) { $env:UDP_ADDR } else { "127.0.0.1:14433" }
$h3Addr = if ($env:H3_ADDR) { $env:H3_ADDR } else { "127.0.0.1:15443" }
$dashboardAddr = if ($env:DASHBOARD_ADDR) { $env:DASHBOARD_ADDR } else { "127.0.0.1:19090" }

if ($buildLocalBinary) {
  $binDir = Split-Path -Parent $bin
  if ($binDir -and -not (Test-Path $binDir)) {
    $null = New-Item -ItemType Directory -Path $binDir -Force
  }
  & go build -o $bin ./cmd/triton
  if (-not (Test-Path $bin)) {
    throw "[smoke] binary not found at $bin after build"
  }
} elseif (-not (Test-Path $bin)) {
  throw "[smoke] binary not found at $bin"
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("triton-smoke-" + [guid]::NewGuid().ToString("N"))
$null = New-Item -ItemType Directory -Path $tempDir -Force
$stdoutLog = Join-Path $tempDir "server.out.log"
$stderrLog = Join-Path $tempDir "server.err.log"

$server = $null
try {
  & $bin version | Out-Null

  $server = Start-Process -FilePath $bin -ArgumentList @(
    "server",
    "--listen", $udpAddr,
    "--allow-experimental-h3",
    "--allow-mixed-h3-planes",
    "--listen-h3", $h3Addr,
    "--listen-tcp", $tcpAddr,
    "--dashboard=false",
    "--dashboard-listen", $dashboardAddr
  ) -PassThru -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog

  $healthUri = "https://$tcpAddr/healthz"
  $readyUri = "https://$tcpAddr/readyz"
  $metricsUri = "https://$tcpAddr/metrics"

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
    throw "[smoke] server did not become healthy in time"
  }

  $health = & curl.exe -skf $healthUri | ConvertFrom-Json
  if ($health.status -ne "ok") {
    throw "[smoke] unexpected health payload: $($health | ConvertTo-Json -Compress)"
  }

  $readyPayload = & curl.exe -skf $readyUri | ConvertFrom-Json
  if ($readyPayload.status -ne "ready") {
    throw "[smoke] unexpected ready payload: $($readyPayload | ConvertTo-Json -Compress)"
  }

  $metrics = (& curl.exe -skf $metricsUri) -join "`n"
  if ($metrics -notmatch "triton_requests_total") {
    throw "[smoke] metrics endpoint missing triton_requests_total"
  }

  $tcpProbe = & $bin probe --target "https://$tcpAddr/ping" --insecure --allow-insecure-tls --format json | ConvertFrom-Json
  if ([int]$tcpProbe.status -ne 200) {
    throw "[smoke] https probe did not return status 200"
  }

  $h3Probe = & $bin probe --target "h3://$h3Addr/ping" --insecure --allow-insecure-tls --format json | ConvertFrom-Json
  if ([string]$h3Probe.tls.alpn -ne "h3") {
    throw "[smoke] h3 probe did not negotiate h3"
  }

  $labProbe = & $bin probe --target "triton://$udpAddr/ping" --format json | ConvertFrom-Json
  if ([string]$labProbe.proto -ne "HTTP/3-triton") {
    throw "[smoke] experimental probe did not report HTTP/3-triton"
  }

  $bench = & $bin bench --target "triton://loopback/ping" --protocols h3 --duration 1s --concurrency 1 --format json | ConvertFrom-Json
  if (-not $bench.stats -or -not $bench.stats.h3) {
    throw "[smoke] bench output missing h3 stats"
  }

  Write-Host "[smoke] ok"
} finally {
  if ($server -and -not $server.HasExited) {
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
  }
  if (Test-Path $tempDir) {
    Remove-Item -LiteralPath $tempDir -Recurse -Force
  }
}
