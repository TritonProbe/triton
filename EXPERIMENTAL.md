# Experimental Lab Surface

This document describes the parts of Triton that are intentionally experimental.

Use it together with [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md) when deciding what is safe to run, describe, or support today.

## What This Covers

The experimental lab surface currently includes:

- `triton lab`
- `triton://...` targets
- `server.listen` for the in-repo UDP H3 listener
- `internal/quic/*`
- `internal/h3/*`

These areas are useful for transport research, controlled loopback experiments, and protocol learning.

They are not the supported production-like HTTP/3 path.

## Supported Alternative

If you need the supported product path, use:

- `triton server` with `server.listen_tcp`
- optional `server.listen_h3`
- `triton probe --target https://...`
- `triton probe --target h3://...`
- `triton bench` against normal HTTPS targets

The supported HTTP/3 implementation path is `quic-go` via `internal/realh3`.

## Safety Rules

Triton intentionally keeps the lab surface behind explicit gates:

- `server.listen` requires `allow_experimental_h3`
- non-loopback experimental bind requires `allow_remote_experimental_h3`
- mixing real HTTP/3 and experimental UDP H3 requires `allow_mixed_h3_planes`
- `triton lab` defaults to an isolated loopback listener and disables the normal HTTPS/dashboard profile

Recommended usage:

- prefer `triton lab` for isolated transport work
- keep experimental listeners on loopback unless there is a deliberate lab reason not to
- treat mixed-plane runs as research, not as a normal deployment profile

## What Not To Claim

Do not describe the experimental lab surface as:

- production-ready QUIC
- RFC-complete HTTP/3
- the supported deployment path
- packet-accurate proof of future roadmap items

In particular, `internal/quic/*` and `internal/h3/*` should be read as research code and protocol-building blocks, not as a supported engine.

## Appropriate Use Cases

Good fits:

- protocol experiments
- loopback transport testing
- educational inspection of QUIC/H3 building blocks
- internal development and research

Bad fits:

- external production claims
- customer-facing deployment promises
- remote exposure without deliberate lab controls
- using lab behavior as proof that the supported path has the same transport properties

## Operator Guidance

If experimental transport is enabled:

- review startup logs for the experimental and mixed-plane warnings
- keep the deployment description honest about which plane is serving traffic
- separate lab findings from supported-path operational guidance

For deployment, support, and operator decisions, prefer:

1. [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
2. [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
3. [OPERATIONS.md](/d:/Codebox/TritonProbe/OPERATIONS.md)
4. this document
