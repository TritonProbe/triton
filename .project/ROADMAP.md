# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-11
> This roadmap prioritizes work needed to bring Triton to production quality.

## Current State Assessment

Triton is a solid scaffold, not a production-ready QUIC/HTTP/3 platform. The CLI, config system, result persistence, local dashboard, and TCP/TLS test server all work, but the core promise of the product, a real custom QUIC and HTTP/3 test platform, is not implemented yet.

Key blockers for production readiness:

- No real QUIC TLS handshake or packet protection
- No network-capable HTTP/3 server/client
- No H3 benchmarking path
- No analytics/qlog/observability layer
- Weak security defaults and no auth
- No CI/CD or release automation

What is working well:

- Basic CLI workflow
- File-backed probe/bench persistence
- TCP/TLS test server
- Focused unit and loopback tests
- Small dependency surface

## Phase 1: Critical Fixes (Week 1-2)

### Must-fix items blocking basic functionality

- [ ] Remove committed runtime artifacts and key material from `triton-data/` and stop treating generated data as source
- [ ] Delete or consolidate duplicate handler implementations in `internal/appmux/mux.go` and `internal/server/endpoints.go`
- [ ] Fix insecure benchmark defaults in `internal/bench/bench.go`; certificate verification must be on by default
- [ ] Fix H3 loopback stream truncation in `internal/h3/loopback.go`
- [ ] Fix `stream.Stream` lock discipline in `internal/quic/stream/stream.go`
- [ ] Make CLI output truthful: either implement real table/markdown formatting or document JSON-only behavior

## Phase 2: Core Completion (Week 3-6)

### Complete missing core features from specification

- [ ] Implement QUIC packet protection and key schedule from SPEC §3 / TASKS 37-41
- [ ] Replace hardcoded toy dialer/listener handshake with a real transport state machine from TASKS 44-47
- [ ] Implement network-capable HTTP/3 request/response handling instead of loopback-only H3 helpers
- [ ] Complete missing server features: true H3 serving, `/push/:n` or remove it from claims, and truthful `quic-info`/`migration-test`
- [ ] Add actual `h3` protocol support to benchmark mode or remove it from product messaging
- [ ] Expand probe mode beyond HTTPS timing to at least handshake, TLS, throughput, streams, and Alt-Svc

## Phase 3: Hardening (Week 7-8)

### Security, error handling, edge cases

- [ ] Add bounded request-body handling for `/echo` and `/upload`
- [ ] Add dashboard authentication and safe bind defaults
- [ ] Add strict method enforcement on handlers to match API docs
- [ ] Introduce structured error responses for dashboard APIs
- [ ] Move server TLS minimum to 1.3 unless there is a compatibility reason not to
- [ ] Add config validation for currently unused or weakly validated fields

## Phase 4: Testing (Week 9-10)

### Comprehensive test coverage

- [ ] Add tests for `internal/cli`
- [ ] Add tests for `internal/appmux`
- [ ] Add tests for `internal/probe` including failure paths
- [ ] Add tests for `internal/bench`
- [ ] Add tests for `internal/dashboard`
- [ ] Enable race testing in CI with CGO-capable runners
- [ ] Add fuzz tests for packet/frame/header parsing promised in `TASKS.md`

## Phase 5: Performance & Optimization (Week 11-12)

### Performance tuning and optimization

- [ ] Replace naive H3 header codec with a real QPACK-aware implementation or explicitly demote current code to test-only
- [ ] Avoid per-request blocking/sleep pitfalls where they distort benchmark results
- [ ] Add histogram-based latency measurement instead of averages only
- [ ] Optimize dashboard asset serving with compression and cache headers
- [ ] Review allocation behavior in frame/header parsing once the real QUIC path exists

## Phase 6: Documentation & DX (Week 13-14)

### Documentation and developer experience

- [ ] Add `LICENSE`
- [ ] Add `CONTRIBUTING.md`
- [ ] Add `ARCHITECTURE.md` describing the actual architecture, not only the target one
- [ ] Add `API.md` for dashboard and server endpoints
- [ ] Document dev vs prod security posture explicitly
- [ ] Expand `Makefile` to include lint, race, cross-build, and release targets

## Phase 7: Release Preparation (Week 15-16)

### Final production preparation

- [ ] Add GitHub Actions for build, test, vet, staticcheck, and race runs
- [ ] Add `.goreleaser.yml`
- [ ] Rework Docker image to multi-stage minimal runtime, non-root user, and correct exposed ports
- [ ] Embed version/build metadata consistently
- [ ] Add observability basics: structured logs, health endpoints, request IDs
- [ ] Define rollback and support strategy

## Beyond v1.0: Future Enhancements

- [ ] Real-time dashboard charts, SSE, and WebSocket controls
- [ ] qlog export and protocol timeline visualization
- [ ] Distributed benchmarking
- [ ] Multipath QUIC and WebTransport exploration
- [ ] Prometheus/Grafana integration

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|---|---:|---|---|
| Phase 1 | 20h | Critical | None |
| Phase 2 | 160h | High | Phase 1 |
| Phase 3 | 40h | High | Phase 2 |
| Phase 4 | 50h | High | Phase 2 |
| Phase 5 | 40h | Medium | Phase 2 |
| Phase 6 | 24h | Medium | Phase 1 |
| Phase 7 | 36h | High | Phases 2-4 |
| **Total** | **370h** |  |  |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| QUIC/H3 scope remains much larger than current team bandwidth | High | High | Narrow v1 scope or extend schedule before claiming production readiness |
| Product messaging continues to outrun implementation | High | High | Align README/site/spec with actual shipped capabilities every release |
| Security shortcuts leak into release builds | Medium | High | Add CI policy checks for `InsecureSkipVerify`, committed keys, and exposed dashboard binds |
| Loopback scaffolding calcifies into public API | Medium | Medium | Clearly label experimental/test-only code and separate it from production packages |
| No CI leads to regressions once implementation expands | High | Medium | Add automated pipelines before major feature work continues |
