package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/probe"
	"gopkg.in/yaml.v3"
)

func renderOutput(w io.Writer, format string, v any) error {
	if err := validateFormat(format); err != nil {
		return err
	}
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case "yaml":
		data, err := yaml.Marshal(v)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	case "markdown":
		_, err := io.WriteString(w, renderMarkdown(v))
		return err
	default:
		_, err := io.WriteString(w, renderTable(v))
		return err
	}
}

func renderTable(v any) string {
	switch value := v.(type) {
	case *probe.Result:
		return renderProbeTable(*value)
	case probe.Result:
		return renderProbeTable(value)
	case *bench.Result:
		return renderBenchTable(*value)
	case bench.Result:
		return renderBenchTable(value)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v\n", v)
		}
		return strings.TrimSpace(string(data)) + "\n"
	}
}

func renderMarkdown(v any) string {
	switch value := v.(type) {
	case *probe.Result:
		return renderProbeMarkdown(*value)
	case probe.Result:
		return renderProbeMarkdown(value)
	case *bench.Result:
		return renderBenchMarkdown(*value)
	case bench.Result:
		return renderBenchMarkdown(value)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "```text\n<unrenderable>\n```\n"
		}
		return "```json\n" + strings.TrimSpace(string(data)) + "\n```\n"
	}
}

func renderProbeTable(result probe.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Probe Result\n")
	fmt.Fprintf(&b, "Target: %s\n", result.Target)
	fmt.Fprintf(&b, "Status: %d\n", result.Status)
	fmt.Fprintf(&b, "Proto:  %s\n", result.Proto)
	fmt.Fprintf(&b, "Total:  %s\n", result.Duration)
	if len(result.Timings) > 0 {
		b.WriteString("\nTimings (ms)\n")
		for _, key := range sortedInt64Keys(result.Timings) {
			fmt.Fprintf(&b, "  %-10s %d\n", key, result.Timings[key])
		}
	}
	if len(result.TLS) > 0 {
		b.WriteString("\nTLS\n")
		for _, key := range sortedAnyKeys(result.TLS) {
			fmt.Fprintf(&b, "  %-10s %v\n", key, result.TLS[key])
		}
	}
	return b.String()
}

func renderBenchTable(result bench.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Bench Result\n")
	fmt.Fprintf(&b, "Target:      %s\n", result.Target)
	fmt.Fprintf(&b, "Duration:    %s\n", result.Duration)
	fmt.Fprintf(&b, "Concurrency: %d\n", result.Concurrency)
	fmt.Fprintf(&b, "Protocols:   %s\n", strings.Join(result.Protocols, ", "))
	b.WriteString("\nProtocol  Requests  Errors  Avg(ms)  Req/s   Bytes\n")
	for _, protocol := range sortedBenchKeys(result.Stats) {
		stats := result.Stats[protocol]
		fmt.Fprintf(&b, "%-8s  %-8d  %-6d  %-7.2f  %-6.2f  %d\n",
			protocol, stats.Requests, stats.Errors, stats.AverageMS, stats.RequestsPerS, stats.Transferred)
	}
	return b.String()
}

func renderProbeMarkdown(result probe.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Probe Result\n\n")
	fmt.Fprintf(&b, "- Target: `%s`\n", result.Target)
	fmt.Fprintf(&b, "- Status: `%d`\n", result.Status)
	fmt.Fprintf(&b, "- Proto: `%s`\n", result.Proto)
	fmt.Fprintf(&b, "- Duration: `%s`\n\n", result.Duration)
	if len(result.Timings) > 0 {
		b.WriteString("## Timings\n\n| Metric | ms |\n|---|---:|\n")
		for _, key := range sortedInt64Keys(result.Timings) {
			fmt.Fprintf(&b, "| %s | %d |\n", key, result.Timings[key])
		}
		b.WriteString("\n")
	}
	if len(result.TLS) > 0 {
		b.WriteString("## TLS\n\n| Field | Value |\n|---|---|\n")
		for _, key := range sortedAnyKeys(result.TLS) {
			fmt.Fprintf(&b, "| %s | %v |\n", key, result.TLS[key])
		}
	}
	return b.String()
}

func renderBenchMarkdown(result bench.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bench Result\n\n")
	fmt.Fprintf(&b, "- Target: `%s`\n", result.Target)
	fmt.Fprintf(&b, "- Duration: `%s`\n", result.Duration)
	fmt.Fprintf(&b, "- Concurrency: `%d`\n\n", result.Concurrency)
	b.WriteString("| Protocol | Requests | Errors | Avg ms | Req/s | Bytes |\n|---|---:|---:|---:|---:|---:|\n")
	for _, protocol := range sortedBenchKeys(result.Stats) {
		stats := result.Stats[protocol]
		fmt.Fprintf(&b, "| %s | %d | %d | %.2f | %.2f | %d |\n",
			protocol, stats.Requests, stats.Errors, stats.AverageMS, stats.RequestsPerS, stats.Transferred)
	}
	return b.String()
}

func sortedInt64Keys(input map[string]int64) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAnyKeys(input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBenchKeys(input map[string]bench.Stats) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
