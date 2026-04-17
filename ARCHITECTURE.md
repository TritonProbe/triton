# Triton Architecture

This document describes Triton's current architecture boundary as implemented in the repository.

For current support and deployment decisions, prefer [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md). This file is the architecture-oriented explanation of that supported boundary.

## Product Boundary

Triton currently runs three transport planes in one binary:

1. Real HTTP server plane (supported)
2. Real HTTP/3 plane via `quic-go` (supported)
3. Experimental in-repo UDP H3 plane (lab-only)

The supported production-like path is the real HTTP server + real HTTP/3 via `quic-go`.
The in-repo `internal/quic` and `internal/h3` stacks are intentionally experimental.

For the explicit lab-only boundary and usage guidance, see [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md).
Product positioning and conditional custom-engine milestones are tracked in [.project/ENGINE_STRATEGY.md](./.project/ENGINE_STRATEGY.md).

If this file and [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md) ever appear to disagree, treat `SUPPORTED.md` as authoritative for current-state truth.

## Probe Fidelity Boundary

The probe surface now exposes both `support` and `fidelity_summary` metadata so callers can distinguish implemented transport truth from approximations.

- `full`: directly implemented and supported in the current product path
- `observed`: derived from visible protocol/client-layer state, not packet capture
- `partial`: heuristic, capability-check, or estimate-based

Today that means:

- `version`, `retry`, and `ecn` are surfaced as `observed`
- `0rtt`, `migration`, `qpack`, `loss`, `congestion`, and `spin-bit` are surfaced as `partial`
- only the baseline probe dimensions such as handshake, TLS metadata, throughput, latency, streams, and Alt-Svc should be read as fully implemented current-path diagnostics

CLI output, dashboard cards, and JSON results are expected to preserve that distinction.

## Runtime Profiles

### `triton server`

- Starts supported listeners (`listen_tcp`, optional `listen_h3`) plus optional dashboard.
- Experimental `listen` is disabled unless explicitly allowed.
- Remote dashboard and remote experimental listener binds require explicit opt-in.
- Running real and experimental H3 listeners together requires explicit mixed-plane opt-in.

### `triton lab`

- Runs only the experimental UDP H3 listener.
- Disables supported HTTPS/dashboard surfaces.
- Intended for protocol experimentation, not production service hosting.

## Main Components

- `cmd/triton`: CLI entrypoint
- `internal/cli`: command parsing, mode wiring
- `internal/config`: defaults, YAML/env/flag layering, validation
- `internal/server`: listener setup, lifecycle, endpoint exposure
- `internal/appmux`: shared endpoint handlers (`/ping`, `/echo`, `/drip`, etc.)
- `internal/probe`: target analysis and structured result generation
- `internal/bench`: protocol benchmarking and summary rollups
- `internal/dashboard`: embedded static UI + JSON API over stored runs
- `internal/storage`: compressed result persistence and retention cleanup
- `internal/observability`: access logging, request IDs, qlog support
- `internal/realh3`: real HTTP/3 client/server wiring through `quic-go`
- `internal/quic`, `internal/h3`: experimental in-repo transport/frame layers

## Data Flow

1. CLI loads merged config (`default` + `yaml` + `env` + `flags`).
2. Config validation enforces listener safety rules.
3. Server/probe/bench runs and emits structured results.
4. Results are stored in filesystem gzip JSON files.
5. Dashboard API reads stored results and returns summary views.
6. Embedded dashboard assets render operator cards from API responses.

## Safety Design Highlights

- Experimental listener requires explicit enablement.
- Non-loopback experimental listener requires additional explicit enablement.
- Mixed real/experimental H3 listeners require explicit intent.
- Remote dashboard bind requires explicit enablement and auth.
- Insecure probe/bench TLS requires explicit opt-in flags/config.

## Testing and Quality Gates

- Unit and integration tests across CLI/config/server/probe/bench/storage/dashboard.
- Parser fuzz targets for QUIC/H3 frame and packet surfaces.
- Security scanner (`gosec`) integrated and expected clean.
- CI includes CGO-capable `go test -race ./...` job.

## Known Limits

- Several advanced QUIC/H3 analyses are still approximation-based.
- Experimental in-repo transport remains lab-grade.
- Dashboard is a lightweight operator surface, not a full live protocol workbench.
- Packet-level transport claims should be reserved for future work unless the implementation explicitly graduates from `observed`/`partial` fidelity.
