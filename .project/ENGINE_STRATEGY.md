# Engine Strategy Decision (2026-04-14)

## Decision

Triton's product track is now explicitly:

- Supported diagnostics server and tooling (`server`, `probe`, `bench`)
- Real HTTP/3 behavior via `quic-go` for production-like usage
- Embedded dashboard as an operator surface over stored runs

The in-repo custom QUIC/H3 engine (`internal/quic`, `internal/h3`) is explicitly a **lab/research track**.

## Product vs Lab Boundary

- Product-safe runtime:
- `triton server` with `listen_tcp` and optional `listen_h3`
- `h3://` probe and bench paths
- Dashboard APIs and UI summaries

- Lab runtime:
- `triton lab`
- `triton://` targets
- Experimental UDP H3 listener (`server.listen`) with explicit opt-ins

## Why this decision

- The real H3 path is stable enough to deliver user value now.
- The custom engine remains incomplete for production claims (QUIC-TLS, packet protection, QPACK, migration, congestion/loss behavior).
- Keeping both tracks explicit prevents over-promising and reduces operator risk.

## Conditional vNext Milestone (only if custom engine is re-scoped to product)

1. QUIC-TLS key schedule + packet protection with interop tests.
2. Real QPACK encoder/decoder with dynamic table correctness and fuzz coverage.
3. Connection migration + 0-RTT end-to-end with replay safety and state tests.
4. Loss detection + congestion control baseline (NewReno at minimum) with CI race/fuzz gates.
5. Protocol-level telemetry parity for retry/version/ecn/spin/loss/congestion claims.

Until those milestones are delivered, custom engine claims remain lab-only.
