# Troubleshooting Guide

This guide covers the most common Triton issues on the current supported product path.

Current-state authority:

- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
- [API.md](/d:/Codebox/TritonProbe/API.md)

## Server startup fails

### `at least one server listener must be configured`

Cause:

- all of `server.listen_tcp`, `server.listen_h3`, and `server.listen` are empty

Fix:

- set at least one listener
- for normal supported usage, prefer `server.listen_tcp` and optionally `server.listen_h3`

## Experimental UDP H3 will not start

### `server.listen is experimental and requires server.allow_experimental_h3 to be true`

Cause:

- experimental UDP H3 listener was configured without explicit opt-in

Fix:

- set `server.allow_experimental_h3: true`
- for isolated transport work, prefer `triton lab`

### `server.listen must stay on loopback unless server.allow_remote_experimental_h3 is true`

Cause:

- experimental UDP H3 was bound to a non-loopback interface without the extra safety flag

Fix:

- bind it to `127.0.0.1:port`, or
- set `server.allow_remote_experimental_h3: true` intentionally

### `enabling both server.listen and server.listen_h3 requires server.allow_mixed_h3_planes to be true`

Cause:

- real HTTP/3 and experimental UDP H3 were enabled together without explicit approval

Fix:

- set `server.allow_mixed_h3_planes: true`, or
- run only one H3 plane

## Dashboard is not reachable

### Dashboard only listens on loopback

Cause:

- default dashboard bind is `127.0.0.1:9090`

Fix:

- access it locally, or
- set `server.dashboard_listen` to a non-loopback address
- if exposing remotely, also set:
  - `server.allow_remote_dashboard: true`
  - `server.dashboard_user`
  - `server.dashboard_pass`

### `server dashboard auth is required when server.allow_remote_dashboard is true`

Cause:

- remote dashboard exposure was enabled without credentials

Fix:

- set both `server.dashboard_user` and `server.dashboard_pass`

### Dashboard returns `401 unauthorized`

Cause:

- dashboard Basic Auth is enabled and the request did not include valid credentials

Fix:

- use the configured username/password
- check `/api/...` callers too, not just the browser

## Probe or bench fails with TLS verification errors

Symptoms:

- certificate validation fails against local/self-signed targets

Fix:

- for local lab usage only, set:
  - `probe.insecure: true` and `probe.allow_insecure_tls: true`
  - or `bench.insecure: true` and `bench.allow_insecure_tls: true`

Important:

- insecure TLS is intentionally blocked unless the matching `allow_insecure_tls` flag is also set

## Real HTTP/3 is not available

Check:

- `server.listen_h3` is configured
- UDP port is reachable
- the server startup log includes `http/3 listener on udp://...`

Notes:

- the supported H3 path is `quic-go`
- `triton://...` is not the supported real HTTP/3 path

## Probe output looks richer than the underlying fidelity

Cause:

- advanced probe fields can be `observed` or `partial` rather than packet-level telemetry

Fix:

- inspect `analysis.support`
- inspect `analysis.fidelity_summary`
- treat:
  - `full` as directly implemented current-path diagnostics
  - `observed` as visible protocol/client-layer observation
  - `partial` as heuristic, estimate-based, or capability-check output

## Trace endpoints are empty

Cause:

- `trace_dir` is not configured
- no traces have been written yet
- the trace directory does not exist

Fix:

- set `server.trace_dir`, `probe.trace_dir`, or `bench.trace_dir` as needed
- generate at least one real HTTP/3 run that emits traces
- check `/api/v1/status` for `trace_enabled`

Notes:

- the dashboard returns an empty trace list when tracing is disabled or the directory is absent

## Trace lookup returns `404`

Cause:

- requested file does not exist
- file extension is not `.sqlog`
- invalid/traversal-style name was used

Fix:

- list available traces first via `/api/v1/traces`
- use the returned `download_url` or `meta_url`

## Result lists feel slow or too large

Cause:

- dashboard lists are filesystem-backed
- list filtering/sorting is done on in-memory summaries
- large retained result sets or large trace directories increase work

Fix:

- lower `storage.max_results`
- shorten `storage.retention`
- reduce trace retention in the configured trace directory
- use list endpoint `q`, `sort`, and `limit` parameters

## `go test -race ./...` does not run locally

Cause:

- local environment lacks CGO support

Fix:

- use a CGO-capable Go toolchain locally, or
- rely on the dedicated CI race job for authoritative race coverage

## Which docs should I trust?

For current-state truth, prefer:

1. [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
2. [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
3. [API.md](/d:/Codebox/TritonProbe/API.md)
4. [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md)

Treat `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, and `.project/TASKS.md` as target-state or research-oriented unless they explicitly say otherwise.
