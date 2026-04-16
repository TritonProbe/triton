package dashboard

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

func (s *Server) probeSummary(item storage.Item) (ProbeSummary, error) {
	cacheKey := item.ID
	modTime := item.ModTime.UTC().Format(time.RFC3339Nano)
	s.cacheMu.RLock()
	cached, ok := s.probeCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok && cached.modTime == modTime && cached.size == item.Size {
		return cached.summary, nil
	}
	summary, err := s.loadPersistedProbeSummary(item)
	if err != nil {
		return ProbeSummary{}, err
	}
	s.cacheMu.Lock()
	s.probeCache[cacheKey] = cachedProbeSummary{modTime: modTime, size: item.Size, summary: summary}
	s.cacheMu.Unlock()
	return summary, nil
}

func BuildProbeSummary(result probe.Result) ProbeSummary {
	return ProbeSummary{
		ID:           result.ID,
		Target:       result.Target,
		Timestamp:    result.Timestamp.UTC().Format(time.RFC3339),
		Status:       result.Status,
		Proto:        result.Proto,
		Duration:     result.Duration.String(),
		Analysis:     result.Analysis,
		AnalysisView: buildProbeAnalysisView(result.Analysis),
		TraceFiles:   append([]string(nil), result.TraceFiles...),
	}
}

func (s *Server) loadPersistedProbeSummary(item storage.Item) (ProbeSummary, error) {
	var summary ProbeSummary
	if err := s.store.LoadProbeSummary(item.ID, &summary); err == nil {
		summary.ModTime = item.ModTime.UTC().Format(time.RFC3339)
		summary.Size = item.Size
		if probeAnalysisViewEmpty(summary.AnalysisView) && len(summary.Analysis) > 0 {
			summary.AnalysisView = buildProbeAnalysisView(summary.Analysis)
		}
		return summary, nil
	}

	var result probe.Result
	if err := s.store.Load("probes", item.ID, &result); err != nil {
		return ProbeSummary{}, err
	}
	summary = BuildProbeSummary(result)
	summary.ModTime = item.ModTime.UTC().Format(time.RFC3339)
	summary.Size = item.Size
	_ = s.store.SaveProbeSummary(item.ID, summary)
	return summary, nil
}

func buildProbeAnalysisView(analysis map[string]any) ProbeAnalysisView {
	if len(analysis) == 0 {
		return ProbeAnalysisView{}
	}
	return ProbeAnalysisView{
		Response:        decodeAnalysis[probe.ResponseAnalysis](analysis["response"]),
		Latency:         decodeAnalysis[probe.LatencyAnalysis](analysis["latency"]),
		Streams:         decodeAnalysis[probe.StreamAnalysis](analysis["streams"]),
		AltSvc:          decodeAnalysis[probe.AltSvcAnalysis](analysis["alt_svc"]),
		ZeroRTT:         decodeAnalysis[probe.ZeroRTTAnalysis](analysis["0rtt"]),
		Migration:       decodeAnalysis[probe.MigrationAnalysis](analysis["migration"]),
		QPACK:           decodeAnalysis[probe.QPACKAnalysis](analysis["qpack"]),
		Loss:            decodeAnalysis[probe.LossAnalysis](analysis["loss"]),
		Congestion:      decodeAnalysis[probe.CongestionAnalysis](analysis["congestion"]),
		Version:         decodeAnalysis[probe.VersionAnalysis](analysis["version"]),
		Retry:           decodeAnalysis[probe.RetryAnalysis](analysis["retry"]),
		ECN:             decodeAnalysis[probe.ECNAnalysis](analysis["ecn"]),
		SpinBit:         decodeAnalysis[probe.SpinBitAnalysis](analysis["spin-bit"]),
		Support:         decodeSupportEntries(analysis["support"]),
		SupportSummary:  decodeAnalysis[probe.SupportSummary](analysis["support_summary"]),
		FidelitySummary: decodeAnalysis[probe.FidelitySummary](analysis["fidelity_summary"]),
		TestPlan:        decodeAnalysis[probe.TestPlan](analysis["test_plan"]),
	}
}

func decodeSupportEntries(value any) map[string]probe.SupportEntry {
	switch typed := value.(type) {
	case map[string]probe.SupportEntry:
		if len(typed) == 0 {
			return nil
		}
		out := make(map[string]probe.SupportEntry, len(typed))
		for key, entry := range typed {
			out[key] = entry
		}
		return out
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		out := make(map[string]probe.SupportEntry, len(typed))
		for key, item := range typed {
			switch entry := item.(type) {
			case probe.SupportEntry:
				out[key] = entry
			case map[string]any:
				decoded := decodeAnalysis[probe.SupportEntry](entry)
				if decoded != nil {
					out[key] = *decoded
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func decodeAnalysis[T any](value any) *T {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case T:
		out := typed
		return &out
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil
		}
		var out T
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return &out
	}
}

func (s *Server) benchSummary(item storage.Item) (BenchSummary, error) {
	cacheKey := item.ID
	modTime := item.ModTime.UTC().Format(time.RFC3339Nano)
	s.cacheMu.RLock()
	cached, ok := s.benchCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok && cached.modTime == modTime && cached.size == item.Size {
		return cached.summary, nil
	}
	summary, err := s.loadPersistedBenchSummary(item)
	if err != nil {
		return BenchSummary{}, err
	}
	s.cacheMu.Lock()
	s.benchCache[cacheKey] = cachedBenchSummary{modTime: modTime, size: item.Size, summary: summary}
	s.cacheMu.Unlock()
	return summary, nil
}

func BuildBenchSummary(result bench.Result) BenchSummary {
	return BenchSummary{
		ID:          result.ID,
		Target:      result.Target,
		Timestamp:   result.Timestamp.UTC().Format(time.RFC3339),
		Duration:    result.Duration.String(),
		Concurrency: result.Concurrency,
		Protocols:   append([]string(nil), result.Protocols...),
		Summary:     result.Summary,
		Stats:       result.Stats,
		StatsView:   buildBenchStatsView(result.Stats),
		TraceFiles:  append([]string(nil), result.TraceFiles...),
	}
}

func (s *Server) loadPersistedBenchSummary(item storage.Item) (BenchSummary, error) {
	var summary BenchSummary
	if err := s.store.LoadBenchSummary(item.ID, &summary); err == nil {
		summary.ModTime = item.ModTime.UTC().Format(time.RFC3339)
		summary.Size = item.Size
		if len(summary.StatsView) == 0 && len(summary.Stats) > 0 {
			summary.StatsView = buildBenchStatsView(summary.Stats)
		}
		return summary, nil
	}

	var result bench.Result
	if err := s.store.Load("benches", item.ID, &result); err != nil {
		return BenchSummary{}, err
	}
	summary = BuildBenchSummary(result)
	summary.ModTime = item.ModTime.UTC().Format(time.RFC3339)
	summary.Size = item.Size
	_ = s.store.SaveBenchSummary(item.ID, summary)
	return summary, nil
}

func buildBenchStatsView(stats map[string]bench.Stats) []BenchProtocolView {
	if len(stats) == 0 {
		return nil
	}
	keys := make([]string, 0, len(stats))
	for protocol := range stats {
		keys = append(keys, protocol)
	}
	sort.Strings(keys)
	out := make([]BenchProtocolView, 0, len(keys))
	for _, protocol := range keys {
		out = append(out, BenchProtocolView{
			Protocol: protocol,
			Stats:    stats[protocol],
		})
	}
	return out
}

func probeAnalysisViewEmpty(view ProbeAnalysisView) bool {
	return view.Response == nil &&
		view.Latency == nil &&
		view.Streams == nil &&
		view.AltSvc == nil &&
		view.ZeroRTT == nil &&
		view.Migration == nil &&
		view.QPACK == nil &&
		view.Loss == nil &&
		view.Congestion == nil &&
		view.Version == nil &&
		view.Retry == nil &&
		view.ECN == nil &&
		view.SpinBit == nil &&
		len(view.Support) == 0 &&
		view.SupportSummary == nil &&
		view.FidelitySummary == nil &&
		view.TestPlan == nil
}
