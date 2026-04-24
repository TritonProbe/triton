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

Validation behavior:

- unknown YAML fields are rejected at load time
- invalid typed environment overrides fail startup instead of being silently ignored
- `server` and `lab` require at least one active listener after config, env, and flags are merged
- `probe`, `bench`, and `check` may use client-only config files with all server listeners disabled

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
- Validation: the certificate and key must load as a valid TLS pair

If omitted, Triton uses its existing runtime cert behavior instead of requiring user-provided cert files.

Certificate guidance:

- local-only or disposable environments can use the runtime certificate behavior
- shared, remote, or operator-facing environments should set both `server.cert` and `server.key`
- if clients outside your machine will connect, treat custom certificates as the safer default
- remote dashboard mode uses the same resolved certificate/key pair as the supported server surfaces

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
- Safety rule: requires explicit `server.cert` and `server.key`
- Runtime behavior: when enabled, the dashboard is served over HTTPS using the resolved server certificate and key material

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

### `probe.thresholds`

- Type: object
- Default: empty / disabled thresholds
- Purpose: define pass/fail conditions for probe and `check` workflows

Supported fields:

- `require_status_min`
- `require_status_max`
- `max_total_ms`
- `max_latency_p95_ms`
- `max_stream_p95_ms`
- `min_stream_success_rate`
- `min_coverage_ratio`

Threshold ratios use `0..1` values.

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

### `bench.default_format`

- Type: string
- Default: `table`

### `bench.thresholds`

- Type: object
- Default: empty / disabled thresholds
- Purpose: define pass/fail conditions for bench and `check` workflows

Supported fields:

- `require_all_healthy`
- `max_error_rate`
- `min_req_per_sec`
- `max_p95_ms`

Threshold ratios use `0..1` values.

## `probe_profiles`

### `probe_profiles.<name>`

- Type: object
- Purpose: reusable named probe workflow for `triton probe --profile ...` and `triton check`

Supported fields:

- `target`
- `report_name`
- `timeout`
- `insecure`
- `allow_insecure_tls`
- `trace_dir`
- `default_tests`
- `default_format`
- `default_streams`
- `thresholds`

Validation rules:

- `target` must be non-empty
- `default_streams` must be zero or positive
- `default_format` must be one of `table`, `json`, `yaml`, or `markdown`

## `bench_profiles`

### `bench_profiles.<name>`

- Type: object
- Purpose: reusable named bench workflow for `triton bench --profile ...` and `triton check`

Supported fields:

- `target`
- `report_name`
- `warmup`
- `default_duration`
- `default_concurrency`
- `default_protocols`
- `default_format`
- `insecure`
- `allow_insecure_tls`
- `trace_dir`
- `thresholds`

Validation rules:

- `target` must be non-empty
- durations and concurrency must be zero or positive
- `default_format` must be one of `table`, `json`, `yaml`, or `markdown`

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

Operational note:

- retention and trace directories should be planned together so result JSON and qlog files do not grow without review

## Validation Rules That Matter Most

Triton intentionally blocks a few risky combinations:

- server/lab mode requires at least one server listener
- client-only probe/bench/check configs may disable all server listeners
- experimental UDP H3 needs explicit opt-in
- remote experimental UDP H3 needs an extra explicit opt-in
- real and experimental H3 together need explicit mixed-plane opt-in
- remote dashboard needs explicit opt-in plus auth
- remote dashboard also needs explicit cert/key files
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

### Remote supported deployment

```yaml
server:
  listen_tcp: ":8443"
  listen_h3: ":4434"
  cert: "/etc/triton/tls/fullchain.pem"
  key: "/etc/triton/tls/privkey.pem"
  dashboard: true
  dashboard_listen: "127.0.0.1:9090"
  trace_dir: "/var/lib/triton/traces"
storage:
  results_dir: "/var/lib/triton/results"
  max_results: 1000
  retention: "168h"
```

Notes:

- keep the dashboard on loopback unless you have a real remote access requirement
- enable `server.listen_h3` only when you actually intend to serve supported HTTP/3
- size `storage.retention` and trace directories with disk headroom in mind

### Reusable CI / check profile

```yaml
probe_profiles:
  production-edge:
    target: "https://example.com"
    report_name: "Production Edge"
    default_tests: ["handshake", "latency", "streams"]
    thresholds:
      require_status_min: 200
      require_status_max: 299
      max_latency_p95_ms: 250
      min_stream_success_rate: 0.95

bench_profiles:
  production-edge:
    target: "https://example.com"
    report_name: "Production Edge"
    default_protocols: ["h1", "h2", "h3"]
    thresholds:
      require_all_healthy: true
      max_error_rate: 0.05
      min_req_per_sec: 10
      max_p95_ms: 300
```

This enables:

- `triton probe --profile production-edge`
- `triton bench --profile production-edge`
- `triton check --profile production-edge`

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
  cert: "/etc/triton/tls/fullchain.pem"
  key: "/etc/triton/tls/privkey.pem"
  dashboard_user: "admin"
  dashboard_pass: "change-me"
```

## Related Docs

- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [API.md](/d:/Codebox/TritonProbe/API.md)
- [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md)
- [README.md](/d:/Codebox/TritonProbe/README.md)
