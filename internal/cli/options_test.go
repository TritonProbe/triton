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
		"-allow-experimental-h3",
		"-allow-remote-experimental-h3",
		"-allow-mixed-h3-planes",
		"-listen-h3", ":4444",
		"-listen-tcp", ":9443",
		"-cert", "cert.pem",
		"-key", "key.pem",
		"-dashboard=false",
		"-dashboard-listen", "0.0.0.0:9191",
		"-allow-remote-dashboard",
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
	if !cfg.Server.AllowExperimentalH3 {
		t.Fatalf("expected experimental h3 opt-in to be applied: %+v", cfg.Server)
	}
	if !cfg.Server.AllowRemoteExperimentalH3 {
		t.Fatalf("expected remote experimental h3 opt-in to be applied: %+v", cfg.Server)
	}
	if !cfg.Server.AllowMixedH3Planes {
		t.Fatalf("expected mixed h3 planes opt-in to be applied: %+v", cfg.Server)
	}
	if cfg.Server.CertFile != "cert.pem" || cfg.Server.KeyFile != "key.pem" {
		t.Fatalf("TLS options not applied: %+v", cfg.Server)
	}
	if cfg.Server.Dashboard {
		t.Fatal("expected dashboard to be disabled")
	}
	if cfg.Server.DashboardListen != "0.0.0.0:9191" || cfg.Server.DashboardUser != "admin" || cfg.Server.DashboardPass != "secret" {
		t.Fatalf("dashboard options not applied: %+v", cfg.Server)
	}
	if !cfg.Server.AllowRemoteDashboard {
		t.Fatalf("expected allow remote dashboard option to be applied: %+v", cfg.Server)
	}
	if cfg.Server.MaxBodyBytes != 2048 || cfg.Server.AccessLog != "logs/access.log" || cfg.Server.TraceDir != "traces/server" {
		t.Fatalf("server limits/logging options not applied: %+v", cfg.Server)
	}
}

func TestParseProbeOptionsAndApply(t *testing.T) {
	opts, err := parseProbeOptions([]string{
		"-config", "probe.yaml",
		"-profile", "prod-edge",
		"-target", "https://example.com",
		"-format", "json",
		"-report-out", "reports/probe.md",
		"-report-format", "yaml",
		"-timeout", "3s",
		"-streams", "12",
		"-tests", "latency,streams",
		"-0rtt",
		"-migration",
		"-insecure",
		"-allow-insecure-tls",
		"-trace-dir", "traces/probe",
		"-threshold-status-min", "200",
		"-threshold-status-max", "299",
		"-threshold-total-ms", "1500",
		"-threshold-latency-p95-ms", "300",
		"-threshold-stream-p95-ms", "450",
		"-threshold-stream-success-rate", "0.95",
		"-threshold-coverage-ratio", "0.80",
	})
	if err != nil {
		t.Fatalf("parseProbeOptions returned error: %v", err)
	}
	if opts.ConfigPath != "probe.yaml" || opts.Profile != "prod-edge" || opts.Target != "https://example.com" || opts.Format != "json" {
		t.Fatalf("unexpected parsed probe options: %+v", opts)
	}
	if opts.ReportOut != "reports/probe.md" || opts.ReportFormat != "yaml" {
		t.Fatalf("unexpected parsed report options: %+v", opts)
	}
	if opts.Timeout != 3*time.Second || !opts.Insecure || !opts.AllowInsecureTLS || opts.TraceDir != "traces/probe" || opts.Streams != 12 {
		t.Fatalf("probe flags not parsed correctly: %+v", opts)
	}
	if !reflect.DeepEqual(opts.selectedTests(nil), []string{"latency", "streams", "0rtt", "migration"}) {
		t.Fatalf("unexpected selected tests: %+v", opts.selectedTests(nil))
	}

	cfg := config.Default()
	opts.Apply(&cfg)
	if cfg.Probe.Timeout != 3*time.Second || !cfg.Probe.Insecure || !cfg.Probe.AllowInsecureTLS || cfg.Probe.TraceDir != "traces/probe" || cfg.Probe.DefaultStreams != 12 {
		t.Fatalf("probe options not applied: %+v", cfg.Probe)
	}
	if cfg.Probe.Thresholds.RequireStatusMin != 200 || cfg.Probe.Thresholds.RequireStatusMax != 299 || cfg.Probe.Thresholds.MaxTotalMS != 1500 {
		t.Fatalf("probe thresholds not applied: %+v", cfg.Probe.Thresholds)
	}
	if cfg.Probe.Thresholds.MaxLatencyP95MS != 300 || cfg.Probe.Thresholds.MaxStreamP95MS != 450 || cfg.Probe.Thresholds.MinStreamSuccessRate != 0.95 || cfg.Probe.Thresholds.MinCoverageRatio != 0.80 {
		t.Fatalf("probe threshold details not applied: %+v", cfg.Probe.Thresholds)
	}
	if !reflect.DeepEqual(cfg.Probe.DefaultTests, []string{"latency", "streams", "0rtt", "migration"}) {
		t.Fatalf("probe tests not applied: %+v", cfg.Probe.DefaultTests)
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

func TestProbeSelectedTestsFull(t *testing.T) {
	opts := probeOptions{Full: true}
	got := opts.selectedTests([]string{"latency"})
	want := []string{"handshake", "tls", "latency", "throughput", "streams", "alt-svc", "0rtt", "migration", "ecn", "retry", "version", "qpack", "congestion", "loss", "spin-bit"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected full test list: got %v want %v", got, want)
	}
}

func TestParseBenchOptionsAndApply(t *testing.T) {
	opts, err := parseBenchOptions([]string{
		"-config", "bench.yaml",
		"-profile", "staging-api",
		"-target", "https://example.com",
		"-format", "markdown",
		"-report-out", "reports/bench.md",
		"-report-format", "json",
		"-duration", "5s",
		"-concurrency", "7",
		"-protocols", "h1, h2 ,h3",
		"-insecure",
		"-allow-insecure-tls",
		"-trace-dir", "traces/bench",
		"-threshold-require-all-healthy",
		"-threshold-max-error-rate", "0.05",
		"-threshold-min-req-per-sec", "10",
		"-threshold-max-p95-ms", "250",
	})
	if err != nil {
		t.Fatalf("parseBenchOptions returned error: %v", err)
	}
	if opts.ConfigPath != "bench.yaml" || opts.Profile != "staging-api" || opts.Target != "https://example.com" || opts.Format != "markdown" {
		t.Fatalf("unexpected parsed bench options: %+v", opts)
	}
	if opts.ReportOut != "reports/bench.md" || opts.ReportFormat != "json" {
		t.Fatalf("unexpected bench report options: %+v", opts)
	}
	if opts.Duration != 5*time.Second || opts.Concurrency != 7 || !opts.Insecure || !opts.AllowInsecureTLS || opts.TraceDir != "traces/bench" {
		t.Fatalf("bench flags not parsed correctly: %+v", opts)
	}

	cfg := config.Default()
	opts.Apply(&cfg)
	if cfg.Bench.DefaultDuration != 5*time.Second || cfg.Bench.DefaultConcurrency != 7 || !cfg.Bench.Insecure || !cfg.Bench.AllowInsecureTLS || cfg.Bench.TraceDir != "traces/bench" {
		t.Fatalf("bench options not applied: %+v", cfg.Bench)
	}
	if !cfg.Bench.Thresholds.RequireAllHealthy || cfg.Bench.Thresholds.MaxErrorRate != 0.05 || cfg.Bench.Thresholds.MinReqPerSec != 10 || cfg.Bench.Thresholds.MaxP95MS != 250 {
		t.Fatalf("bench thresholds not applied: %+v", cfg.Bench.Thresholds)
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

func TestParseCheckOptions(t *testing.T) {
	opts, err := parseCheckOptions([]string{
		"-config", "check.yaml",
		"-profile", "prod",
		"-probe-profile", "probe-prod",
		"-bench-profile", "bench-prod",
		"-target", "https://example.com",
		"-format", "markdown",
		"-report-out", "reports/check.md",
		"-summary-out", "reports/check-summary.json",
		"-junit-out", "reports/check-junit.xml",
		"-report-format", "json",
		"-skip-bench",
	})
	if err != nil {
		t.Fatalf("parseCheckOptions returned error: %v", err)
	}
	if opts.ConfigPath != "check.yaml" || opts.Profile != "prod" || opts.ProbeProfile != "probe-prod" || opts.BenchProfile != "bench-prod" {
		t.Fatalf("unexpected parsed check options: %+v", opts)
	}
	if opts.Target != "https://example.com" || opts.Format != "markdown" || opts.ReportOut != "reports/check.md" || opts.ReportFormat != "json" {
		t.Fatalf("unexpected parsed output options: %+v", opts)
	}
	if opts.SummaryOut != "reports/check-summary.json" || opts.JUnitOut != "reports/check-junit.xml" {
		t.Fatalf("unexpected parsed machine-readable options: %+v", opts)
	}
	if !opts.SkipBench || opts.SkipProbe {
		t.Fatalf("unexpected parsed skip flags: %+v", opts)
	}
	if got := opts.FormatOrDefault("table"); got != "markdown" {
		t.Fatalf("expected explicit check format, got %q", got)
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

func TestValidateReportFormat(t *testing.T) {
	valid := []string{"", "json", "yaml", "markdown"}
	for _, format := range valid {
		if err := validateReportFormat(format); err != nil {
			t.Fatalf("expected report format %q to be valid: %v", format, err)
		}
	}
	if err := validateReportFormat("table"); err == nil {
		t.Fatal("expected invalid report format error")
	}
}
