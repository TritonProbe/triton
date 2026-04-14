package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	if tlsFields := anyToTLSFields(result.TLS); len(tlsFields) > 0 {
		b.WriteString("\nTLS\n")
		for _, key := range sortedAnyKeys(tlsFields) {
			fmt.Fprintf(&b, "  %-10s %v\n", key, tlsFields[key])
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
	if result.Summary.Protocols > 0 {
		fmt.Fprintf(&b, "Summary:     healthy=%v degraded=%v failed=%v best=%v risk=%v\n",
			result.Summary.HealthyProtocols,
			result.Summary.DegradedProtocols,
			result.Summary.FailedProtocols,
			result.Summary.BestProtocol,
			result.Summary.RiskiestProtocol)
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
	if tlsFields := anyToTLSFields(result.TLS); len(tlsFields) > 0 {
		b.WriteString("## TLS\n\n| Field | Value |\n|---|---|\n")
		for _, key := range sortedAnyKeys(tlsFields) {
			fmt.Fprintf(&b, "| %s | %v |\n", key, tlsFields[key])
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
	if response, ok := anyToResponseAnalysis(analysis["response"]); ok {
		fmt.Fprintf(b, "  %-10s body=%v status=%v throughput=%v B/s\n",
			"response",
			response.BodyBytes,
			response.StatusClass,
			response.ThroughputBytesSec)
	}
	if latency, ok := anyToLatencyAnalysis(analysis["latency"]); ok {
		fmt.Fprintf(b, "  %-10s samples=%v avg=%vms p50=%v p95=%v p99=%v\n",
			"latency",
			latency.Samples,
			latency.AverageMS,
			latency.P50,
			latency.P95,
			latency.P99)
	}
	if streams, ok := anyToStreamAnalysis(analysis["streams"]); ok {
		fmt.Fprintf(b, "  %-10s attempted=%v ok=%v err=%v p95=%vms success=%v\n",
			"streams",
			streams.Attempted,
			streams.Successful,
			streams.Errors,
			streams.P95Latency,
			streams.SuccessRate)
	}
	if altSvc, ok := anyToAltSvcAnalysis(analysis["alt_svc"]); ok {
		fmt.Fprintf(b, "  %-10s present=%v values=%v\n", "alt-svc", altSvc["present"], altSvc["values"])
	}
	if qpack, ok := anyToQPACKAnalysis(analysis["qpack"]); ok {
		fmt.Fprintf(b, "  %-10s headers=%v raw=%v block=%v ratio=%v\n",
			"qpack",
			qpack.HeaderCount,
			qpack.RawBytes,
			qpack.EstimatedBlock,
			qpack.EstimatedRatio)
	}
	if loss, ok := anyToLossAnalysis(analysis["loss"]); ok {
		fmt.Fprintf(b, "  %-10s signal=%v errors=%v success=%v\n",
			"loss",
			loss.Signal,
			loss.StreamErrors,
			loss.SuccessRate)
	}
	if congestion, ok := anyToCongestionAnalysis(analysis["congestion"]); ok {
		fmt.Fprintf(b, "  %-10s signal=%v p50=%v p95=%v ratio=%v\n",
			"congestion",
			congestion.Signal,
			congestion.P50MS,
			congestion.P95MS,
			congestion.SpreadRatio)
	}
	if version, ok := anyToVersionAnalysis(analysis["version"]); ok {
		fmt.Fprintf(b, "  %-10s proto=%v alpn=%v quic=%v\n",
			"version",
			version.ObservedProto,
			version.ALPN,
			version.QUICVersion)
	}
	if retry, ok := anyToRetryAnalysis(analysis["retry"]); ok {
		fmt.Fprintf(b, "  %-10s observed=%v connect=%vms tls=%vms\n",
			"retry",
			retry.RetryObserved,
			retry.ConnectMS,
			retry.TLSMS)
	}
	if ecn, ok := anyToECNAnalysis(analysis["ecn"]); ok {
		fmt.Fprintf(b, "  %-10s visible=%v proto=%v marks=%v\n",
			"ecn",
			ecn.ECNVisible,
			ecn.ObservedProto,
			ecn.PacketMarks)
	}
	if spin, ok := anyToSpinBitAnalysis(analysis["spin-bit"]); ok {
		fmt.Fprintf(b, "  %-10s observed=%v rtt=%vms stability=%v\n",
			"spin-bit",
			spin.SpinObserved,
			spin.RTTEstimateMS,
			spin.Stability)
	}
	if zeroRTT, ok := anyToZeroRTTAnalysis(analysis["0rtt"]); ok {
		fmt.Fprintf(b, "  %-10s mode=%v resumed=%v saved=%vms\n",
			"0rtt",
			zeroRTT.Mode,
			zeroRTT.Resumed,
			zeroRTT.TimeSavedMS)
	}
	if migration, ok := anyToMigrationAnalysis(analysis["migration"]); ok {
		fmt.Fprintf(b, "  %-10s mode=%v supported=%v status=%v\n",
			"migration",
			migration.Mode,
			migration.Supported,
			migration.StatusClass)
	}
	if plan, ok := anyToTestPlan(analysis["test_plan"]); ok {
		fmt.Fprintf(b, "  %-10s requested=%s executed=%s\n",
			"test-plan",
			joinStringSlice(plan.Requested),
			joinStringSlice(plan.Executed))
		if skipped := formatSkippedTests(plan.Skipped); skipped != "" {
			fmt.Fprintf(b, "  %-10s skipped=%s\n", "", skipped)
		}
	}
	if support := anyToSupportEntries(analysis["support"]); len(support) > 0 {
		for _, name := range sortedSupportKeys(support) {
			entry := support[name]
			fmt.Fprintf(b, "  %-10s %s coverage=%v state=%v summary=%v\n",
				"support",
				name,
				entry.Coverage,
				entry.State,
				entry.Summary)
		}
	}
	if summary, ok := anyToSupportSummary(analysis["support_summary"]); ok {
		fmt.Fprintf(b, "  %-10s requested=%v available=%v not-run=%v unavailable=%v ratio=%v\n",
			"coverage",
			summary.RequestedTests,
			summary.Available,
			summary.NotRun,
			summary.Unavailable,
			summary.CoverageRatio)
	}
	for _, key := range sortedAnyKeys(analysis) {
		if key == "response" || key == "latency" || key == "streams" || key == "alt_svc" || key == "qpack" || key == "loss" || key == "congestion" || key == "version" || key == "retry" || key == "ecn" || key == "spin-bit" || key == "0rtt" || key == "migration" || key == "test_plan" || key == "support" || key == "support_summary" {
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
	if response, ok := anyToResponseAnalysis(analysis["response"]); ok {
		fmt.Fprintf(b, "- Response: body `%v` bytes, status class `%v`, throughput `%v` B/s\n",
			response.BodyBytes, response.StatusClass, response.ThroughputBytesSec)
	}
	if latency, ok := anyToLatencyAnalysis(analysis["latency"]); ok {
		fmt.Fprintf(b, "- Latency: samples `%v`, avg `%vms`, p50 `%v`, p95 `%v`, p99 `%v`\n",
			latency.Samples, latency.AverageMS, latency.P50, latency.P95, latency.P99)
	}
	if streams, ok := anyToStreamAnalysis(analysis["streams"]); ok {
		fmt.Fprintf(b, "- Streams: attempted `%v`, successful `%v`, errors `%v`, p95 `%vms`, success `%v`\n",
			streams.Attempted, streams.Successful, streams.Errors, streams.P95Latency, streams.SuccessRate)
	}
	if altSvc, ok := anyToAltSvcAnalysis(analysis["alt_svc"]); ok {
		fmt.Fprintf(b, "- Alt-Svc: present `%v`, values `%v`\n", altSvc["present"], altSvc["values"])
	}
	if qpack, ok := anyToQPACKAnalysis(analysis["qpack"]); ok {
		fmt.Fprintf(b, "- QPACK: headers `%v`, raw `%v`, block `%v`, ratio `%v`\n",
			qpack.HeaderCount, qpack.RawBytes, qpack.EstimatedBlock, qpack.EstimatedRatio)
	}
	if loss, ok := anyToLossAnalysis(analysis["loss"]); ok {
		fmt.Fprintf(b, "- Loss: signal `%v`, stream errors `%v`, success `%v`\n",
			loss.Signal, loss.StreamErrors, loss.SuccessRate)
	}
	if congestion, ok := anyToCongestionAnalysis(analysis["congestion"]); ok {
		fmt.Fprintf(b, "- Congestion: signal `%v`, p50 `%v`, p95 `%v`, spread `%v`\n",
			congestion.Signal, congestion.P50MS, congestion.P95MS, congestion.SpreadRatio)
	}
	if version, ok := anyToVersionAnalysis(analysis["version"]); ok {
		fmt.Fprintf(b, "- Version: proto `%v`, alpn `%v`, quic `%v`\n",
			version.ObservedProto, version.ALPN, version.QUICVersion)
	}
	if retry, ok := anyToRetryAnalysis(analysis["retry"]); ok {
		fmt.Fprintf(b, "- Retry: observed `%v`, connect `%vms`, tls `%vms`\n",
			retry.RetryObserved, retry.ConnectMS, retry.TLSMS)
	}
	if ecn, ok := anyToECNAnalysis(analysis["ecn"]); ok {
		fmt.Fprintf(b, "- ECN: visible `%v`, proto `%v`, marks `%v`\n",
			ecn.ECNVisible, ecn.ObservedProto, ecn.PacketMarks)
	}
	if spin, ok := anyToSpinBitAnalysis(analysis["spin-bit"]); ok {
		fmt.Fprintf(b, "- Spin Bit: observed `%v`, rtt `%vms`, stability `%v`\n",
			spin.SpinObserved, spin.RTTEstimateMS, spin.Stability)
	}
	if zeroRTT, ok := anyToZeroRTTAnalysis(analysis["0rtt"]); ok {
		fmt.Fprintf(b, "- 0-RTT: mode `%v`, resumed `%v`, saved `%vms`\n",
			zeroRTT.Mode, zeroRTT.Resumed, zeroRTT.TimeSavedMS)
	}
	if migration, ok := anyToMigrationAnalysis(analysis["migration"]); ok {
		fmt.Fprintf(b, "- Migration: mode `%v`, supported `%v`, status `%v`\n",
			migration.Mode, migration.Supported, migration.StatusClass)
	}
	if plan, ok := anyToTestPlan(analysis["test_plan"]); ok {
		fmt.Fprintf(b, "- Test Plan: requested `%s`, executed `%s`\n",
			joinStringSlice(plan.Requested),
			joinStringSlice(plan.Executed))
		if skipped := formatSkippedTests(plan.Skipped); skipped != "" {
			fmt.Fprintf(b, "- Skipped: `%s`\n", skipped)
		}
	}
	if support := anyToSupportEntries(analysis["support"]); len(support) > 0 {
		for _, name := range sortedSupportKeys(support) {
			entry := support[name]
			fmt.Fprintf(b, "- Support `%s`: coverage `%v`, state `%v`, summary `%v`\n",
				name,
				entry.Coverage,
				entry.State,
				entry.Summary)
		}
	}
	if summary, ok := anyToSupportSummary(analysis["support_summary"]); ok {
		fmt.Fprintf(b, "- Coverage Summary: requested `%v`, available `%v`, not-run `%v`, unavailable `%v`, ratio `%v`\n",
			summary.RequestedTests,
			summary.Available,
			summary.NotRun,
			summary.Unavailable,
			summary.CoverageRatio)
	}
	for _, key := range sortedAnyKeys(analysis) {
		if key == "response" || key == "latency" || key == "streams" || key == "alt_svc" || key == "qpack" || key == "loss" || key == "congestion" || key == "version" || key == "retry" || key == "ecn" || key == "spin-bit" || key == "0rtt" || key == "migration" || key == "test_plan" || key == "support" || key == "support_summary" {
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

func anyToTestPlan(value any) (probe.TestPlan, bool) {
	switch typed := value.(type) {
	case probe.TestPlan:
		return probe.TestPlan{
			Requested: append([]string(nil), typed.Requested...),
			Executed:  append([]string(nil), typed.Executed...),
			Skipped:   append([]probe.SkippedTest(nil), typed.Skipped...),
		}, true
	case map[string]any:
		return probe.TestPlan{
			Requested: anyToStringSlice(typed["requested"]),
			Executed:  anyToStringSlice(typed["executed"]),
			Skipped:   anyToSkippedTests(typed["skipped"]),
		}, true
	default:
		return probe.TestPlan{}, false
	}
}

func anyToResponseAnalysis(value any) (probe.ResponseAnalysis, bool) {
	switch typed := value.(type) {
	case probe.ResponseAnalysis:
		return typed, true
	case map[string]any:
		return probe.ResponseAnalysis{
			BodyBytes:          int64Value(typed["body_bytes"]),
			ThroughputBytesSec: floatValue(typed["throughput_bytes_sec"]),
			ThroughputBitsSec:  floatValue(typed["throughput_bits_sec"]),
			StatusClass:        intValue(typed["status_class"]),
		}, true
	default:
		return probe.ResponseAnalysis{}, false
	}
}

func anyToTLSFields(value any) map[string]any {
	switch typed := value.(type) {
	case probe.TLSMetadata:
		out := map[string]any{}
		if typed.Mode != "" {
			out["mode"] = typed.Mode
		}
		if typed.Version != "" {
			out["version"] = typed.Version
		}
		if typed.Cipher != "" {
			out["cipher"] = typed.Cipher
		}
		if typed.ALPN != "" {
			out["alpn"] = typed.ALPN
		}
		if typed.ServerName != "" {
			out["server_name"] = typed.ServerName
		}
		if typed.PeerCerts > 0 {
			out["peer_certs"] = typed.PeerCerts
		}
		if typed.HandshakeState != "" {
			out["handshake_state"] = typed.HandshakeState
		}
		if typed.VerifiedChains > 0 {
			out["verified_chains"] = typed.VerifiedChains
		}
		if typed.Resumed || typed.HandshakeState != "" || typed.Mode != "" {
			out["resumed"] = typed.Resumed
		}
		if typed.LeafCert != nil {
			out["leaf_cert"] = typed.LeafCert
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return nil
	}
}

func anyToAltSvcAnalysis(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case probe.AltSvcAnalysis:
		return map[string]any{
			"present": typed.Present,
			"values":  append([]string(nil), typed.Values...),
		}, true
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func anyToQPACKAnalysis(value any) (probe.QPACKAnalysis, bool) {
	switch typed := value.(type) {
	case probe.QPACKAnalysis:
		return typed, true
	case map[string]any:
		return probe.QPACKAnalysis{
			Supported:         boolValue(typed["supported"]),
			Mode:              stringValue(typed["mode"]),
			HeaderCount:       intValue(typed["header_count"]),
			RawBytes:          intValue(typed["raw_bytes"]),
			EstimatedBlock:    intValue(typed["estimated_block"]),
			EstimatedRatio:    floatValue(typed["estimated_ratio"]),
			CompressionSaving: intValue(typed["compression_saving"]),
			Note:              stringValue(typed["note"]),
		}, true
	default:
		return probe.QPACKAnalysis{}, false
	}
}

func anyToVersionAnalysis(value any) (probe.VersionAnalysis, bool) {
	switch typed := value.(type) {
	case probe.VersionAnalysis:
		return typed, true
	case map[string]any:
		return probe.VersionAnalysis{
			Supported:       boolValue(typed["supported"]),
			Mode:            stringValue(typed["mode"]),
			ObservedProto:   stringValue(typed["observed_proto"]),
			ALPN:            stringValue(typed["alpn"]),
			QUICVersion:     stringValue(typed["quic_version"]),
			NegotiationSeen: boolValue(typed["negotiation_seen"]),
			Note:            stringValue(typed["note"]),
		}, true
	default:
		return probe.VersionAnalysis{}, false
	}
}

func anyToRetryAnalysis(value any) (probe.RetryAnalysis, bool) {
	switch typed := value.(type) {
	case probe.RetryAnalysis:
		return typed, true
	case map[string]any:
		return probe.RetryAnalysis{
			Supported:     boolValue(typed["supported"]),
			Mode:          stringValue(typed["mode"]),
			ObservedProto: stringValue(typed["observed_proto"]),
			ALPN:          stringValue(typed["alpn"]),
			RetryObserved: boolValue(typed["retry_observed"]),
			ConnectMS:     int64Value(typed["connect_ms"]),
			TLSMS:         int64Value(typed["tls_ms"]),
			Visibility:    stringValue(typed["visibility"]),
			Note:          stringValue(typed["note"]),
		}, true
	default:
		return probe.RetryAnalysis{}, false
	}
}

func anyToECNAnalysis(value any) (probe.ECNAnalysis, bool) {
	switch typed := value.(type) {
	case probe.ECNAnalysis:
		return typed, true
	case map[string]any:
		return probe.ECNAnalysis{
			Supported:     boolValue(typed["supported"]),
			Mode:          stringValue(typed["mode"]),
			ObservedProto: stringValue(typed["observed_proto"]),
			ALPN:          stringValue(typed["alpn"]),
			ECNVisible:    boolValue(typed["ecn_visible"]),
			PacketMarks:   stringValue(typed["packet_marks"]),
			Note:          stringValue(typed["note"]),
		}, true
	default:
		return probe.ECNAnalysis{}, false
	}
}

func anyToSpinBitAnalysis(value any) (probe.SpinBitAnalysis, bool) {
	switch typed := value.(type) {
	case probe.SpinBitAnalysis:
		return typed, true
	case map[string]any:
		return probe.SpinBitAnalysis{
			Supported:     boolValue(typed["supported"]),
			Mode:          stringValue(typed["mode"]),
			RTTEstimateMS: floatValue(typed["rtt_estimate_ms"]),
			P95MS:         floatValue(typed["p95_ms"]),
			Stability:     stringValue(typed["stability"]),
			SpinObserved:  boolValue(typed["spin_observed"]),
			Note:          stringValue(typed["note"]),
		}, true
	default:
		return probe.SpinBitAnalysis{}, false
	}
}

func anyToLossAnalysis(value any) (probe.LossAnalysis, bool) {
	switch typed := value.(type) {
	case probe.LossAnalysis:
		return probe.LossAnalysis{
			Supported:         typed.Supported,
			Mode:              typed.Mode,
			Signal:            typed.Signal,
			LatencyErrors:     typed.LatencyErrors,
			LatencySamples:    typed.LatencySamples,
			StreamAttempts:    typed.StreamAttempts,
			StreamErrors:      typed.StreamErrors,
			SuccessRate:       typed.SuccessRate,
			ErrorCategories:   copyStringIntMap(typed.ErrorCategories),
			TimeoutIndicators: typed.TimeoutIndicators,
			Note:              typed.Note,
		}, true
	case map[string]any:
		return probe.LossAnalysis{
			Supported:         boolValue(typed["supported"]),
			Mode:              stringValue(typed["mode"]),
			Signal:            stringValue(typed["signal"]),
			LatencyErrors:     intValue(typed["latency_errors"]),
			LatencySamples:    intValue(typed["latency_samples"]),
			StreamAttempts:    intValue(typed["stream_attempts"]),
			StreamErrors:      intValue(typed["stream_errors"]),
			SuccessRate:       floatValue(typed["success_rate"]),
			ErrorCategories:   anyToStringIntMap(typed["error_categories"]),
			TimeoutIndicators: intValue(typed["timeout_indicators"]),
			Note:              stringValue(typed["note"]),
		}, true
	default:
		return probe.LossAnalysis{}, false
	}
}

func anyToCongestionAnalysis(value any) (probe.CongestionAnalysis, bool) {
	switch typed := value.(type) {
	case probe.CongestionAnalysis:
		return typed, true
	case map[string]any:
		return probe.CongestionAnalysis{
			Supported:          boolValue(typed["supported"]),
			Mode:               stringValue(typed["mode"]),
			Signal:             stringValue(typed["signal"]),
			P50MS:              floatValue(typed["p50_ms"]),
			P95MS:              floatValue(typed["p95_ms"]),
			JitterMS:           floatValue(typed["jitter_ms"]),
			SpreadRatio:        floatValue(typed["spread_ratio"]),
			StreamAverageMS:    floatValue(typed["stream_avg_ms"]),
			StreamP95MS:        floatValue(typed["stream_p95_ms"]),
			ConcurrentAttempts: intValue(typed["concurrent_attempts"]),
			SuccessRate:        floatValue(typed["success_rate"]),
			Note:               stringValue(typed["note"]),
		}, true
	default:
		return probe.CongestionAnalysis{}, false
	}
}

func anyToZeroRTTAnalysis(value any) (probe.ZeroRTTAnalysis, bool) {
	switch typed := value.(type) {
	case probe.ZeroRTTAnalysis:
		return typed, true
	case map[string]any:
		return probe.ZeroRTTAnalysis{
			Supported:      boolValue(typed["supported"]),
			Mode:           stringValue(typed["mode"]),
			InitialMS:      floatValue(typed["initial_ms"]),
			ResumedMS:      floatValue(typed["resumed_ms"]),
			InitialResumed: boolValue(typed["initial_resumed"]),
			Resumed:        boolValue(typed["resumed"]),
			TimeSavedMS:    floatValue(typed["time_saved_ms"]),
			Requested0RTT:  boolValue(typed["requested_0rtt"]),
			Note:           stringValue(typed["note"]),
			Error:          stringValue(typed["error"]),
		}, true
	default:
		return probe.ZeroRTTAnalysis{}, false
	}
}

func anyToMigrationAnalysis(value any) (probe.MigrationAnalysis, bool) {
	switch typed := value.(type) {
	case probe.MigrationAnalysis:
		return typed, true
	case map[string]any:
		return probe.MigrationAnalysis{
			Supported:      boolValue(typed["supported"]),
			Mode:           stringValue(typed["mode"]),
			Target:         stringValue(typed["target"]),
			StatusClass:    intValue(typed["status_class"]),
			BodyBytes:      intValue(typed["body_bytes"]),
			DurationMS:     floatValue(typed["duration_ms"]),
			RequestedCheck: boolValue(typed["requested_check"]),
			Note:           stringValue(typed["note"]),
			Message:        stringValue(typed["message"]),
			Error:          stringValue(typed["error"]),
		}, true
	default:
		return probe.MigrationAnalysis{}, false
	}
}

func anyToLatencyAnalysis(value any) (probe.LatencyAnalysis, bool) {
	switch typed := value.(type) {
	case probe.LatencyAnalysis:
		return probe.LatencyAnalysis{
			Samples:   typed.Samples,
			AverageMS: typed.AverageMS,
			P50:       typed.P50,
			P95:       typed.P95,
			P99:       typed.P99,
			Errors:    typed.Errors,
			SamplesMS: append([]float64(nil), typed.SamplesMS...),
		}, true
	case map[string]any:
		return probe.LatencyAnalysis{
			Samples:   intValue(typed["samples"]),
			AverageMS: floatValue(typed["avg_ms"]),
			P50:       floatValue(typed["p50"]),
			P95:       floatValue(typed["p95"]),
			P99:       floatValue(typed["p99"]),
			Errors:    intValue(typed["errors"]),
			SamplesMS: anyToFloat64Slice(typed["samples_ms"]),
		}, true
	default:
		return probe.LatencyAnalysis{}, false
	}
}

func anyToStreamAnalysis(value any) (probe.StreamAnalysis, bool) {
	switch typed := value.(type) {
	case probe.StreamAnalysis:
		return probe.StreamAnalysis{
			Attempted:       typed.Attempted,
			Successful:      typed.Successful,
			Errors:          typed.Errors,
			SuccessRate:     typed.SuccessRate,
			AverageLatency:  typed.AverageLatency,
			P95Latency:      typed.P95Latency,
			ThroughputBytes: typed.ThroughputBytes,
			StatusClasses:   copyStringIntMap(typed.StatusClasses),
			ErrorCategories: copyStringIntMap(typed.ErrorCategories),
		}, true
	case map[string]any:
		return probe.StreamAnalysis{
			Attempted:       intValue(typed["attempted"]),
			Successful:      intValue(typed["successful"]),
			Errors:          intValue(typed["errors"]),
			SuccessRate:     floatValue(typed["success_rate"]),
			AverageLatency:  floatValue(typed["avg_latency_ms"]),
			P95Latency:      floatValue(typed["p95_latency_ms"]),
			ThroughputBytes: int64Value(typed["throughput_bytes"]),
			StatusClasses:   anyToStringIntMap(typed["status_classes"]),
			ErrorCategories: anyToStringIntMap(typed["error_categories"]),
		}, true
	default:
		return probe.StreamAnalysis{}, false
	}
}

func anyToSkippedTests(value any) []probe.SkippedTest {
	switch typed := value.(type) {
	case []probe.SkippedTest:
		return append([]probe.SkippedTest(nil), typed...)
	case []map[string]any:
		out := make([]probe.SkippedTest, 0, len(typed))
		for _, item := range typed {
			out = append(out, probe.SkippedTest{
				Name:   stringValue(item["name"]),
				Reason: stringValue(item["reason"]),
			})
		}
		return out
	case []any:
		out := make([]probe.SkippedTest, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(probe.SkippedTest); ok {
				out = append(out, entry)
				continue
			}
			if m, ok := item.(map[string]any); ok {
				out = append(out, probe.SkippedTest{
					Name:   stringValue(m["name"]),
					Reason: stringValue(m["reason"]),
				})
			}
		}
		return out
	default:
		return nil
	}
}

func formatSkippedTests(skipped []probe.SkippedTest) string {
	if len(skipped) == 0 {
		return ""
	}
	parts := make([]string, 0, len(skipped))
	for _, item := range skipped {
		parts = append(parts, fmt.Sprintf("%s (%s)", item.Name, item.Reason))
	}
	return strings.Join(parts, ", ")
}

func anyToSupportEntries(value any) map[string]probe.SupportEntry {
	switch typed := value.(type) {
	case map[string]probe.SupportEntry:
		out := make(map[string]probe.SupportEntry, len(typed))
		for key, entry := range typed {
			out[key] = entry
		}
		return out
	case map[string]any:
		out := make(map[string]probe.SupportEntry, len(typed))
		for key, item := range typed {
			switch entry := item.(type) {
			case probe.SupportEntry:
				out[key] = entry
			case map[string]any:
				out[key] = probe.SupportEntry{
					Requested: boolValue(entry["requested"]),
					Coverage:  stringValue(entry["coverage"]),
					State:     stringValue(entry["state"]),
					Summary:   stringValue(entry["summary"]),
					Mode:      stringValue(entry["mode"]),
				}
			}
		}
		return out
	default:
		return nil
	}
}

func anyToFloat64Slice(value any) []float64 {
	switch typed := value.(type) {
	case []float64:
		return append([]float64(nil), typed...)
	case []any:
		out := make([]float64, 0, len(typed))
		for _, item := range typed {
			out = append(out, floatValue(item))
		}
		return out
	default:
		return nil
	}
}

func anyToStringIntMap(value any) map[string]int {
	switch typed := value.(type) {
	case map[string]int:
		return copyStringIntMap(typed)
	case map[string]any:
		out := make(map[string]int, len(typed))
		for key, item := range typed {
			out[key] = intValue(item)
		}
		return out
	default:
		return nil
	}
}

func copyStringIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func anyToSupportSummary(value any) (probe.SupportSummary, bool) {
	switch typed := value.(type) {
	case probe.SupportSummary:
		return typed, true
	case map[string]any:
		return probe.SupportSummary{
			RequestedTests: intValue(typed["requested_tests"]),
			Available:      intValue(typed["available"]),
			NotRun:         intValue(typed["not_run"]),
			Unavailable:    intValue(typed["unavailable"]),
			Full:           intValue(typed["full"]),
			Partial:        intValue(typed["partial"]),
			CoverageRatio:  floatValue(typed["coverage_ratio"]),
		}, true
	default:
		return probe.SupportSummary{}, false
	}
}

func sortedSupportKeys(input map[string]probe.SupportEntry) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func boolValue(value any) bool {
	b, _ := value.(bool)
	return b
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return safeUint64ToInt64(uint64(typed))
	case uint32:
		return int64(typed)
	case uint64:
		return safeUint64ToInt64(typed)
	case float64:
		if typed > math.MaxInt64 {
			return math.MaxInt64
		}
		if typed < math.MinInt64 {
			return math.MinInt64
		}
		return int64(typed)
	default:
		return 0
	}
}

func safeUint64ToInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
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
	if result.Summary.Protocols > 0 {
		fmt.Fprintf(&b, "- Summary: healthy `%v`, degraded `%v`, failed `%v`, best `%v`, risk `%v`\n\n",
			result.Summary.HealthyProtocols,
			result.Summary.DegradedProtocols,
			result.Summary.FailedProtocols,
			result.Summary.BestProtocol,
			result.Summary.RiskiestProtocol)
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
