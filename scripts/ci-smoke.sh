#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN="${BIN:-./bin/triton}"
TCP_ADDR="${TCP_ADDR:-127.0.0.1:18443}"
UDP_ADDR="${UDP_ADDR:-127.0.0.1:14433}"
H3_ADDR="${H3_ADDR:-127.0.0.1:15443}"
DASHBOARD_ADDR="${DASHBOARD_ADDR:-127.0.0.1:19090}"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

"$BIN" version >/dev/null

"$BIN" server \
  --listen "$UDP_ADDR" \
  --allow-experimental-h3 \
  --allow-mixed-h3-planes \
  --listen-h3 "$H3_ADDR" \
  --listen-tcp "$TCP_ADDR" \
  --dashboard=false \
  --dashboard-listen "$DASHBOARD_ADDR" \
  >/tmp/triton-smoke.log 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 40); do
  if curl -skf "https://${TCP_ADDR}/healthz" >/dev/null; then
    break
  fi
  sleep 0.25
done

curl -skf "https://${TCP_ADDR}/healthz" | grep -q '"status":"ok"'
curl -skf "https://${TCP_ADDR}/readyz" | grep -q '"status":"ready"'
curl -skf "https://${TCP_ADDR}/metrics" | grep -q 'triton_requests_total'

"$BIN" probe --target "https://${TCP_ADDR}/ping" --insecure --allow-insecure-tls --format json | grep -q '"status": 200'
"$BIN" probe --target "h3://${H3_ADDR}/ping" --insecure --allow-insecure-tls --format json | grep -q '"alpn": "h3"'
"$BIN" probe --target "triton://${UDP_ADDR}/ping" --format json | grep -q '"proto": "HTTP/3-triton"'
"$BIN" bench --target "triton://loopback/ping" --protocols h3 --duration 1s --concurrency 1 --format json | grep -q '"h3"'
