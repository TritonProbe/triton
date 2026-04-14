# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-11
> This roadmap prioritizes the work needed to turn Triton into a clear, production-grade product instead of a mixed experimental platform.

## Current State Assessment

Triton already works as a local HTTP diagnostics tool. The CLI is usable, storage is solid, CI/release automation exists, the dashboard works as a read-only status surface, H1/H2 benchmarking works, `h3://` probing and benchmarking work through `quic-go`, and `triton://` works through the experimental in-repo transport.

The core blocker is not "nothing works." The blocker is that the product story is split in two:

- a real, dependency-backed HTTP/3 path that is useful today
- an experimental custom QUIC/H3 path that is far from the spec

The next roadmap should decide which one is the product and which one is the lab.

## Completed Since Initial Audit

- Experimental UDP H3 is no longer a default-style product path; it now requires explicit opt-in and has a dedicated `triton lab` flow.
- Dashboard APIs now return structured summaries for status/config/probes/benches/traces, including trace metadata preview.
- Dashboard UI now renders typed cards, probe support summaries, bench health rollups, and a top-level overview panel instead of raw JSON-style scaffolding.
- Probe mode now emits partial-but-explicit support for `0rtt`, `migration`, `qpack`, `loss`, `congestion`, `retry`, `version`, `ecn`, and `spin-bit`, plus a `support_summary` rollup.
- Bench mode now emits a per-run `summary` rollup with healthy/degraded/failed protocol counts and best/risk protocol classification.
- Dashboard exposure is now materially safer: remote bind requires explicit opt-in plus auth, constant-time credential checks, and request timeouts.
- Insecure probe/bench TLS now requires explicit `allow_insecure_tls` opt-in, storage path traversal is blocked, and `gosec ./...` now passes with `0 issues`.
- CI now includes a dedicated CGO-capable `go test -race ./...` job, and parser fuzz targets plus targeted connection/stream/transport concurrency tests are in place for the experimental QUIC surface.
- Mixed real/experimental H3 listener startup now requires explicit intent (`allow_mixed_h3_planes`) to avoid accidental dual-plane exposure.
- A repository-level `ARCHITECTURE.md` now documents supported transport planes, lab-only boundaries, and runtime profiles.

The roadmap below should now be read as the remaining work after those improvements.

## Phase 1: Critical Alignment (Week 1-2)

### Must-fix items blocking a clean release story

- [x] Decide product positioning: ship Triton as a pragmatic HTTP/3 diagnostics tool powered by `quic-go`, while keeping the custom engine lab-only.
- [x] Finish the product-language cleanup so “real H3” and “experimental/lab H3” are unmistakable across all docs and operator output.
- [x] Separate “real H3” and “experimental H3” terminology consistently across README, config, CLI help, and dashboard.
- [x] Remove generated runtime artifacts and built binaries from the maintained source snapshot.

## Phase 2: Production Hardening (Week 3-4)

### Security, reliability, and operator clarity

- [x] Add explicit config validation for unsupported combinations such as both experimental and real H3 listeners being enabled without clear intent.
- [x] Add a startup banner or log summary that clearly states which listeners are real HTTP/3 vs experimental UDP H3.
- [ ] Watch the new CI `-race` job and fix any transport synchronization findings it reveals before broad release.
- [x] Add response/request-size coverage and negative-path tests around upload, drip, and trace download flows.

## Phase 3: Concurrency & Correctness (Week 5-6)

### Stabilize the experimental in-repo transport

- [ ] Keep tightening `stream` and `connection` state handling in response to CI `-race` findings.
- [ ] Keep tightening listener ordering/consumer semantics in response to CI `-race` findings.
- [ ] Expand fuzz coverage beyond packet/frame parsing into more transport and wire edge cases.
- [ ] Add transport/property tests for malformed packets and partial frame streams.

## Phase 4: Metrics & Benchmark Fidelity (Week 7-8)

### Make results trustworthy

- [ ] Turn existing percentile/phase/error summaries into stored historical comparisons and deltas between runs.
- [x] Document how `h1`, `h2`, `h3`, and `triton://` measurements differ so users do not compare incomparable transport stacks.
- [x] Add benchmark baselines or lightweight perf regression checks so the richer metrics become actionable.

## Phase 5: Dashboard Evolution (Week 9-10)

### Upgrade from JSON viewer to operator surface

- [x] Add filter/sort support for stored runs.
- [x] Add request/trace status indicators and empty-state UX.
- [x] Add compare/trend views that use the existing probe support summaries and bench rollups.
- [ ] Keep the dashboard static and dependency-free unless there is a strong reason to add a frontend toolchain.

## Phase 6: Spec Reconciliation (Week 11-14)

### Either narrow the spec or fund the missing engine work

- [x] Update `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, and `.project/TASKS.md` so they reflect the current architecture, hardening work, and dependency strategy.
- [x] If the custom engine remains in-scope: add a concrete vNext milestone for QUIC-TLS, packet protection, QPACK, and migration.
- [x] If the custom engine becomes lab-only: move it under an explicitly experimental namespace and reduce production promises accordingly.
- [x] Add `ARCHITECTURE.md` describing the three transport planes now in the repo.

## Phase 7: Deep Protocol Features (Week 15-20)

### Only if the custom-engine roadmap remains active

- [ ] Implement QUIC-TLS key schedule and packet protection for the in-repo transport.
- [ ] Add transport parameter handling, loss recovery, and congestion control.
- [ ] Replace the newline-based experimental H3 header codec with real QPACK.
- [ ] Implement 0-RTT and migration in either the custom transport or via honest wrappers around `quic-go`.
- [ ] Expand probe mode to include deep protocol analysis promised in the spec.

## Beyond v1.0: Future Enhancements

- [ ] Real-time dashboard updates via SSE or WebSocket.
- [ ] Historical comparison views and trend reports.
- [ ] Prometheus metrics export beyond the embedded `/metrics` surface.
- [ ] Distributed benchmarking and remote agents.
- [ ] A true protocol-inspection UI for qlog and packet/frame timelines.

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|---|---:|---|---|
| Phase 1 | 24h | Critical | None |
| Phase 2 | 28h | High | Phase 1 |
| Phase 3 | 32h | High | Phase 2 |
| Phase 4 | 32h | High | Phase 2 |
| Phase 5 | 32h | Medium | Phase 2 |
| Phase 6 | 24h | High | Phase 1 |
| Phase 7 | 160h+ | Strategic | Phase 6 |
| **Total to a strong pragmatic v1** | **140h** |  |  |
| **Total including custom-engine roadmap** | **300h+** |  |  |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Product messaging continues to outrun implementation | High | High | Align docs/config/CLI naming immediately |
| Experimental H3 path gets mistaken for the real supported server path | High | High | Disable by default or rename aggressively |
| Race issues surface once `-race` is added to CI | Medium | Medium | Fix stream/accept-channel synchronization before enabling gate |
| Benchmark numbers are over-interpreted as protocol-science quality | Medium | High | Improve metrics and document methodology |
| Custom QUIC roadmap absorbs team bandwidth without shipping user value | High | High | Decide early whether it is product-critical or research-only |
