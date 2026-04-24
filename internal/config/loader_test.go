package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadAppliesEnvOverrides(t *testing.T) {
	t.Setenv("TRITON_SERVER_LISTEN", ":5555")
	t.Setenv("TRITON_SERVER_ALLOW_EXPERIMENTAL_H3", "true")
	t.Setenv("TRITON_SERVER_ALLOW_REMOTE_EXPERIMENTAL_H3", "true")
	t.Setenv("TRITON_SERVER_ALLOW_MIXED_H3_PLANES", "true")
	t.Setenv("TRITON_SERVER_LISTEN_H3", ":5556")
	t.Setenv("TRITON_SERVER_ACCESS_LOG", "logs/access.jsonl")
	t.Setenv("TRITON_SERVER_TRACE_DIR", "traces/server")
	t.Setenv("TRITON_PROBE_TRACE_DIR", "traces/probe")
	t.Setenv("TRITON_PROBE_DEFAULT_STREAMS", "21")
	t.Setenv("TRITON_PROBE_ALLOW_INSECURE_TLS", "true")
	t.Setenv("TRITON_BENCH_TRACE_DIR", "traces/bench")
	t.Setenv("TRITON_BENCH_ALLOW_INSECURE_TLS", "true")
	t.Setenv("TRITON_PROBE_TIMEOUT", "4s")
	t.Setenv("TRITON_BENCH_DEFAULT_PROTOCOLS", "h2, h3")
	t.Setenv("TRITON_DASHBOARD_ENABLED", "false")
	t.Setenv("TRITON_SERVER_ALLOW_REMOTE_DASHBOARD", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Listen != ":5555" || cfg.Server.ListenH3 != ":5556" {
		t.Fatalf("server listen env not applied: %+v", cfg.Server)
	}
	if !cfg.Server.AllowExperimentalH3 {
		t.Fatal("expected experimental h3 env opt-in to be applied")
	}
	if !cfg.Server.AllowRemoteExperimentalH3 {
		t.Fatal("expected remote experimental h3 env opt-in to be applied")
	}
	if !cfg.Server.AllowMixedH3Planes {
		t.Fatal("expected mixed h3 planes env opt-in to be applied")
	}
	if cfg.Server.AccessLog != "logs/access.jsonl" || cfg.Server.TraceDir != "traces/server" {
		t.Fatalf("server observability env not applied: %+v", cfg.Server)
	}
	if cfg.Server.Dashboard {
		t.Fatal("expected dashboard env override to disable dashboard")
	}
	if !cfg.Server.AllowRemoteDashboard {
		t.Fatal("expected remote dashboard env override to be applied")
	}
	if cfg.Probe.TraceDir != "traces/probe" || cfg.Probe.Timeout != 4*time.Second || cfg.Probe.DefaultStreams != 21 || !cfg.Probe.AllowInsecureTLS {
		t.Fatalf("probe env not applied: %+v", cfg.Probe)
	}
	wantProtocols := []string{"h2", "h3"}
	if cfg.Bench.TraceDir != "traces/bench" || !reflect.DeepEqual(cfg.Bench.DefaultProtocols, wantProtocols) || !cfg.Bench.AllowInsecureTLS {
		t.Fatalf("bench env not applied: %+v", cfg.Bench)
	}
}

func TestLoadReadsYAMLFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "triton.yaml")
	content := []byte(`
server:
  listen: ":6001"
  allow_experimental_h3: true
  allow_remote_experimental_h3: true
  allow_mixed_h3_planes: true
  listen_h3: ":6002"
  listen_tcp: ":6003"
  dashboard: false
  max_body_bytes: 4096
probe:
  timeout: 6s
  thresholds:
    require_status_min: 200
    require_status_max: 299
bench:
  default_duration: 3s
  default_concurrency: 2
  default_format: markdown
probe_profiles:
  prod-edge:
    target: "https://example.com"
    default_tests: [latency, streams]
    thresholds:
      max_latency_p95_ms: 250
bench_profiles:
  staging-api:
    target: "https://example.com"
    default_protocols: [h1, h2, h3]
    thresholds:
      max_error_rate: 0.05
storage:
  results_dir: "./data"
  max_results: 50
  retention: 48h
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Listen != ":6001" || cfg.Server.ListenH3 != ":6002" || cfg.Server.ListenTCP != ":6003" {
		t.Fatalf("server config not read from yaml: %+v", cfg.Server)
	}
	if !cfg.Server.AllowRemoteExperimentalH3 {
		t.Fatalf("expected remote experimental h3 yaml override to be applied: %+v", cfg.Server)
	}
	if !cfg.Server.AllowMixedH3Planes {
		t.Fatalf("expected mixed h3 planes yaml override to be applied: %+v", cfg.Server)
	}
	if cfg.Server.Dashboard || cfg.Server.MaxBodyBytes != 4096 {
		t.Fatalf("server yaml overrides not applied: %+v", cfg.Server)
	}
	if cfg.Probe.Timeout != 6*time.Second {
		t.Fatalf("probe timeout not loaded: %+v", cfg.Probe)
	}
	if cfg.Probe.Thresholds.RequireStatusMin != 200 || cfg.Probe.Thresholds.RequireStatusMax != 299 {
		t.Fatalf("probe thresholds not loaded: %+v", cfg.Probe.Thresholds)
	}
	if cfg.Bench.DefaultDuration != 3*time.Second || cfg.Bench.DefaultConcurrency != 2 || cfg.Bench.DefaultFormat != "markdown" {
		t.Fatalf("bench yaml overrides not applied: %+v", cfg.Bench)
	}
	if cfg.ProbeProfiles["prod-edge"].Target != "https://example.com" || cfg.BenchProfiles["staging-api"].Thresholds.MaxErrorRate != 0.05 {
		t.Fatalf("profile data not loaded from yaml: %+v %+v", cfg.ProbeProfiles, cfg.BenchProfiles)
	}
	if cfg.Storage.MaxResults != 50 || cfg.Storage.Retention != 48*time.Hour {
		t.Fatalf("storage yaml overrides not applied: %+v", cfg.Storage)
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(cfgPath, []byte("server: ["), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected malformed yaml error")
	}
}

func TestLoadRejectsUnknownYAMLFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unknown.yaml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  listen_tcp: \":8443\"\n  unknown_field: true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected unknown yaml field error")
	}
}

func TestLoadRejectsInvalidTypedEnvOverride(t *testing.T) {
	t.Setenv("TRITON_SERVER_RATE_LIMIT", "not-an-int")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid env override to fail")
	}
}
