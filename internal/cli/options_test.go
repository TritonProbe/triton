package cli

import (
	"reflect"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/config"
)

func TestParseServerOptionsAndApply(t *testing.T) {
	opts, err := parseServerOptions([]string{
		"-config", "custom.yaml",
		"-listen", ":4443",
		"-listen-h3", ":4444",
		"-listen-tcp", ":9443",
		"-cert", "cert.pem",
		"-key", "key.pem",
		"-dashboard=false",
		"-dashboard-listen", "127.0.0.1:9191",
		"-dashboard-user", "admin",
		"-dashboard-pass", "secret",
		"-max-body-bytes", "2048",
		"-access-log", "logs/access.log",
		"-trace-dir", "traces/server",
	})
	if err != nil {
		t.Fatalf("parseServerOptions returned error: %v", err)
	}
	if opts.ConfigPath != "custom.yaml" || opts.ListenH3 != ":4444" || opts.AccessLog != "logs/access.log" || opts.TraceDir != "traces/server" {
		t.Fatalf("unexpected parsed options: %+v", opts)
	}

	cfg := config.Default()
	opts.Apply(&cfg)

	if cfg.Server.Listen != ":4443" || cfg.Server.ListenH3 != ":4444" || cfg.Server.ListenTCP != ":9443" {
		t.Fatalf("server listen options not applied: %+v", cfg.Server)
	}
	if cfg.Server.CertFile != "cert.pem" || cfg.Server.KeyFile != "key.pem" {
		t.Fatalf("TLS options not applied: %+v", cfg.Server)
	}
	if cfg.Server.Dashboard {
		t.Fatal("expected dashboard to be disabled")
	}
	if cfg.Server.DashboardListen != "127.0.0.1:9191" || cfg.Server.DashboardUser != "admin" || cfg.Server.DashboardPass != "secret" {
		t.Fatalf("dashboard options not applied: %+v", cfg.Server)
	}
	if cfg.Server.MaxBodyBytes != 2048 || cfg.Server.AccessLog != "logs/access.log" || cfg.Server.TraceDir != "traces/server" {
		t.Fatalf("server limits/logging options not applied: %+v", cfg.Server)
	}
}

func TestParseProbeOptionsAndApply(t *testing.T) {
	opts, err := parseProbeOptions([]string{
		"-config", "probe.yaml",
		"-target", "https://example.com",
		"-format", "json",
		"-timeout", "3s",
		"-insecure",
		"-trace-dir", "traces/probe",
	})
	if err != nil {
		t.Fatalf("parseProbeOptions returned error: %v", err)
	}
	if opts.ConfigPath != "probe.yaml" || opts.Target != "https://example.com" || opts.Format != "json" {
		t.Fatalf("unexpected parsed probe options: %+v", opts)
	}
	if opts.Timeout != 3*time.Second || !opts.Insecure || opts.TraceDir != "traces/probe" {
		t.Fatalf("probe flags not parsed correctly: %+v", opts)
	}

	cfg := config.Default()
	opts.Apply(&cfg)
	if cfg.Probe.Timeout != 3*time.Second || !cfg.Probe.Insecure || cfg.Probe.TraceDir != "traces/probe" {
		t.Fatalf("probe options not applied: %+v", cfg.Probe)
	}
	if got := opts.FormatOrDefault("table"); got != "json" {
		t.Fatalf("expected explicit format, got %q", got)
	}
}

func TestProbeFormatOrDefaultFallback(t *testing.T) {
	opts := probeOptions{}
	if got := opts.FormatOrDefault("yaml"); got != "yaml" {
		t.Fatalf("expected fallback format, got %q", got)
	}
}

func TestParseBenchOptionsAndApply(t *testing.T) {
	opts, err := parseBenchOptions([]string{
		"-config", "bench.yaml",
		"-target", "https://example.com",
		"-format", "markdown",
		"-duration", "5s",
		"-concurrency", "7",
		"-protocols", "h1, h2 ,h3",
		"-insecure",
		"-trace-dir", "traces/bench",
	})
	if err != nil {
		t.Fatalf("parseBenchOptions returned error: %v", err)
	}
	if opts.ConfigPath != "bench.yaml" || opts.Target != "https://example.com" || opts.Format != "markdown" {
		t.Fatalf("unexpected parsed bench options: %+v", opts)
	}
	if opts.Duration != 5*time.Second || opts.Concurrency != 7 || !opts.Insecure || opts.TraceDir != "traces/bench" {
		t.Fatalf("bench flags not parsed correctly: %+v", opts)
	}

	cfg := config.Default()
	opts.Apply(&cfg)
	if cfg.Bench.DefaultDuration != 5*time.Second || cfg.Bench.DefaultConcurrency != 7 || !cfg.Bench.Insecure || cfg.Bench.TraceDir != "traces/bench" {
		t.Fatalf("bench options not applied: %+v", cfg.Bench)
	}
	wantProtocols := []string{"h1", "h2", "h3"}
	if !reflect.DeepEqual(cfg.Bench.DefaultProtocols, wantProtocols) {
		t.Fatalf("unexpected protocols: got %v want %v", cfg.Bench.DefaultProtocols, wantProtocols)
	}
	if got := opts.FormatOrDefault("table"); got != "markdown" {
		t.Fatalf("expected explicit format, got %q", got)
	}
}

func TestBenchFormatOrDefaultFallback(t *testing.T) {
	opts := benchOptions{}
	if got := opts.FormatOrDefault("table"); got != "table" {
		t.Fatalf("expected fallback format, got %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" h1, ,h2, h3 ,, ")
	want := []string{"h1", "h2", "h3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected splitCSV result: got %v want %v", got, want)
	}
}

func TestValidateFormat(t *testing.T) {
	valid := []string{"", "table", "json", "yaml", "markdown"}
	for _, format := range valid {
		if err := validateFormat(format); err != nil {
			t.Fatalf("expected format %q to be valid: %v", format, err)
		}
	}
	if err := validateFormat("xml"); err == nil {
		t.Fatal("expected invalid format error")
	}
}
