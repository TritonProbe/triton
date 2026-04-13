package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/probe"
)

func TestRenderProbeTable(t *testing.T) {
	out := renderTable(probe.Result{
		Target:   "https://example.com",
		Status:   200,
		Proto:    "HTTP/2.0",
		Duration: 150 * time.Millisecond,
		Timings: map[string]int64{
			"total": 150,
			"tls":   10,
		},
		Analysis: map[string]any{
			"latency": map[string]any{"p95": 12.5},
			"test_plan": probe.TestPlan{
				Requested: []string{"latency", "0rtt"},
				Executed:  []string{"latency"},
				Skipped:   []probe.SkippedTest{{Name: "0rtt", Reason: "not implemented"}},
			},
			"support": map[string]any{
				"0rtt": map[string]any{"coverage": "partial", "state": "unavailable", "summary": "resumption not available"},
			},
		},
	})
	if !strings.Contains(out, "Probe Result") || !strings.Contains(out, "https://example.com") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "Analysis") || !strings.Contains(out, "latency") || !strings.Contains(out, "test-plan") || !strings.Contains(out, "0rtt") || !strings.Contains(out, "coverage=partial") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRenderProbeMarkdown(t *testing.T) {
	out := renderMarkdown(probe.Result{
		Target:   "https://example.com",
		Status:   200,
		Proto:    "HTTP/2.0",
		Duration: 150 * time.Millisecond,
		Analysis: map[string]any{
			"response": map[string]any{"body_bytes": 5, "status_class": 2, "throughput_bytes_sec": 99},
			"qpack":    map[string]any{"header_count": 4, "raw_bytes": 120, "estimated_block": 84, "estimated_ratio": 0.7},
			"loss":     map[string]any{"signal": "elevated", "stream_errors": 1, "success_rate": 0.75},
			"congestion": map[string]any{"signal": "moderate", "p50_ms": 12, "p95_ms": 24, "spread_ratio": 1.0},
			"version": map[string]any{"observed_proto": "HTTP/3.0", "alpn": "h3", "quic_version": "not_exposed"},
			"retry":   map[string]any{"retry_observed": false, "connect_ms": 5, "tls_ms": 8},
			"ecn":     map[string]any{"ecn_visible": false, "observed_proto": "HTTP/3.0", "packet_marks": "not_exposed"},
			"spin-bit": map[string]any{"spin_observed": false, "rtt_estimate_ms": 11, "stability": "steady"},
			"test_plan": probe.TestPlan{
				Requested: []string{"latency", "migration", "qpack"},
				Executed:  []string{"latency"},
				Skipped: []probe.SkippedTest{
					{Name: "migration", Reason: "not implemented"},
					{Name: "qpack", Reason: "not implemented"},
				},
			},
			"support": map[string]any{
				"migration": map[string]any{"coverage": "partial", "state": "unavailable", "summary": "endpoint contract only"},
				"qpack":     map[string]any{"coverage": "unavailable", "state": "unavailable", "summary": "QPACK inspection is not implemented yet"},
			},
			"support_summary": map[string]any{"requested_tests": 2, "available": 0, "not_run": 0, "unavailable": 2, "coverage_ratio": 0.0},
		},
	})
	if !strings.Contains(out, "## Analysis") || !strings.Contains(out, "Test Plan") || !strings.Contains(out, "migration") || !strings.Contains(out, "Support `migration`") || !strings.Contains(out, "Support `qpack`") || !strings.Contains(out, "QPACK") || !strings.Contains(out, "Loss") || !strings.Contains(out, "Congestion") || !strings.Contains(out, "Version") || !strings.Contains(out, "Retry") || !strings.Contains(out, "ECN") || !strings.Contains(out, "Spin Bit") || !strings.Contains(out, "Coverage Summary") {
		t.Fatalf("unexpected markdown output: %q", out)
	}
}

func TestRenderBenchMarkdown(t *testing.T) {
	out := renderMarkdown(bench.Result{
		Target:      "https://example.com",
		Duration:    time.Second,
		Concurrency: 2,
		Summary: bench.Summary{
			Protocols:         1,
			HealthyProtocols:  1,
			DegradedProtocols: 0,
			FailedProtocols:   0,
			BestProtocol:      "h1",
			RiskiestProtocol:  "h1",
		},
		Stats: map[string]bench.Stats{
			"h1": {
				Requests:     10,
				Errors:       1,
				AverageMS:    5.5,
				RequestsPerS: 10,
				Transferred:  100,
				Latency:      bench.Percentiles{P50: 4.5, P95: 8.5, P99: 10.5},
				Phases:       bench.PhaseAverages{ConnectMS: 1, TLSMS: 2, FirstByteMS: 3, TransferMS: 4},
				ErrorSummary: map[string]int64{"timeout": 1},
			},
		},
	})
	if !strings.Contains(out, "| Protocol | Requests |") || !strings.Contains(out, "h1") {
		t.Fatalf("unexpected markdown output: %q", out)
	}
	if !strings.Contains(out, "P95") || !strings.Contains(out, "Error summary") {
		t.Fatalf("unexpected markdown output: %q", out)
	}
	if !strings.Contains(out, "Summary: healthy") {
		t.Fatalf("expected bench summary in markdown output: %q", out)
	}
}

func TestRenderBenchTable(t *testing.T) {
	out := renderTable(bench.Result{
		Target:      "https://example.com",
		Duration:    time.Second,
		Concurrency: 2,
		Summary: bench.Summary{
			Protocols:         1,
			HealthyProtocols:  1,
			DegradedProtocols: 0,
			FailedProtocols:   0,
			BestProtocol:      "h1",
			RiskiestProtocol:  "h1",
		},
		Stats: map[string]bench.Stats{
			"h1": {
				Requests:      10,
				Errors:        1,
				AverageMS:     5.5,
				RequestsPerS:  10,
				Transferred:   100,
				ErrorRate:     0.1,
				Latency:       bench.Percentiles{P50: 4.5, P95: 8.5, P99: 10.5},
				Phases:        bench.PhaseAverages{ConnectMS: 1, TLSMS: 2, FirstByteMS: 3, TransferMS: 4},
				ErrorSummary:  map[string]int64{"timeout": 1},
				SampledPoints: 10,
			},
		},
	})
	if !strings.Contains(out, "P95(ms)") || !strings.Contains(out, "first-byte") || !strings.Contains(out, "timeout=1") || !strings.Contains(out, "Summary:") {
		t.Fatalf("unexpected table output: %q", out)
	}
}
