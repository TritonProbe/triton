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

## Safe Deployment Checklist

Use this short list before exposing a server beyond local development:

1. Pick the supported listeners you actually need.
   Prefer `server.listen_tcp` and optionally `server.listen_h3`.
2. Decide how TLS certificates will be handled.
   Use custom `server.cert` and `server.key` for shared or remote environments.
3. Keep the dashboard on loopback unless remote access is intentional.
   If remote access is needed, require `server.allow_remote_dashboard: true` plus auth.
4. Set storage and trace directories intentionally.
   Confirm retention, max-results, and disk headroom before enabling long-running collection.
5. Verify health and status endpoints after startup.
   Check `/healthz`, `/readyz`, `/metrics`, and `/api/v1/status`.
6. Confirm no lab-only transport flags were enabled by accident.
   Review startup logs for experimental or mixed-plane warnings.

## TLS Certificate Choice

Choose one of these profiles explicitly:

- Local-only evaluation:
  rely on Triton's existing runtime certificate behavior if the service stays local or disposable.
- Shared or remote deployment:
  provide `server.cert` and `server.key` so clients see a stable, intentional certificate chain.

Recommended rule:

- if other people, automation, or remote probes hit the service, treat custom certificates as the default choice

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

Recommended remote posture:

- bind the dashboard only to the interface you need
- avoid default credentials or shared throwaway passwords
- pair dashboard exposure with network-layer restrictions when possible

## Tracing

qlog traces are useful but can grow quickly.

Recommendations:

- enable tracing intentionally, not by default everywhere
- dedicate a trace directory with enough disk headroom
- periodically prune old `.sqlog` files
- use dashboard trace list/filtering for inspection, not long-term archival

## Container Runtime Assumptions

The current container image is designed around these assumptions:

- it runs as non-root user `10001`
- default working directory is `/var/lib/triton`
- default command is `triton server`
- default writable runtime state lives under `/var/lib/triton/triton-data`
- the image includes CA certificates for outbound `probe` and `bench` TLS verification

Exposed ports:

- `8443/tcp` for HTTPS/TCP
- `4434/udp` for supported real HTTP/3 when enabled
- `4433/udp` for experimental lab UDP H3 when enabled
- `9090/tcp` for the dashboard

Container recommendations:

- mount `/var/lib/triton` or at least `/var/lib/triton/triton-data` if you want persistent results and generated self-signed cert material
- provide explicit `server.cert` and `server.key` for shared or remote deployments
- do not expose `4433/udp` unless you intentionally want the lab-only transport surface
- keep dashboard exposure loopback-only or pair remote bind with auth and network controls

## Supported vs Lab Change Control

If you enable any of the following, treat the runtime as a lab or mixed-stability deployment:

- `server.listen`
- `server.allow_experimental_h3`
- `server.allow_mixed_h3_planes`

That path is appropriate for experimentation, not for broad production claims.

## Local And CI Quality Gates

Local release-prep commands:

- `go test ./... -count=1`
- `go vet ./...`
- `staticcheck ./...`
- `gosec ./...`
- `bash ./scripts/ci-smoke.sh` on bash-capable environments
- `pwsh -File ./scripts/ci-smoke.ps1` on Windows/PowerShell
- `bash ./scripts/ci-bench-guard.sh`

Optional but recommended when your local toolchain supports CGO:

- `CGO_ENABLED=1 go test -race ./... -count=1`

CI remains the authoritative gate for:

- `go test ./...`
- `go vet ./...`
- `staticcheck ./...`
- `gosec ./...`
- smoke verification
- benchmark regression guard
- the dedicated Linux race run

## Release Checklist

Use this flow for repeatable publishing:

1. Run the local quality gates that apply to your environment.
2. Confirm the worktree is clean and `main` contains the intended release changes.
3. Create and push a `v*` tag such as `v1.0.0`.
4. Let `.github/workflows/release.yml` invoke GoReleaser from `.goreleaser.yml`.
5. Verify the GitHub release contains cross-platform archives and `checksums.txt`.
6. Smoke-check the released binary on at least one target platform.

Current release automation expects:

- tag-driven releases only
- a full-history checkout on the release runner
- passing Go tests plus release smoke verification before packaging

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
