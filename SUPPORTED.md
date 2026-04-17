# Supported Product Boundary

This document is the canonical description of what Triton supports today.

If another document describes a larger future architecture, backlog, or research goal, prefer this file for current-state truth.

For the lab-only transport boundary in one place, see [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md).

Use this file as the first answer to questions such as:

- what is supported today
- what is lab-only
- what is safe to describe as production-like
- how to interpret advanced probe fidelity

## Supported Runtime

### Supported server path

- `triton server`
- HTTPS/TCP listener via `server.listen_tcp`
- Optional real HTTP/3 listener via `server.listen_h3`
- Optional embedded dashboard

### Supported client path

- `triton probe --target https://...`
- `triton probe --target h3://...`
- `triton bench` against `https://...` targets with H1/H2/H3 coverage

### Lab-only path

- `triton lab`
- `triton://...` targets
- Experimental in-repo transport and HTTP/3 layers under `internal/quic/*` and `internal/h3/*`

These surfaces are documented in more detail in [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md).

The supported HTTP/3 implementation path is `quic-go` via `internal/realh3`.

## Probe Fidelity

Probe output exposes both:

- `support`: whether a requested test is available, unavailable, or not run
- `fidelity_summary`: whether the resulting metric is `full`, `observed`, or `partial`

Use those fields as the contract for interpreting advanced probe output.

### `full`

Directly implemented current-path diagnostics:

- `handshake`
- `tls`
- `latency`
- `throughput`
- `streams`
- `alt-svc`

### `observed`

Derived from visible protocol/client-layer state, not packet capture:

- `version`
- `retry`
- `ecn`

### `partial`

Heuristic, estimate-based, or endpoint-capability checks:

- `0rtt`
- `migration`
- `qpack`
- `loss`
- `congestion`
- `spin-bit`

## Supported Dashboard Surface

The embedded dashboard is a lightweight operator surface, not the full live protocol workbench from the target-state docs.

Supported today:

- status view
- config snapshot
- recent probe list/detail
- recent bench list/detail
- trace list/detail
- in-page filtering, sorting, offset-based pagination, and compare/overview cards

Not supported today:

- SSE live streaming
- WebSocket control plane
- packet timeline workbench
- browser-heavy SPA architecture

## Supported Server Endpoints

Current endpoint set:

- `GET /`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /ping`
- `GET,POST /echo`
- `GET /download/:size`
- `POST /upload`
- `GET /delay/:ms`
- `GET /redirect/:n`
- `GET /streams/:n`
- `GET /headers/:n`
- `GET /status/:code`
- `GET /drip/:size/:delay`
- `GET /tls-info`
- `GET /quic-info`
- `GET /migration-test`
- `GET /.well-known/triton`

## Supported Dashboard API

- `GET /api/v1/status`
- `GET /api/v1/config`
- `GET /api/v1/probes`
- `GET /api/v1/benches`
- `GET /api/v1/probes/:id`
- `GET /api/v1/benches/:id`
- `GET /api/v1/traces`
- `GET /api/v1/traces/meta/:name`
- `GET /api/v1/traces/:name`

## Safety Boundary

The current product boundary depends on explicit safety gates:

- experimental UDP H3 requires explicit enablement
- non-loopback experimental bind requires additional explicit enablement
- mixing real and experimental H3 listeners requires explicit opt-in
- remote dashboard bind requires explicit opt-in and auth
- insecure probe/bench TLS requires explicit opt-in

## Strategic Boundary

Future-looking documents such as `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, and `.project/TASKS.md` describe target-state or research goals unless they explicitly say otherwise.

For current deployment and support decisions, treat this file plus [README.md](/d:/Codebox/TritonProbe/README.md) and [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md) as authoritative.
