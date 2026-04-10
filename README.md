# Triton

Triton is a Go-based test server, probe, and benchmark scaffold for the TritonProbe project.

Current status:

- Buildable CLI with `server`, `probe`, `bench`, and `version`
- HTTPS server with self-signed certificate generation and test endpoints
- Simple dashboard with persisted probe and benchmark listings
- Probe and benchmark commands with JSON/YAML/Markdown/table output
- Tested custom QUIC building blocks: UDP transport, packet/header parsing, frame parsing, stream/connection lifecycle
- Minimal H3 loopback with `HEADERS + DATA` and `http.Handler` dispatch over the in-repo QUIC transport scaffold

Quick start:

```bash
go run ./cmd/triton version
go run ./cmd/triton server
go run ./cmd/triton probe --target https://example.com --format json
go run ./cmd/triton bench --target https://example.com --duration 3s --concurrency 4
```
