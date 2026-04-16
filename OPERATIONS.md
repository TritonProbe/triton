# Operations Guide

This guide documents the current operational expectations for running Triton safely.

## Recommended Production-Like Boundary

Prefer the supported path only:

- `server.listen_tcp`
- optional `server.listen_h3`
- optional authenticated dashboard

Avoid treating the following as production-ready:

- `server.listen` experimental UDP H3
- `triton lab`
- `triton://...` transport

## Monitoring

Primary endpoints:

- `/healthz`
- `/readyz`
- `/metrics`
- `/api/v1/status`

Operational checks:

- HTTP server responds on expected TCP port
- real HTTP/3 listener responds on expected UDP port when configured
- dashboard `status` payload shows increasing uptime
- storage counts for probes/benches/traces look sane
- trace enablement matches your configuration

## Logging

Triton emits:

- access logs
- startup profile summary
- explicit warnings when experimental transport is enabled

Recommendations:

- persist access logs when running outside local development
- review startup logs for mixed-plane or experimental warnings
- avoid treating dashboard/API `detail` omissions as silent failures; check server logs

## Storage and Retention

Triton uses filesystem-backed gzip JSON storage for results.

Important knobs:

- `storage.results_dir`
- `storage.max_results`
- `storage.retention`
- `server.trace_dir`
- `probe.trace_dir`
- `bench.trace_dir`

Recommendations:

- keep `storage.max_results` bounded
- set realistic `storage.retention` for your usage
- rotate or prune trace directories separately if traces are large
- expect dashboard list performance to degrade as retained files grow

## Dashboard Exposure

Safer profile:

- keep dashboard on loopback
- enable auth whenever exposing dashboard remotely

Remote dashboard requirements:

- `server.allow_remote_dashboard: true`
- `server.dashboard_user`
- `server.dashboard_pass`

## Tracing

qlog traces are useful but can grow quickly.

Recommendations:

- enable tracing intentionally, not by default everywhere
- dedicate a trace directory with enough disk headroom
- periodically prune old `.sqlog` files
- use dashboard trace list/filtering for inspection, not long-term archival

## Supported vs Lab Change Control

If you enable any of the following, treat the runtime as a lab or mixed-stability deployment:

- `server.listen`
- `server.allow_experimental_h3`
- `server.allow_mixed_h3_planes`

That path is appropriate for experimentation, not for broad production claims.

## Verification Checklist

- config validates cleanly
- HTTPS/TCP listener is reachable
- real HTTP/3 listener is reachable when configured
- dashboard auth works as intended
- `/healthz`, `/readyz`, `/metrics`, and `/api/v1/status` respond
- result retention limits are set intentionally
- trace directories exist and have headroom if tracing is enabled
- no unsupported experimental mode is enabled accidentally

## Related Docs

- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
- [API.md](/d:/Codebox/TritonProbe/API.md)
- [TROUBLESHOOTING.md](/d:/Codebox/TritonProbe/TROUBLESHOOTING.md)
