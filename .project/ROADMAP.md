# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-14
> This roadmap prioritizes work needed to bring the current supported product path to production quality without pretending the experimental in-repo QUIC stack is ready.

## Current State Assessment

TritonProbe is already a functional operator tool. The supported path is: HTTPS/TCP test server, real HTTP/3 via `quic-go`, probe/bench result persistence, and an embedded read-only dashboard for status, stored probes, stored benches, and traces. Build, tests, and vet all pass locally.

For current-state scope questions, treat [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md) as the source of truth and read this roadmap as planning guidance layered on top of that boundary.

The main blockers are not "nothing works." The blockers are truth and scope: target-state docs still promise far more than the code delivers, several advanced probe features are heuristic estimates rather than packet-level implementations, and the experimental transport stack still needs to be clearly treated as research rather than a deployable engine.

What is working well:

- Stable HTTP/1.1, HTTP/2, and real HTTP/3 product path
- Strong config safety gates
- Healthy automated test coverage
- Useful dashboard/operator surface

## Phase 1: Critical Fixes (Week 1-2)

### Must-fix items blocking production trust

- [x] Reconcile architecture docs with the real supported boundary
  - Affected files: `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, `.project/TASKS.md`, `README.md`, `ARCHITECTURE.md`, `SUPPORTED.md`
  - Completed in: `README.md`, `ARCHITECTURE.md`, `SUPPORTED.md`, `.project/ROADMAP.md`
  - Notes: `SUPPORTED.md` is now treated as current-state authority and surrounding docs defer to it for shipped behavior
- [x] Make heuristic probe features much harder to misread as packet-level truth
  - Completed in: `internal/probe/probe.go`, `internal/cli/output.go`, `internal/dashboard/assets/app.js`
  - Notes: CLI/dashboard labels and `fidelity_summary` now distinguish `full`, `partial`, and `observed` fidelity, but packet-level implementations are still future work

## Phase 2: Core Completion (Week 3-6)

### Complete missing core features that fit the actual product direction

- [x] Add formal API and operator-surface documentation for dashboard endpoints
  - Spec reference: dashboard/API portions of Spec §9
  - Completed in: `API.md`
  - Notes: current route set, error shape, list query semantics, typed summary fields, and fidelity semantics are now documented
- [x] Improve probe and bench result detail views in dashboard
  - Completed in: `internal/dashboard/assets/index.html`, `internal/dashboard/assets/app.css`, `internal/dashboard/assets/app.js`
  - Notes: the embedded dashboard now exposes selected probe/bench detail cards with summary, fidelity, risk, and richer protocol health context
- [ ] Deepen trace browsing in dashboard
  - Current gap: result detail views are stronger, but trace browsing is still mostly list-and-open
  - Effort: 8-12h
- [x] Split `internal/probe/probe.go` into maintainable modules
  - Completed in: `internal/probe/probe.go`, `internal/probe/models.go`, `internal/probe/support.go`, `internal/probe/analytics.go`
  - Notes: probe orchestration remains in `probe.go`; models, support/fidelity logic, and analytics helpers are now separated
- [x] Add stronger CLI affordances around supported vs experimental modes
  - Completed in: `internal/cli/app.go`, `internal/cli/options.go`, `internal/server/server.go`
  - Notes: command help and startup warnings now describe the supported path separately from the lab-only transport surface

## Phase 3: Hardening (Week 7-8)

### Security, error handling, and operator safety

- [x] Add explicit mixed-fidelity labeling to 0-RTT, migration, QPACK, loss, congestion, retry, ECN, and spin-bit results
- [x] Push the same fidelity language into CLI, dashboard, and machine-readable result summaries
  - Completed in: `internal/probe/models.go`, `internal/probe/support.go`, `internal/cli/output.go`, `internal/dashboard/assets/app.js`
  - Notes: `full`, `observed`, and `partial` now share one canonical legend, and `version`/`retry`/`ecn` are consistently surfaced as `observed`
- [x] Review dashboard auth and remote-bind defaults again under a deployment checklist
  - Completed in: `OPERATIONS.md`, `CONFIG.md`, `TROUBLESHOOTING.md`
  - Notes: deployment guidance now spells out remote dashboard auth requirements, TLS choices, retention, and release-prep checks
- [ ] Add a startup/runtime banner when experimental transport is enabled outside loopback
- [x] Add documented retention, log, and trace disk-usage guidance
  - Completed in: `OPERATIONS.md`
- [ ] Decide whether to de-emphasize or quarantine `internal/quic/*` further for non-lab builds

## Phase 4: Testing (Week 9-10)

### Raise confidence in critical paths

- [ ] Make local and CI race testing symmetric where possible
  - Current gap: local `go test -race ./...` could not run with CGO disabled
- [x] Add API contract tests for dashboard list/filter/sort behavior
  - Completed in: `internal/dashboard/server_test.go`
  - Notes: filter, sort, limit, offset pagination, summary-view shaping, and list metadata headers are covered
- [x] Add regression tests around CLI output labels and fidelity summaries for heuristic probe features
- [x] Add browserless smoke tests for dashboard asset rendering
  - Completed in: `internal/dashboard/server_test.go`
  - Notes: embedded HTML, JS asset serving, HEAD handling, and renderer/pagination hints are exercised without a browser
- [ ] Add benchmark-style load checks for larger result stores

## Phase 5: Performance & Optimization (Week 11-12)

### Performance tuning for supported path

- [ ] Profile probe hot paths and repeated sampling overhead
- [ ] Reduce filesystem churn for large result/traces directories
- [x] Add pagination or more efficient filtering for larger dashboard result sets
  - Completed in: `internal/dashboard/server.go`, `internal/dashboard/assets/app.js`
- [x] Reduce dashboard list payload size for repeated summary views
  - Completed in: `internal/dashboard/server.go`, `internal/dashboard/assets/app.js`
  - Notes: `view=summary` now omits heavier raw list fields while preserving typed summary projections
- [x] Split `internal/dashboard/server.go` into maintainable modules
  - Completed in: `internal/dashboard/server.go`, `internal/dashboard/models.go`, `internal/dashboard/summaries.go`, `internal/dashboard/lists.go`
  - Notes: HTTP orchestration now stays in `server.go`; models, summary/cache logic, and list/query helpers are isolated
- [x] Reduce repeated filesystem list overhead for dashboard polling
  - Completed in: `internal/storage/filesystem.go`
  - Notes: storage category listings now reuse cached results when directory metadata is unchanged, while still invalidating on writes and external deletes
- [x] Cache repeated dashboard query/filter/sort/view combinations
  - Completed in: `internal/dashboard/server.go`, `internal/dashboard/lists.go`
  - Notes: repeated list requests now reuse cached post-filter/post-sort summary sets until underlying item signatures change
- [x] Persist compact probe/bench summaries beside full results
  - Completed in: `internal/storage/filesystem.go`, `internal/cli/app.go`, `internal/dashboard/summaries.go`
  - Notes: probe/bench commands now write sidecar summaries and dashboard list views prefer them, reducing first-hit decode cost after restart
- [x] Persist category-level summary manifest indexes
  - Completed in: `internal/storage/filesystem.go`
  - Notes: summary loads now prefer a persisted per-category `index.json`, so cold-start reads do not require opening every sidecar file individually
- [ ] Measure bundle/render cost of the embedded dashboard at larger data volumes
- [ ] Evaluate whether app-side compare/trend rendering should move partially server-side

## Phase 6: Documentation & DX (Week 13-14)

### Documentation and contributor clarity

- [x] Publish an authoritative "Supported Features" document
- [ ] Publish an "Experimental Lab Surface" document for `internal/quic/*` and `triton lab`
- [ ] Add a concise onboarding path for contributors
- [x] Add a config reference with examples for server, probe, bench, traces, and dashboard auth
  - Completed in: `CONFIG.md`
- [x] Document CI expectations, including race/staticcheck behavior
  - Completed in: `TROUBLESHOOTING.md`

## Phase 7: Release Preparation (Week 15-16)

### Final production preparation for the supported product path

- [x] Ensure release artifacts and `.goreleaser.yml` are still aligned with actual product positioning
  - Completed in: `.goreleaser.yml`, `OPERATIONS.md`, `README.md`, `scripts/ci-smoke.sh`, `scripts/ci-smoke.ps1`
  - Notes: release prep now includes smoke verification and the documented release checklist matches current product positioning
- [ ] Add version/build metadata to all user-visible surfaces
- [x] Add deployment checklist for self-signed vs custom cert operation
  - Completed in: `OPERATIONS.md`, `CONFIG.md`
  - Notes: docs now distinguish local runtime certificate use from shared or remote deployments that should provide custom cert/key material
- [x] Add operational monitoring guidance around `/healthz`, `/readyz`, `/metrics`, access logs, and traces
  - Completed in: `OPERATIONS.md`
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
