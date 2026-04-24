#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN="${BIN:-./bin/triton}"
TCP_ADDR="${TCP_ADDR:-127.0.0.1:18443}"
DASHBOARD_ADDR="${DASHBOARD_ADDR:-127.0.0.1:19090}"

TEMP_DIR="$(mktemp -d)"
CONFIG_PATH="${TEMP_DIR}/triton-check.yaml"
REPORT_PATH="${TEMP_DIR}/check-report.md"
SERVER_LOG="${TEMP_DIR}/server.log"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

cat >"$CONFIG_PATH" <<EOF
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
    target: https://${TCP_ADDR}/ping
    report_name: CI Local Probe
    default_format: markdown
    thresholds:
      require_status_min: 200
      require_status_max: 299
      max_total_ms: 1000
bench_profiles:
  ci-local:
    target: https://${TCP_ADDR}/ping
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
  results_dir: ${TEMP_DIR}/triton-data
  max_results: 20
  retention: 24h
EOF

"$BIN" version >/dev/null

"$BIN" server \
  --listen-tcp "$TCP_ADDR" \
  --dashboard=false \
  --dashboard-listen "$DASHBOARD_ADDR" \
  >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 40); do
  if curl -skf "https://${TCP_ADDR}/healthz" >/dev/null; then
    break
  fi
  sleep 0.25
done

curl -skf "https://${TCP_ADDR}/healthz" | grep -q '"status":"ok"'

"$BIN" check \
  --config "$CONFIG_PATH" \
  --profile ci-local \
  --report-out "$REPORT_PATH" \
  --report-format markdown \
  --format json \
  | grep -q '"passed": true'

grep -q 'Check Result' "$REPORT_PATH"
echo "[check-guard] ok"
