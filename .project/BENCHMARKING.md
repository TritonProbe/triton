# Benchmarking Methodology (2026-04-14)

This document defines how Triton benchmark numbers should be interpreted today.

## Scope

Triton bench currently supports:

- `h1` and `h2` via Go `net/http` over TLS/TCP
- `h3` via real HTTP/3 (`quic-go`) for normal HTTPS targets
- `h3` via experimental in-repo transport for `triton://...` targets

These are not all equivalent transport stacks. Results must be compared with context.

## Comparison Rules

1. Compare `h1`, `h2`, and real `h3` only against the same HTTPS target and environment.
2. Treat `triton://` benchmark numbers as lab diagnostics, not internet-facing protocol performance.
3. Interpret `req_per_sec`, `error_rate`, and latency percentiles together; never use a single metric alone.
4. Use at least one warmup run and at least one measured run before drawing conclusions.
5. Keep concurrency and duration fixed across protocols when making cross-protocol claims.

## Known Non-Equivalences

- `h1`/`h2` path uses standard HTTP transport over TCP/TLS.
- Real `h3` path uses QUIC/TLS with `quic-go`.
- `triton://` path uses experimental in-repo UDP transport and is not RFC-complete QUIC/H3.

Because of this, direct “winner” claims involving `triton://` should be framed as implementation diagnostics, not protocol-level truth.

## Lightweight Regression Guard

Repository now includes a lightweight benchmark guard script:

- `scripts/ci-bench-guard.sh`

It validates that a loopback `h3` benchmark still produces non-zero throughput and bounded error rate.
This is a sanity signal, not a full performance certification.

## Next Step (future)

- Store historical benchmark snapshots and compare deltas over time per protocol/target profile.
