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
	if len(result.Analysis) > 0 {
		renderProbeAnalysisTable(&b, result.Analysis)
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
	if len(result.Summary) > 0 {
		fmt.Fprintf(&b, "Summary:     healthy=%v degraded=%v failed=%v best=%v risk=%v\n",
			result.Summary["healthy_protocols"],
			result.Summary["degraded_protocols"],
			result.Summary["failed_protocols"],
			result.Summary["best_protocol"],
			result.Summary["riskiest_protocol"])
	}
	b.WriteString("\nProtocol  Requests  Errors  Avg(ms)  P95(ms)  Req/s   Bytes\n")
	for _, protocol := range sortedBenchKeys(result.Stats) {
		stats := result.Stats[protocol]
		fmt.Fprintf(&b, "%-8s  %-8d  %-6d  %-7.2f  %-7.2f  %-6.2f  %d\n",
			protocol, stats.Requests, stats.Errors, stats.AverageMS, stats.Latency.P95, stats.RequestsPerS, stats.Transferred)
		if stats.SampledPoints > 0 {
			fmt.Fprintf(&b, "          p50 %.2f  p99 %.2f  error-rate %.2f%%  samples %d\n",
				stats.Latency.P50, stats.Latency.P99, stats.ErrorRate*100, stats.SampledPoints)
		}
		if stats.Phases.FirstByteMS > 0 || stats.Phases.TransferMS > 0 || stats.Phases.ConnectMS > 0 || stats.Phases.TLSMS > 0 {
			fmt.Fprintf(&b, "          phases connect %.2f  tls %.2f  first-byte %.2f  transfer %.2f\n",
				stats.Phases.ConnectMS, stats.Phases.TLSMS, stats.Phases.FirstByteMS, stats.Phases.TransferMS)
		}
		if len(stats.ErrorSummary) > 0 {
			fmt.Fprintf(&b, "          errors %s\n", formatErrorSummary(stats.ErrorSummary))
		}
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
		b.WriteString("\n")
	}
	if len(result.Analysis) > 0 {
		renderProbeAnalysisMarkdown(&b, result.Analysis)
	}
	return b.String()
}

func renderProbeAnalysisTable(b *strings.Builder, analysis map[string]any) {
	if b == nil || len(analysis) == 0 {
		return
	}
	b.WriteString("\nAnalysis\n")
	if response, ok := analysis["response"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s body=%v status=%v throughput=%v B/s\n",
			"response",
			response["body_bytes"],
			response["status_class"],
			response["throughput_bytes_sec"])
	}
	if latency, ok := analysis["latency"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s samples=%v avg=%vms p50=%v p95=%v p99=%v\n",
			"latency",
			latency["samples"],
			latency["avg_ms"],
			latency["p50"],
			latency["p95"],
			latency["p99"])
	}
	if streams, ok := analysis["streams"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s attempted=%v ok=%v err=%v p95=%vms success=%v\n",
			"streams",
			streams["attempted"],
			streams["successful"],
			streams["errors"],
			streams["p95_latency_ms"],
			streams["success_rate"])
	}
	if altSvc, ok := analysis["alt_svc"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s present=%v values=%v\n", "alt-svc", altSvc["present"], altSvc["values"])
	}
	if qpack, ok := analysis["qpack"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s headers=%v raw=%v block=%v ratio=%v\n",
			"qpack",
			qpack["header_count"],
			qpack["raw_bytes"],
			qpack["estimated_block"],
			qpack["estimated_ratio"])
	}
	if loss, ok := analysis["loss"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s signal=%v errors=%v success=%v\n",
			"loss",
			loss["signal"],
			loss["stream_errors"],
			loss["success_rate"])
	}
	if congestion, ok := analysis["congestion"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s signal=%v p50=%v p95=%v ratio=%v\n",
			"congestion",
			congestion["signal"],
			congestion["p50_ms"],
			congestion["p95_ms"],
			congestion["spread_ratio"])
	}
	if version, ok := analysis["version"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s proto=%v alpn=%v quic=%v\n",
			"version",
			version["observed_proto"],
			version["alpn"],
			version["quic_version"])
	}
	if retry, ok := analysis["retry"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s observed=%v connect=%vms tls=%vms\n",
			"retry",
			retry["retry_observed"],
			retry["connect_ms"],
			retry["tls_ms"])
	}
	if ecn, ok := analysis["ecn"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s visible=%v proto=%v marks=%v\n",
			"ecn",
			ecn["ecn_visible"],
			ecn["observed_proto"],
			ecn["packet_marks"])
	}
	if spin, ok := analysis["spin-bit"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s observed=%v rtt=%vms stability=%v\n",
			"spin-bit",
			spin["spin_observed"],
			spin["rtt_estimate_ms"],
			spin["stability"])
	}
	if plan, ok := analysis["test_plan"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s requested=%s executed=%s\n",
			"test-plan",
			joinStringSlice(anyToStringSlice(plan["requested"])),
			joinStringSlice(anyToStringSlice(plan["executed"])))
		if skipped := anyToSkippedSummary(plan["skipped"]); skipped != "" {
			fmt.Fprintf(b, "  %-10s skipped=%s\n", "", skipped)
		}
	}
	if support, ok := analysis["support"].(map[string]any); ok {
		for _, name := range sortedAnyKeys(support) {
			entry, ok := support[name].(map[string]any)
			if !ok {
				continue
			}
			fmt.Fprintf(b, "  %-10s %s coverage=%v state=%v summary=%v\n",
				"support",
				name,
				entry["coverage"],
				entry["state"],
				entry["summary"])
		}
	}
	if summary, ok := analysis["support_summary"].(map[string]any); ok {
		fmt.Fprintf(b, "  %-10s requested=%v available=%v not-run=%v unavailable=%v ratio=%v\n",
			"coverage",
			summary["requested_tests"],
			summary["available"],
			summary["not_run"],
			summary["unavailable"],
			summary["coverage_ratio"])
	}
	for _, key := range sortedAnyKeys(analysis) {
		if key == "response" || key == "latency" || key == "streams" || key == "alt_svc" || key == "qpack" || key == "loss" || key == "congestion" || key == "version" || key == "retry" || key == "ecn" || key == "spin-bit" || key == "test_plan" || key == "support" || key == "support_summary" {
			continue
		}
		fmt.Fprintf(b, "  %-10s %v\n", key, analysis[key])
	}
}

func renderProbeAnalysisMarkdown(b *strings.Builder, analysis map[string]any) {
	if b == nil || len(analysis) == 0 {
		return
	}
	b.WriteString("## Analysis\n\n")
	if response, ok := analysis["response"].(map[string]any); ok {
		fmt.Fprintf(b, "- Response: body `%v` bytes, status class `%v`, throughput `%v` B/s\n",
			response["body_bytes"], response["status_class"], response["throughput_bytes_sec"])
	}
	if latency, ok := analysis["latency"].(map[string]any); ok {
		fmt.Fprintf(b, "- Latency: samples `%v`, avg `%vms`, p50 `%v`, p95 `%v`, p99 `%v`\n",
			latency["samples"], latency["avg_ms"], latency["p50"], latency["p95"], latency["p99"])
	}
	if streams, ok := analysis["streams"].(map[string]any); ok {
		fmt.Fprintf(b, "- Streams: attempted `%v`, successful `%v`, errors `%v`, p95 `%vms`, success `%v`\n",
			streams["attempted"], streams["successful"], streams["errors"], streams["p95_latency_ms"], streams["success_rate"])
	}
	if altSvc, ok := analysis["alt_svc"].(map[string]any); ok {
		fmt.Fprintf(b, "- Alt-Svc: present `%v`, values `%v`\n", altSvc["present"], altSvc["values"])
	}
	if qpack, ok := analysis["qpack"].(map[string]any); ok {
		fmt.Fprintf(b, "- QPACK: headers `%v`, raw `%v`, block `%v`, ratio `%v`\n",
			qpack["header_count"], qpack["raw_bytes"], qpack["estimated_block"], qpack["estimated_ratio"])
	}
	if loss, ok := analysis["loss"].(map[string]any); ok {
		fmt.Fprintf(b, "- Loss: signal `%v`, stream errors `%v`, success `%v`\n",
			loss["signal"], loss["stream_errors"], loss["success_rate"])
	}
	if congestion, ok := analysis["congestion"].(map[string]any); ok {
		fmt.Fprintf(b, "- Congestion: signal `%v`, p50 `%v`, p95 `%v`, spread `%v`\n",
			congestion["signal"], congestion["p50_ms"], congestion["p95_ms"], congestion["spread_ratio"])
	}
	if version, ok := analysis["version"].(map[string]any); ok {
		fmt.Fprintf(b, "- Version: proto `%v`, alpn `%v`, quic `%v`\n",
			version["observed_proto"], version["alpn"], version["quic_version"])
	}
	if retry, ok := analysis["retry"].(map[string]any); ok {
		fmt.Fprintf(b, "- Retry: observed `%v`, connect `%vms`, tls `%vms`\n",
			retry["retry_observed"], retry["connect_ms"], retry["tls_ms"])
	}
	if ecn, ok := analysis["ecn"].(map[string]any); ok {
		fmt.Fprintf(b, "- ECN: visible `%v`, proto `%v`, marks `%v`\n",
			ecn["ecn_visible"], ecn["observed_proto"], ecn["packet_marks"])
	}
	if spin, ok := analysis["spin-bit"].(map[string]any); ok {
		fmt.Fprintf(b, "- Spin Bit: observed `%v`, rtt `%vms`, stability `%v`\n",
			spin["spin_observed"], spin["rtt_estimate_ms"], spin["stability"])
	}
	if plan, ok := analysis["test_plan"].(map[string]any); ok {
		fmt.Fprintf(b, "- Test Plan: requested `%s`, executed `%s`\n",
			joinStringSlice(anyToStringSlice(plan["requested"])),
			joinStringSlice(anyToStringSlice(plan["executed"])))
		if skipped := anyToSkippedSummary(plan["skipped"]); skipped != "" {
			fmt.Fprintf(b, "- Skipped: `%s`\n", skipped)
		}
	}
	if support, ok := analysis["support"].(map[string]any); ok {
		for _, name := range sortedAnyKeys(support) {
			entry, ok := support[name].(map[string]any)
			if !ok {
				continue
			}
			fmt.Fprintf(b, "- Support `%s`: coverage `%v`, state `%v`, summary `%v`\n",
				name,
				entry["coverage"],
				entry["state"],
				entry["summary"])
		}
	}
	if summary, ok := analysis["support_summary"].(map[string]any); ok {
		fmt.Fprintf(b, "- Coverage Summary: requested `%v`, available `%v`, not-run `%v`, unavailable `%v`, ratio `%v`\n",
			summary["requested_tests"],
			summary["available"],
			summary["not_run"],
			summary["unavailable"],
			summary["coverage_ratio"])
	}
	for _, key := range sortedAnyKeys(analysis) {
		if key == "response" || key == "latency" || key == "streams" || key == "alt_svc" || key == "qpack" || key == "loss" || key == "congestion" || key == "version" || key == "retry" || key == "ecn" || key == "spin-bit" || key == "test_plan" || key == "support" || key == "support_summary" {
			continue
		}
		fmt.Fprintf(b, "- %s: `%v`\n", key, analysis[key])
	}
}

func anyToStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func anyToSkippedSummary(value any) string {
	switch typed := value.(type) {
	case []map[string]any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fmt.Sprintf("%v (%v)", item["name"], item["reason"]))
		}
		return strings.Join(parts, ", ")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				parts = append(parts, fmt.Sprintf("%v (%v)", m["name"], m["reason"]))
			}
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}

func joinStringSlice(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func renderBenchMarkdown(result bench.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bench Result\n\n")
	fmt.Fprintf(&b, "- Target: `%s`\n", result.Target)
	fmt.Fprintf(&b, "- Duration: `%s`\n", result.Duration)
	fmt.Fprintf(&b, "- Concurrency: `%d`\n\n", result.Concurrency)
	if len(result.Summary) > 0 {
		fmt.Fprintf(&b, "- Summary: healthy `%v`, degraded `%v`, failed `%v`, best `%v`, risk `%v`\n\n",
			result.Summary["healthy_protocols"],
			result.Summary["degraded_protocols"],
			result.Summary["failed_protocols"],
			result.Summary["best_protocol"],
			result.Summary["riskiest_protocol"])
	}
	b.WriteString("| Protocol | Requests | Errors | Avg ms | P50 | P95 | P99 | Req/s | Bytes |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, protocol := range sortedBenchKeys(result.Stats) {
		stats := result.Stats[protocol]
		fmt.Fprintf(&b, "| %s | %d | %d | %.2f | %.2f | %.2f | %.2f | %.2f | %d |\n",
			protocol, stats.Requests, stats.Errors, stats.AverageMS, stats.Latency.P50, stats.Latency.P95, stats.Latency.P99, stats.RequestsPerS, stats.Transferred)
		if stats.Phases.FirstByteMS > 0 || stats.Phases.TransferMS > 0 || stats.Phases.ConnectMS > 0 || stats.Phases.TLSMS > 0 {
			fmt.Fprintf(&b, "\nPhase averages for `%s`: connect `%.2fms`, tls `%.2fms`, first-byte `%.2fms`, transfer `%.2fms`.\n",
				protocol, stats.Phases.ConnectMS, stats.Phases.TLSMS, stats.Phases.FirstByteMS, stats.Phases.TransferMS)
		}
		if len(stats.ErrorSummary) > 0 {
			fmt.Fprintf(&b, "\nError summary for `%s`: `%s`.\n", protocol, formatErrorSummary(stats.ErrorSummary))
		}
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

func formatErrorSummary(input map[string]int64) string {
	if len(input) == 0 {
		return ""
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, input[key]))
	}
	return strings.Join(parts, ", ")
}
