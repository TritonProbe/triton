# Contributing

This guide is the shortest practical path for contributing to Triton.

## Start Here

Read these in order before making non-trivial changes:

1. [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
2. [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md)
3. [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
4. [API.md](/d:/Codebox/TritonProbe/API.md)
5. [OPERATIONS.md](/d:/Codebox/TritonProbe/OPERATIONS.md)

Why this order:

- `SUPPORTED.md` tells you what is actually supported today
- `EXPERIMENTAL.md` tells you what is still lab-only
- the remaining docs explain how that supported surface is configured and operated

If a target-state document under `.project/` sounds larger than the running product, prefer the current-state docs above.

## Product Boundary

Triton's supported path today is:

- `triton server` with HTTPS/TCP
- optional real HTTP/3 via `quic-go`
- `probe` for `https://...` and `h3://...`
- `bench` for normal HTTPS targets
- the embedded dashboard and storage APIs

Treat these as lab-only:

- `triton lab`
- `triton://...`
- `internal/quic/*`
- `internal/h3/*`

Do not present lab surfaces as production-ready in code, docs, comments, or tests.

## Local Setup

Typical commands:

```bash
go test ./...
go vet ./...
staticcheck ./...
gosec ./...
```

Helpful shortcuts from [Makefile](/d:/Codebox/TritonProbe/Makefile):

- `make test`
- `make test-race`
- `make test-fuzz`
- `make lint`
- `make security`
- `make smoke`
- `make perf-check`

Smoke flow helpers:

- `bash ./scripts/ci-smoke.sh` on bash-capable environments
- `pwsh -File ./scripts/ci-smoke.ps1` on Windows/PowerShell

Coverage artifacts such as `coverage`, `coverage_review`, and `coverage.out` are local-only and should not be committed.

## Safe Change Strategy

When choosing work, prefer:

- supported-path fixes
- dashboard/operator clarity
- fidelity labeling and accuracy
- tests and documentation that reduce ambiguity

Be cautious around:

- claims about packet-level QUIC behavior
- widening the supported boundary without docs and safety review
- changing lab surfaces in a way that makes them look officially supported

## Common Work Areas

Rough code map:

- [cmd/triton](/d:/Codebox/TritonProbe/cmd/triton): binary entrypoint
- [internal/cli](/d:/Codebox/TritonProbe/internal/cli): command parsing and output rendering
- [internal/server](/d:/Codebox/TritonProbe/internal/server): supported runtime wiring
- [internal/appmux](/d:/Codebox/TritonProbe/internal/appmux): HTTP endpoints and capability document
- [internal/probe](/d:/Codebox/TritonProbe/internal/probe): probe execution and fidelity/support summaries
- [internal/bench](/d:/Codebox/TritonProbe/internal/bench): benchmark execution and summaries
- [internal/dashboard](/d:/Codebox/TritonProbe/internal/dashboard): embedded dashboard API and UI assets
- [internal/storage](/d:/Codebox/TritonProbe/internal/storage): persisted results and summary indexes
- [internal/realh3](/d:/Codebox/TritonProbe/internal/realh3): supported real HTTP/3 path

## Tests And Verification

Before pushing a meaningful change, aim to run the smallest relevant set plus `go test ./...`.

Examples:

- CLI/help/output change:
  run `go test ./internal/cli ./cmd/triton`
- dashboard/API change:
  run `go test ./internal/dashboard ./internal/appmux ./internal/server`
- probe or bench change:
  run `go test ./internal/probe ./internal/bench`
- release/runtime change:
  run `go test ./...` and a smoke flow

If your local machine cannot run race or bash-based helpers, say so clearly in the PR or change note.

## Docs Expectations

If your change affects behavior, update the matching docs:

- supported boundary: [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- lab boundary: [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md)
- config knobs: [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
- operator behavior: [OPERATIONS.md](/d:/Codebox/TritonProbe/OPERATIONS.md)
- dashboard/API response shape: [API.md](/d:/Codebox/TritonProbe/API.md)

Prefer small, honest doc changes over broad aspirational wording.

## Commit And PR Guidance

Good changes here tend to be:

- small enough to explain in one paragraph
- backed by focused tests when behavior changes
- explicit about supported vs experimental impact

Good commit messages describe the user-visible outcome, for example:

- `Clarify supported and experimental CLI boundaries`
- `Expose build metadata across runtime surfaces`
- `Document the experimental lab surface`

## If You Are Unsure

When a change touches product positioning, ask:

1. Does this affect the supported path or only the lab path?
2. Would a new contributor mistake this for a production-ready feature?
3. Which current-state doc should change with the code?

If that answer is fuzzy, tighten the docs and labels before widening the feature claim.
