package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tritonprobe/triton/internal/config"
)

func TestAppHelp(t *testing.T) {
	var out bytes.Buffer
	app := NewApp("dev", "unknown")
	app.SetStdout(&out)
	if err := app.Run(nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Supported product path:") || !strings.Contains(got, "Experimental surface:") || !strings.Contains(got, "lab-only transport research") {
		t.Fatalf("unexpected help output: %q", got)
	}
}

func TestAppCommandHelpClarifiesBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
	}{
		{
			name: "server",
			args: []string{"server", "--help"},
			contains: []string{
				"Usage: triton server [flags]",
				"Supported runtime:",
				"Experimental surface:",
				"lab-only",
			},
		},
		{
			name: "lab",
			args: []string{"lab", "--help"},
			contains: []string{
				"Usage: triton lab [flags]",
				"Lab-only runtime:",
				"transport research",
			},
		},
		{
			name: "probe",
			args: []string{"probe", "--help"},
			contains: []string{
				"Usage: triton probe [flags] [target]",
				"Supported targets:",
				"triton://... is lab-only",
				"full, observed, or partial",
			},
		},
		{
			name: "bench",
			args: []string{"bench", "--help"},
			contains: []string{
				"Usage: triton bench [flags] [target]",
				"Supported comparisons:",
				"triton://... uses the lab transport",
			},
		},
		{
			name: "check",
			args: []string{"check", "--help"},
			contains: []string{
				"Usage: triton check [flags]",
				"Product workflow:",
				"Profile selection:",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			app := NewApp("dev", "unknown")
			app.SetStdout(&out)
			if err := app.Run(tc.args); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			got := out.String()
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("expected help output to contain %q, got %q", want, got)
				}
			}
		})
	}
}

func TestAppVersion(t *testing.T) {
	var out bytes.Buffer
	app := NewApp("1.2.3", "now")
	app.SetStdout(&out)
	if err := app.Run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "1.2.3") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRequireTarget(t *testing.T) {
	target, err := requireTarget([]string{"https://example.com", "-format", "json"})
	if err != nil {
		t.Fatalf("requireTarget returned error: %v", err)
	}
	if target != "https://example.com" {
		t.Fatalf("unexpected target: %q", target)
	}
}

func TestRequireTargetMissing(t *testing.T) {
	if _, err := requireTarget([]string{"-format", "json", "-timeout=1s"}); err == nil {
		t.Fatal("expected missing target error")
	}
}

func TestRequireTargetSkipsKnownFlagValues(t *testing.T) {
	target, err := requireTarget([]string{"-format", "json", "-insecure", "https://example.com"})
	if err != nil {
		t.Fatalf("requireTarget returned error: %v", err)
	}
	if target != "https://example.com" {
		t.Fatalf("unexpected target: %q", target)
	}
}

func TestFlagTakesValue(t *testing.T) {
	if !flagTakesValue("-format") || !flagTakesValue("--trace-dir") || !flagTakesValue("--profile") || !flagTakesValue("--probe-profile") || !flagTakesValue("--threshold-max-p95-ms") {
		t.Fatal("expected known flags to require values")
	}
	if !flagTakesValue("--summary-out") || !flagTakesValue("--junit-out") {
		t.Fatal("expected machine-readable check flags to require values")
	}
	if flagTakesValue("-insecure") {
		t.Fatal("expected boolean flag not to require value")
	}
}

func TestResolveTargetFallsBackToProfile(t *testing.T) {
	target, err := resolveTarget("", []string{"--profile", "prod-edge"}, "https://example.com")
	if err != nil {
		t.Fatalf("resolveTarget returned error: %v", err)
	}
	if target != "https://example.com" {
		t.Fatalf("unexpected target: %q", target)
	}
}

func TestResolveCheckProfilesFromSharedProfile(t *testing.T) {
	cfg := loadCheckTestConfig()
	probeName, benchName, err := resolveCheckProfiles(cfg, checkOptions{Profile: "prod"})
	if err != nil {
		t.Fatalf("resolveCheckProfiles returned error: %v", err)
	}
	if probeName != "prod" || benchName != "prod" {
		t.Fatalf("unexpected resolved profiles: %q %q", probeName, benchName)
	}
}

func TestAppCheckRunsAgainstTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", `h3=":443"; ma=60`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "triton.yaml")
	summaryPath := filepath.Join(dir, "check-summary.json")
	junitPath := filepath.Join(dir, "check-junit.xml")
	cfg := "probe:\n  timeout: 500ms\n  default_tests: [handshake]\n  default_streams: 1\nbench:\n  default_protocols: [h1]\n  default_duration: 50ms\n  default_concurrency: 1\nstorage:\n  results_dir: \"" + filepath.ToSlash(filepath.Join(dir, "data")) + "\"\n  max_results: 10\n  retention: 24h\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var out bytes.Buffer
	app := NewApp("dev", "unknown")
	app.SetStdout(&out)
	if err := app.Run([]string{"check", "-config", cfgPath, "-target", srv.URL, "-summary-out", summaryPath, "-junit-out", junitPath}); err != nil {
		t.Fatalf("check command returned error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Check Result") || !strings.Contains(got, "passed") {
		t.Fatalf("unexpected check output: %q", got)
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("ReadFile(summary) returned error: %v", err)
	}
	if !strings.Contains(string(summary), "\"kind\": \"check_summary\"") || !strings.Contains(string(summary), "\"passed\": true") {
		t.Fatalf("unexpected summary output: %s", string(summary))
	}
	junit, err := os.ReadFile(junitPath)
	if err != nil {
		t.Fatalf("ReadFile(junit) returned error: %v", err)
	}
	if !strings.Contains(string(junit), "<testsuites") || !strings.Contains(string(junit), "testcase") {
		t.Fatalf("unexpected junit output: %s", string(junit))
	}
}

func TestAppProbeAllowsClientOnlyConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client-only.yaml")
	cfg := "server:\n  listen: \"\"\n  listen_h3: \"\"\n  listen_tcp: \"\"\n  dashboard: false\nprobe:\n  timeout: 500ms\n  default_tests: [handshake]\nstorage:\n  results_dir: \"" + filepath.ToSlash(filepath.Join(dir, "data")) + "\"\n  max_results: 10\n  retention: 24h\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var out bytes.Buffer
	app := NewApp("dev", "unknown")
	app.SetStdout(&out)
	if err := app.Run([]string{"probe", "-config", cfgPath, "-target", srv.URL, "-format", "json"}); err != nil {
		t.Fatalf("probe command returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"status": 200`) {
		t.Fatalf("unexpected probe output: %q", out.String())
	}
}

func loadCheckTestConfig() config.Config {
	cfg := config.Default()
	cfg.ProbeProfiles = map[string]config.ProbeProfile{
		"prod": {Target: "https://example.com"},
	}
	cfg.BenchProfiles = map[string]config.BenchProfile{
		"prod": {Target: "https://example.com"},
	}
	return cfg
}

func TestAppUnknownCommand(t *testing.T) {
	app := NewApp("dev", "unknown")
	if err := app.Run([]string{"wat"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}
