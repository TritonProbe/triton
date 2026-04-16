# Dashboard API Reference

This document describes the dashboard API exposed by Triton today.

Current-state authority:

- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md)

Base path: `/api/v1`

## Auth and Security

- Methods: API routes are `GET` only
- Auth: optional HTTP Basic Auth when dashboard credentials are configured
- Content type: JSON responses use `application/json; charset=utf-8`
- Trace downloads use `application/qlog+json-seq`
- Security headers:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Referrer-Policy: no-referrer`
  - `Cache-Control: no-store`
  - restrictive `Content-Security-Policy`

## Error Format

Errors use this shape:

```json
{
  "status": "error",
  "error": {
    "code": 404,
    "message": "probe result not found",
    "detail": "see server logs"
  }
}
```

Notes:

- `detail` is omitted when there is no underlying internal error to hide
- unsupported methods return `405`
- unknown API routes return `404`

## List Query Parameters

The list endpoints accept:

- `q`: case-insensitive substring filter
- `sort`: endpoint-specific sort mode
- `limit`: positive integer capped at `200`
- `offset`: zero-based starting position for paged list reads
- `view`: optional response shape selector; `view=summary` omits heavier raw list payload fields where a typed summary already exists

If `limit` is missing or invalid, all matching items are returned.

List endpoints also return pagination headers:

- `X-Total-Count`
- `X-Page-Offset`
- `X-Page-Limit`
- `X-Has-More`
- `X-Next-Offset`

## `GET /status`

Returns dashboard and storage status.

Response shape:

```json
{
  "status": "ok",
  "dashboard": {
    "started_at": "2026-04-14T00:00:00Z",
    "uptime_seconds": 123,
    "trace_enabled": true
  },
  "storage": {
    "probes": 10,
    "benches": 4,
    "traces": 2
  }
}
```

## `GET /config`

Returns the sanitized dashboard-visible config snapshot.

Notes:

- intended for operator inspection, not as a write API
- sensitive dashboard password values are not exposed

## `GET /probes`

Returns recent stored probe summaries.

Filter fields:

- `id`
- `target`
- `proto`

Supported sort values:

- default: newest first by `mod_time`
- `oldest`
- `target_asc`
- `target_desc`
- `status_asc`
- `status_desc`

Response fields:

- `id`
- `target`
- `timestamp`
- `status`
- `proto`
- `duration`
- `mod_time`
- `size`
- `analysis`
- `analysis_view`
- `trace_files`

`analysis_view` is the typed summary projection intended for dashboard rendering. It may include:

- `response`
- `latency`
- `streams`
- `alt_svc`
- `0rtt`
- `migration`
- `qpack`
- `loss`
- `congestion`
- `version`
- `retry`
- `ecn`
- `spin-bit`
- `support`
- `support_summary`
- `fidelity_summary`
- `test_plan`

Important fidelity note:

- `fidelity_summary.full` means directly implemented current-path diagnostics
- `fidelity_summary.observed` means visible client/protocol observation, not packet capture
- `fidelity_summary.partial` means heuristic, estimate-based, or endpoint-capability checks

When `view=summary` is used:

- `analysis_view` remains present
- raw `analysis` is omitted from list items

## `GET /probes/:id`

Returns the full stored `probe.Result` object for one run.

Typical top-level fields:

- `id`
- `target`
- `timestamp`
- `duration`
- `status`
- `proto`
- `trace_files`
- `timings_ms`
- `tls`
- `headers`
- `analysis`

Returns `404` when the probe id does not exist.

## `GET /benches`

Returns recent stored benchmark summaries.

Filter fields:

- `id`
- `target`
- joined `protocols`

Supported sort values:

- default: newest first by `mod_time`
- `oldest`
- `target_asc`
- `target_desc`
- `concurrency_asc`
- `concurrency_desc`

Response fields:

- `id`
- `target`
- `timestamp`
- `duration`
- `concurrency`
- `protocols`
- `summary`
- `stats`
- `stats_view`
- `mod_time`
- `size`
- `trace_files`

`stats_view` is a stable array form of per-protocol benchmark stats for UI consumption.

When `view=summary` is used:

- `stats_view` remains present
- raw `stats` is omitted from list items

## `GET /benches/:id`

Returns the full stored `bench.Result` object for one run.

Returns `404` when the bench id does not exist.

## `GET /traces`

Returns qlog trace metadata for files in the configured trace directory.

Filter fields:

- `name`
- `preview`

Supported sort values:

- default: `name` ascending
- `name_desc`
- `size_asc`
- `size_desc`
- `newest`
- `oldest`

Response fields:

- `name`
- `size_bytes`
- `modified_at`
- `download_url`
- `meta_url`
- `preview`

If tracing is disabled or the trace directory does not exist, this endpoint returns an empty array.

## `GET /traces/meta/:name`

Returns metadata for one `.sqlog` trace file.

Validation rules:

- path is basename-only
- file extension must be `.sqlog`
- directories and traversal are rejected

Returns `404` when the trace does not exist.

## `GET /traces/:name`

Streams the raw `.sqlog` file content.

Validation rules match `GET /traces/meta/:name`.

Returns:

- `200` with `application/qlog+json-seq` on success
- `404` when the trace does not exist or is invalid

## Not Supported

The dashboard API does not currently provide:

- write/update/delete operations
- SSE or WebSocket streams
- pagination cursors
- OpenAPI generation
- token-based auth

## Stability Notes

This API is intended for Triton's embedded operator dashboard and local tooling.

The most stable surfaces today are:

- route existence
- basic list/filter/sort/pagination behavior
- `analysis_view` and `stats_view` as dashboard-friendly summaries
- `support` and `fidelity_summary` semantics for probe interpretation

Target-state documents may describe a richer future API, but this file documents the API that exists in the current repository.
