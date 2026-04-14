# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-14
> This roadmap prioritizes work needed to bring the current supported product path to production quality without pretending the experimental in-repo QUIC stack is ready.

## Current State Assessment

TritonProbe is already a functional operator tool. The supported path is: HTTPS/TCP test server, real HTTP/3 via `quic-go`, probe/bench result persistence, and an embedded read-only dashboard for status, stored probes, stored benches, and traces. Build, tests, and vet all pass locally.

The main blockers are not "nothing works." The blockers are truth and scope: target-state docs still promise far more than the code delivers, several advanced probe features are heuristic estimates rather than packet-level implementations, and the experimental transport stack still needs to be clearly treated as research rather than a deployable engine.

What is working well:

- Stable HTTP/1.1, HTTP/2, and real HTTP/3 product path
- Strong config safety gates
- Healthy automated test coverage
- Useful dashboard/operator surface

## Phase 1: Critical Fixes (Week 1-2)

### Must-fix items blocking production trust

- [ ] Reconcile architecture docs with the real supported boundary
  - Affected files: `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, `.project/TASKS.md`, `README.md`, `ARCHITECTURE.md`
  - Effort: 8h
- [ ] Add the missing root `LICENSE` file or stop claiming MIT
  - Affected files: `LICENSE`, docs
  - Effort: 1h
- [ ] Make heuristic probe features impossible to misread as packet-level truth
  - Affected files: `internal/probe/probe.go`, `internal/cli/output.go`, `internal/dashboard/assets/app.js`
  - Effort: 6h
- [ ] Fix the local `staticcheck` failure in `internal/quic/wire/more_test.go`
  - Affected files: `internal/quic/wire/more_test.go`
  - Effort: 1h

## Phase 2: Core Completion (Week 3-6)

### Complete missing core features that fit the actual product direction

- [ ] Add formal API and operator-surface documentation for dashboard endpoints
  - Spec reference: dashboard/API portions of Spec §9
  - Current gap: JSON endpoints exist but have no formal contract doc
  - Effort: 6h
- [ ] Improve trace browsing and result detail views in dashboard
  - Current gap: overview is useful but still shallow
  - Effort: 12-18h
- [ ] Split `internal/probe/probe.go` into maintainable modules
  - Current gap: orchestration and analytics are tightly packed
  - Effort: 12-20h
- [ ] Add stronger CLI affordances around supported vs experimental modes
  - Current gap: `lab` exists, but the distinction can still be clearer in help/output
  - Effort: 4-6h

## Phase 3: Hardening (Week 7-8)

### Security, error handling, and operator safety

- [ ] Add explicit "estimated/heuristic" tags to 0-RTT, migration, QPACK, loss, congestion, retry, ECN, and spin-bit results
- [ ] Review dashboard auth and remote-bind defaults again under a deployment checklist
- [ ] Add a startup/runtime banner when experimental transport is enabled outside loopback
- [ ] Add documented retention, log, and trace disk-usage guidance
- [ ] Decide whether to de-emphasize or quarantine `internal/quic/*` further for non-lab builds

## Phase 4: Testing (Week 9-10)

### Raise confidence in critical paths

- [ ] Make local and CI race testing symmetric where possible
  - Current gap: local `go test -race ./...` could not run with CGO disabled
- [ ] Add API contract tests for dashboard list/filter/sort behavior
- [ ] Add regression tests around CLI output labels for heuristic probe features
- [ ] Add browserless smoke tests for dashboard asset rendering
- [ ] Add benchmark-style load checks for larger result stores

## Phase 5: Performance & Optimization (Week 11-12)

### Performance tuning for supported path

- [ ] Profile probe hot paths and repeated sampling overhead
- [ ] Reduce filesystem churn for large result/traces directories
- [ ] Add pagination or more efficient filtering for larger dashboard result sets
- [ ] Measure bundle/render cost of the embedded dashboard at larger data volumes
- [ ] Evaluate whether app-side compare/trend rendering should move partially server-side

## Phase 6: Documentation & DX (Week 13-14)

### Documentation and contributor clarity

- [ ] Publish an authoritative "Supported Features" document
- [ ] Publish an "Experimental Lab Surface" document for `internal/quic/*` and `triton lab`
- [ ] Add a concise onboarding path for contributors
- [ ] Add a config reference with examples for server, probe, bench, traces, and dashboard auth
- [ ] Document CI expectations, including race/staticcheck behavior

## Phase 7: Release Preparation (Week 15-16)

### Final production preparation for the supported product path

- [ ] Ensure release artifacts and `.goreleaser.yml` are still aligned with actual product positioning
- [ ] Add version/build metadata to all user-visible surfaces
- [ ] Add deployment checklist for self-signed vs custom cert operation
- [ ] Add operational monitoring guidance around `/healthz`, `/readyz`, `/metrics`, access logs, and traces
- [ ] Verify container image hardening and runtime assumptions

## Beyond v1.0: Future Enhancements

### Only pursue these if they remain strategic

- [ ] Decide whether the custom QUIC/H3 engine should ever graduate from lab status
- [ ] If yes, implement real QUIC-TLS, packet protection, and QPACK before any production claims
- [ ] If no, slim the experimental surface and position it clearly as transport research
- [ ] Expand the dashboard into a richer workbench only after the product boundary is stable

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|---|---|---|---|
| Phase 1 | 16h | CRITICAL | None |
| Phase 2 | 34h | HIGH | Phase 1 |
| Phase 3 | 24h | HIGH | Phase 1 |
| Phase 4 | 22h | HIGH | Phase 1-3 |
| Phase 5 | 18h | MEDIUM | Phase 2-4 |
| Phase 6 | 18h | MEDIUM | Phase 1 complete |
| Phase 7 | 14h | MEDIUM | Phase 1-6 |
| **Total** | **146h** |  |  |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Operators assume heuristic probe outputs are packet-accurate | High | High | Relabel outputs and docs aggressively |
| Contributors mistake experimental QUIC stack for the supported product path | High | High | Separate docs and CLI messaging |
| Dashboard filesystem approach degrades with large retained result sets | Medium | Medium | Add pagination, retention guidance, and profiling |
| Local quality gates diverge from CI quality gates | Medium | Medium | Align race/staticcheck expectations and docs |
| Architectural docs continue to drift from code | High | Medium | Maintain one authoritative supported-boundary document |
