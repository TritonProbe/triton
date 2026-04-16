# Configuration Reference

This document describes Triton's current configuration surface.

Current-state authority:

- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [API.md](/d:/Codebox/TritonProbe/API.md)
- [triton.yaml.example](/d:/Codebox/TritonProbe/triton.yaml.example)

Configuration precedence is:

1. built-in defaults
2. YAML config
3. environment variables
4. CLI flags

## Example

See [triton.yaml.example](/d:/Codebox/TritonProbe/triton.yaml.example) for a full baseline file.

## `server`

### `server.listen`

- Type: `host:port`
- Default: empty
- Purpose: experimental in-repo UDP H3 listener
- Safety rule: requires `server.allow_experimental_h3: true`
- Safety rule: non-loopback bind also requires `server.allow_remote_experimental_h3: true`

### `server.allow_experimental_h3`

- Type: boolean
- Default: `false`
- Purpose: explicit opt-in for experimental UDP H3

### `server.allow_remote_experimental_h3`

- Type: boolean
- Default: `false`
- Purpose: allow experimental UDP H3 on non-loopback interfaces

### `server.allow_mixed_h3_planes`

- Type: boolean
- Default: `false`
- Purpose: allow `server.listen` and `server.listen_h3` at the same time

### `server.listen_h3`

- Type: `host:port`
- Default: empty
- Purpose: supported real HTTP/3 listener via `quic-go`

### `server.listen_tcp`

- Type: `host:port`
- Default: `:8443`
- Purpose: supported HTTPS/TCP listener

### `server.cert` / `server.key`

- Type: filesystem path
- Default: empty
- Purpose: provide custom TLS certificate and key
- Validation: both must be set together
- Validation: files must exist

If omitted, Triton uses its existing runtime cert behavior instead of requiring user-provided cert files.

### `server.dashboard`

- Type: boolean
- Default: `true`
- Purpose: enable embedded dashboard

### `server.dashboard_listen`

- Type: `host:port`
- Default: `127.0.0.1:9090`
- Purpose: dashboard bind address
- Safety rule: non-loopback bind requires `server.allow_remote_dashboard: true`

### `server.allow_remote_dashboard`

- Type: boolean
- Default: `false`
- Purpose: allow dashboard on non-loopback interfaces
- Safety rule: requires both `server.dashboard_user` and `server.dashboard_pass`

### `server.dashboard_user` / `server.dashboard_pass`

- Type: string
- Default: empty
- Purpose: optional HTTP Basic Auth for dashboard
- Validation: must be provided together
- Requirement: mandatory when remote dashboard access is enabled

### `server.read_timeout`

- Type: duration
- Default: `15s`

### `server.write_timeout`

- Type: duration
- Default: `30s`

### `server.idle_timeout`

- Type: duration
- Default: `30s`

All server timeouts must be positive.

### `server.max_body_bytes`

- Type: integer
- Default: `1048576`
- Purpose: max accepted request body size for applicable endpoints
- Validation: must be positive

### `server.rate_limit`

- Type: integer
- Default: implementation-defined zero value unless set
- Purpose: request rate limiting on server surfaces

### `server.trace_dir`

- Type: filesystem path
- Default: empty
- Purpose: qlog / trace output directory

### `server.access_log`

- Type: filesystem path
- Default: empty
- Purpose: access log output destination when configured

## `probe`

### `probe.timeout`

- Type: duration
- Default: `10s`
- Validation: must be positive

### `probe.insecure`

- Type: boolean
- Default: `false`
- Purpose: disable TLS verification for probe requests
- Safety rule: requires `probe.allow_insecure_tls: true`

### `probe.allow_insecure_tls`

- Type: boolean
- Default: `false`
- Purpose: explicit acknowledgment for insecure probe TLS

### `probe.trace_dir`

- Type: filesystem path
- Default: empty
- Purpose: probe trace output directory

### `probe.default_tests`

- Type: string array
- Default:
  - `handshake`
  - `tls`
  - `latency`
  - `throughput`
  - `streams`
  - `alt-svc`

Use probe fidelity metadata to interpret advanced checks such as `0rtt`, `migration`, `qpack`, `loss`, `congestion`, `retry`, `version`, `ecn`, and `spin-bit`.

### `probe.default_format`

- Type: string
- Default: `table`

### `probe.download_size`

- Type: size string
- Default: `1MB`

### `probe.upload_size`

- Type: size string
- Default: `1MB`

### `probe.default_streams`

- Type: integer
- Default: `10`
- Validation: must be positive

## `bench`

### `bench.warmup`

- Type: duration
- Default: `2s`
- Validation: must be zero or positive

### `bench.default_duration`

- Type: duration
- Default: `10s`
- Validation: must be positive

### `bench.default_concurrency`

- Type: integer
- Default: `10`
- Validation: must be positive

### `bench.default_protocols`

- Type: string array
- Default:
  - `h1`
  - `h2`

Use `h3` explicitly when benchmarking supported real HTTP/3 targets.

### `bench.insecure`

- Type: boolean
- Default: `false`
- Safety rule: requires `bench.allow_insecure_tls: true`

### `bench.allow_insecure_tls`

- Type: boolean
- Default: `false`

### `bench.trace_dir`

- Type: filesystem path
- Default: empty

## `storage`

### `storage.results_dir`

- Type: filesystem path
- Default: `./triton-data`
- Validation: must not be empty

### `storage.max_results`

- Type: integer
- Default: `1000`
- Validation: must be positive

### `storage.retention`

- Type: duration
- Default: `720h`
- Validation: must be positive

## Validation Rules That Matter Most

Triton intentionally blocks a few risky combinations:

- at least one server listener must exist
- experimental UDP H3 needs explicit opt-in
- remote experimental UDP H3 needs an extra explicit opt-in
- real and experimental H3 together need explicit mixed-plane opt-in
- remote dashboard needs explicit opt-in plus auth
- insecure probe/bench TLS needs explicit opt-in
- cert/key must be provided together

## Recommended Safe Profiles

### Local supported development

```yaml
server:
  listen_tcp: ":8443"
  listen_h3: ":4434"
  dashboard: true
  dashboard_listen: "127.0.0.1:9090"
```

### Lab transport work

```yaml
server:
  listen: "127.0.0.1:4433"
  allow_experimental_h3: true
  dashboard: false
```

### Remote dashboard exposure

```yaml
server:
  dashboard: true
  dashboard_listen: "0.0.0.0:9090"
  allow_remote_dashboard: true
  dashboard_user: "admin"
  dashboard_pass: "change-me"
```

## Related Docs

- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [API.md](/d:/Codebox/TritonProbe/API.md)
- [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md)
- [README.md](/d:/Codebox/TritonProbe/README.md)
