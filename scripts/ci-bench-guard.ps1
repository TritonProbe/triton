$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if ($env:BIN) {
  $bin = $env:BIN
} else {
  $bin = ".\bin\triton.exe"
}

if (-not (Test-Path $bin)) {
  throw "[bench-guard] binary not found at $bin"
}

$output = & $bin bench --target "triton://loopback/ping" --protocols h3 --duration 1s --concurrency 1 --format json
$parsed = $output | ConvertFrom-Json

if (-not $parsed.stats -or -not $parsed.stats.h3) {
  throw "[bench-guard] missing h3 stats in output"
}

$stats = $parsed.stats.h3
$reqPerSec = [double]$stats.req_per_sec
$errorRate = [double]$stats.error_rate
$sampledPoints = [int]$stats.sampled_points

if ($reqPerSec -le 0.1) {
  throw "[bench-guard] req_per_sec too low: $reqPerSec"
}
if ($errorRate -gt 0.5) {
  throw "[bench-guard] error_rate too high: $errorRate"
}
if ($sampledPoints -lt 1) {
  throw "[bench-guard] sampled_points invalid: $sampledPoints"
}

Write-Host "[bench-guard] ok (req_per_sec=$reqPerSec, error_rate=$errorRate, sampled_points=$sampledPoints)"
