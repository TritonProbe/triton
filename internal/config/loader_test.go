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
	t.Setenv("TRITON_SERVER_LISTEN_H3", ":5556")
	t.Setenv("TRITON_SERVER_ACCESS_LOG", "logs/access.jsonl")
	t.Setenv("TRITON_SERVER_TRACE_DIR", "traces/server")
	t.Setenv("TRITON_PROBE_TRACE_DIR", "traces/probe")
	t.Setenv("TRITON_PROBE_DEFAULT_STREAMS", "21")
	t.Setenv("TRITON_BENCH_TRACE_DIR", "traces/bench")
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
	if cfg.Server.AccessLog != "logs/access.jsonl" || cfg.Server.TraceDir != "traces/server" {
		t.Fatalf("server observability env not applied: %+v", cfg.Server)
	}
	if cfg.Server.Dashboard {
		t.Fatal("expected dashboard env override to disable dashboard")
	}
	if !cfg.Server.AllowRemoteDashboard {
		t.Fatal("expected remote dashboard env override to be applied")
	}
	if cfg.Probe.TraceDir != "traces/probe" || cfg.Probe.Timeout != 4*time.Second || cfg.Probe.DefaultStreams != 21 {
		t.Fatalf("probe env not applied: %+v", cfg.Probe)
	}
	wantProtocols := []string{"h2", "h3"}
	if cfg.Bench.TraceDir != "traces/bench" || !reflect.DeepEqual(cfg.Bench.DefaultProtocols, wantProtocols) {
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
  listen_h3: ":6002"
  listen_tcp: ":6003"
  dashboard: false
  max_body_bytes: 4096
probe:
  timeout: 6s
bench:
  default_duration: 3s
  default_concurrency: 2
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
	if cfg.Server.Dashboard || cfg.Server.MaxBodyBytes != 4096 {
		t.Fatalf("server yaml overrides not applied: %+v", cfg.Server)
	}
	if cfg.Probe.Timeout != 6*time.Second {
		t.Fatalf("probe timeout not loaded: %+v", cfg.Probe)
	}
	if cfg.Bench.DefaultDuration != 3*time.Second || cfg.Bench.DefaultConcurrency != 2 {
		t.Fatalf("bench yaml overrides not applied: %+v", cfg.Bench)
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
