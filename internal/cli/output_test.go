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
	})
	if !strings.Contains(out, "Probe Result") || !strings.Contains(out, "https://example.com") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRenderBenchMarkdown(t *testing.T) {
	out := renderMarkdown(bench.Result{
		Target:      "https://example.com",
		Duration:    time.Second,
		Concurrency: 2,
		Stats: map[string]bench.Stats{
			"h1": {Requests: 10, Errors: 1, AverageMS: 5.5, RequestsPerS: 10, Transferred: 100},
		},
	})
	if !strings.Contains(out, "| Protocol | Requests |") || !strings.Contains(out, "h1") {
		t.Fatalf("unexpected markdown output: %q", out)
	}
}
