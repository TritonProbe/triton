#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN="${BIN:-./bin/triton}"

if [[ ! -x "$BIN" ]]; then
  echo "[bench-guard] binary not found at $BIN"
  exit 1
fi

output="$("$BIN" bench --target "triton://loopback/ping" --protocols h3 --duration 1s --concurrency 1 --format json)"

flattened="$(echo "$output" | tr -d '\r\n')"

req_per_sec="$(echo "$flattened" | sed -n 's/.*"req_per_sec":[[:space:]]*\([0-9.]*\).*/\1/p' | head -n1)"
error_rate="$(echo "$flattened" | sed -n 's/.*"error_rate":[[:space:]]*\([0-9.]*\).*/\1/p' | head -n1)"
sampled_points="$(echo "$flattened" | sed -n 's/.*"sampled_points":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1)"

if [[ -z "${req_per_sec}" || -z "${error_rate}" || -z "${sampled_points}" ]]; then
  echo "[bench-guard] failed to parse benchmark output"
  echo "$output"
  exit 1
fi

awk "BEGIN { exit !($req_per_sec > 0.1) }" || { echo "[bench-guard] req_per_sec too low: $req_per_sec"; exit 1; }
awk "BEGIN { exit !($error_rate <= 0.5) }" || { echo "[bench-guard] error_rate too high: $error_rate"; exit 1; }
awk "BEGIN { exit !($sampled_points >= 1) }" || { echo "[bench-guard] sampled_points invalid: $sampled_points"; exit 1; }

echo "[bench-guard] ok (req_per_sec=$req_per_sec, error_rate=$error_rate, sampled_points=$sampled_points)"
