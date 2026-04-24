package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/probe"
)

func TestEvaluateProbeThresholds(t *testing.T) {
	result := probe.Result{
		Target:   "https://example.com",
		Status:   503,
		Duration: 2 * time.Second,
		Timings:  map[string]int64{"total": 2000},
		Analysis: map[string]any{
			"latency":         probe.LatencyAnalysis{P95: 420},
			"streams":         probe.StreamAnalysis{P95Latency: 520, SuccessRate: 0.70},
			"support_summary": probe.SupportSummary{CoverageRatio: 0.5},
		},
	}
	violations := evaluateProbeThresholds(result, config.ProbeThresholds{
		RequireStatusMin:     200,
		RequireStatusMax:     299,
		MaxTotalMS:           1000,
		MaxLatencyP95MS:      300,
		MaxStreamP95MS:       400,
		MinStreamSuccessRate: 0.9,
		MinCoverageRatio:     0.8,
	})
	if len(violations) != 6 {
		t.Fatalf("expected 6 violations, got %d: %v", len(violations), violations)
	}
}

func TestEvaluateBenchThresholds(t *testing.T) {
	result := bench.Result{
		Stats: map[string]bench.Stats{
			"h3": {RequestsPerS: 8, ErrorRate: 0.2, Latency: bench.Percentiles{P95: 320}},
		},
	}
	violations := evaluateBenchThresholds(result, config.BenchThresholds{
		RequireAllHealthy: true,
		MaxErrorRate:      0.1,
		MinReqPerSec:      10,
		MaxP95MS:          250,
	})
	if len(violations) != 4 {
		t.Fatalf("expected 4 violations, got %d: %v", len(violations), violations)
	}
}

func TestWriteProbeReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "probe.md")
	err := writeProbeReport(path, "markdown", probeReportOptions{
		ProfileName: "prod-edge",
		ReportName:  "Production Edge",
		Result: probe.Result{
			Target:   "https://example.com",
			Status:   200,
			Proto:    "HTTP/2.0",
			Duration: 500 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("writeProbeReport returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# Probe Execution Report") || !strings.Contains(text, "Production Edge") {
		t.Fatalf("unexpected report body: %s", text)
	}
}

func TestWriteCheckSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "check-summary.json")
	result := CheckResult{
		GeneratedAt: time.Unix(1710000000, 0).UTC(),
		Profile:     "production-edge",
		Passed:      true,
		Probe: &CheckProbeResult{
			Profile: "production-edge",
			Passed:  true,
			Result: &probe.Result{
				ID:     "probe-1",
				Target: "https://example.com",
				Status: 200,
				Proto:  "HTTP/2.0",
			},
		},
	}
	if err := writeCheckSummary(path, result); err != nil {
		t.Fatalf("writeCheckSummary returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload["kind"] != "check_summary" || payload["passed"] != true {
		t.Fatalf("unexpected summary payload: %v", payload)
	}
}

func TestWriteCheckJUnitReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "check-junit.xml")
	result := CheckResult{
		GeneratedAt: time.Unix(1710000000, 0).UTC(),
		Passed:      false,
		Failures:    []string{"probe: status 503 > max 299"},
		Probe: &CheckProbeResult{
			Profile:    "production-edge",
			Passed:     false,
			Violations: []string{"status 503 > max 299"},
			Result: &probe.Result{
				ID:       "probe-1",
				Target:   "https://example.com",
				Status:   503,
				Proto:    "HTTP/2.0",
				Duration: 250 * time.Millisecond,
			},
		},
	}
	if err := writeCheckJUnitReport(path, result); err != nil {
		t.Fatalf("writeCheckJUnitReport returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "<testsuites") || !strings.Contains(text, "<failure") || !strings.Contains(text, "status 503 &gt; max 299") {
		t.Fatalf("unexpected junit payload: %s", text)
	}
}
