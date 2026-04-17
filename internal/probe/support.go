package probe

import (
	"fmt"
	"sort"
	"strings"
)

func fidelityDefinitions() map[string]FidelityDefinition {
	return map[string]FidelityDefinition{
		"full": {
			Label:       "full",
			Description: "direct current-path diagnostics",
		},
		"observed": {
			Label:       "observed",
			Description: "visible protocol/client-layer observation",
		},
		"partial": {
			Label:       "partial",
			Description: "heuristic, estimate, or capability-check output",
		},
		"unavailable": {
			Label:       "unavailable",
			Description: "requested but not available on the current path",
		},
	}
}

func fidelityDefinitionNotice(definitions map[string]FidelityDefinition) string {
	full := definitions["full"].Description
	observed := definitions["observed"].Description
	partial := definitions["partial"].Description
	return fmt.Sprintf("fidelity legend: full=%s; observed=%s; partial=%s", full, observed, partial)
}

func newTestPlan(requested []string) testPlan {
	normalized := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, value := range requested {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		normalized = []string{"handshake", "tls", "latency", "throughput", "streams", "alt-svc"}
	}
	return testPlan{requested: normalized}
}

func (p testPlan) shouldRun(name string) bool {
	for _, requested := range p.requested {
		if requested == "all" || requested == name {
			return true
		}
	}
	return false
}

func finalizeTestPlan(result *Result, plan testPlan) {
	if result == nil {
		return
	}
	if result.Analysis == nil {
		result.Analysis = map[string]any{}
	}
	for _, requested := range expandRequestedTests(plan.requested) {
		if containsString(plan.executed, requested) {
			continue
		}
		definition, ok := probeTestDefinitions[requested]
		if !ok {
			plan.skipped = append(plan.skipped, SkippedTest{Name: requested, Reason: "unknown probe test"})
			continue
		}
		if definition.Reason == "" {
			continue
		}
		plan.skipped = append(plan.skipped, SkippedTest{Name: requested, Reason: definition.Reason})
	}
	result.Analysis["test_plan"] = TestPlan{
		Requested: append([]string(nil), plan.requested...),
		Executed:  append([]string(nil), plan.executed...),
		Skipped:   append([]SkippedTest(nil), plan.skipped...),
	}
	support := buildSupportSummary(result.Analysis, plan)
	if len(support) > 0 {
		result.Analysis["support"] = support
		result.Analysis["support_summary"] = buildSupportRollup(support)
		result.Analysis["fidelity_summary"] = buildFidelitySummary(support)
	}
}

func buildSupportSummary(analysis map[string]any, plan testPlan) map[string]SupportEntry {
	support := map[string]SupportEntry{}
	for _, name := range expandRequestedTests(plan.requested) {
		addSupportEntry(support, name, analysis[name], true, containsString(plan.executed, name))
	}
	if len(support) == 0 {
		return nil
	}
	return support
}

func addSupportEntry(dst map[string]SupportEntry, name string, value any, requested, executed bool) {
	if dst == nil || !requested {
		return
	}
	entry := SupportEntry{Requested: requested}
	definition, known := probeTestDefinitions[name]
	details, _ := value.(map[string]any)
	if len(details) == 0 {
		coverage := "unavailable"
		summary := "not requested or not executed"
		if known && definition.Coverage != "" {
			coverage = definition.Coverage
			summary = definition.Summary
		}
		entry.Coverage = coverage
		entry.State = "not_run"
		if known && coverage == "unavailable" {
			entry.State = "unavailable"
		}
		if executed {
			entry.State = "available"
		}
		entry.Summary = summary
		dst[name] = entry
		return
	}

	mode, _ := details["mode"].(string)
	message, _ := details["message"].(string)
	note, _ := details["note"].(string)
	supported, _ := details["supported"].(bool)
	entry.Mode = mode
	switch {
	case known && definition.Coverage != "":
		entry.Coverage = definition.Coverage
	case mode == "tls-resumption-check", mode == "endpoint-capability-check":
		entry.Coverage = "partial"
	default:
		entry.Coverage = "full"
		if mode == "" {
			entry.Coverage = "observed"
		}
	}
	if supported {
		entry.State = "available"
	} else {
		entry.State = "unavailable"
	}
	summary := message
	if summary == "" {
		summary = note
	}
	if summary == "" && known {
		summary = definition.Summary
	}
	if summary == "" && mode != "" {
		summary = mode
	}
	entry.Summary = summary
	dst[name] = entry
}

func expandRequestedTests(requested []string) []string {
	if containsString(requested, "all") {
		return append([]string(nil), knownProbeTests...)
	}
	return append([]string(nil), requested...)
}

func buildSupportRollup(support map[string]SupportEntry) SupportSummary {
	summary := SupportSummary{}
	if len(support) == 0 {
		return summary
	}
	for _, key := range sortedProbeKeys(support) {
		entry := support[key]
		summary.RequestedTests++
		switch entry.Coverage {
		case "full":
			summary.Full++
		case "observed":
			summary.Observed++
		case "partial":
			summary.Partial++
		}
		switch entry.State {
		case "available":
			summary.Available++
		case "not_run":
			summary.NotRun++
		case "unavailable":
			summary.Unavailable++
		}
	}
	if summary.RequestedTests > 0 {
		summary.CoverageRatio = float64(summary.Available) / float64(summary.RequestedTests)
	}
	return summary
}

func buildFidelitySummary(support map[string]SupportEntry) FidelitySummary {
	definitions := fidelityDefinitions()
	summary := FidelitySummary{
		Definitions: definitions,
		PacketLevel: true,
	}
	if len(support) == 0 {
		return summary
	}
	for _, key := range sortedProbeKeys(support) {
		entry := support[key]
		switch classifyFidelity(key, entry) {
		case "full":
			summary.Full = append(summary.Full, key)
		case "partial":
			summary.Partial = append(summary.Partial, key)
			summary.PacketLevel = false
		case "observed":
			summary.Observed = append(summary.Observed, key)
			summary.PacketLevel = false
		case "unavailable":
			summary.Unavailable = append(summary.Unavailable, key)
			summary.PacketLevel = false
		default:
			summary.Partial = append(summary.Partial, key)
			summary.PacketLevel = false
		}
	}
	if len(summary.Partial) > 0 || len(summary.Observed) > 0 {
		parts := make([]string, 0, 2)
		if len(summary.Partial) > 0 {
			parts = append(parts, fmt.Sprintf("partial=%s", strings.Join(summary.Partial, ",")))
		}
		if len(summary.Observed) > 0 {
			parts = append(parts, fmt.Sprintf("observed=%s", strings.Join(summary.Observed, ",")))
		}
		summary.Notice = "advanced probe fields are not all packet-level telemetry; " + strings.Join(parts, "; ") + "; " + fidelityDefinitionNotice(definitions)
	}
	return summary
}

func classifyFidelity(name string, entry SupportEntry) string {
	switch name {
	case "version", "retry", "ecn":
		return "observed"
	case "0rtt", "migration", "qpack", "loss", "congestion", "spin-bit":
		return "partial"
	}
	switch entry.Mode {
	case "protocol-observation", "handshake-observation", "metadata-observation":
		return "observed"
	case "tls-resumption-check", "endpoint-capability-check", "header-block-estimate", "request-error-signal", "latency-spread-estimate", "rtt-sampling-estimate":
		return "partial"
	}
	switch entry.Coverage {
	case "full", "partial", "observed", "unavailable":
		return entry.Coverage
	default:
		return "partial"
	}
}

func sortedProbeKeys(input map[string]SupportEntry) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
