package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func readTracePreview(fullPath string) (string, error) {
	file, err := os.Open(fullPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 512))
	if err != nil {
		return "", err
	}
	preview := strings.TrimSpace(string(data))
	if preview == "" {
		return "(empty trace)", nil
	}
	return preview, nil
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func writeAPIError(w http.ResponseWriter, status int, message string, err error) {
	payload := APIErrorResponse{
		Status: "error",
		Error: APIErrorDetail{
			Code:    status,
			Message: message,
		},
	}
	if err != nil {
		payload.Error.Detail = "see server logs"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func serveAsset(w http.ResponseWriter, name, contentType string) {
	data, err := assets.ReadFile(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("asset not found: %s", name), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	// #nosec G705 -- embedded assets are static, trusted bytes from go:embed.
	_, _ = w.Write(data)
}

func getOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
			return
		}
		next(w, r)
	}
}

func parseListQuery(r *http.Request) listQuery {
	if r == nil {
		return listQuery{}
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
	limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
	offsetRaw := strings.TrimSpace(r.URL.Query().Get("offset"))
	limit := 0
	offset := 0
	if limitRaw != "" {
		if parsed, err := strconv.Atoi(limitRaw); err == nil {
			if parsed > 200 {
				parsed = 200
			}
			if parsed > 0 {
				limit = parsed
			}
		}
	}
	if offsetRaw != "" {
		if parsed, err := strconv.Atoi(offsetRaw); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	return listQuery{
		Limit:  limit,
		Offset: offset,
		Q:      q,
		Sort:   sortBy,
		View:   view,
	}
}

func applyOffsetLimit[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}
	if offset > 0 {
		items = items[offset:]
	}
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func writeListMetadataHeaders(w http.ResponseWriter, total int, query listQuery) {
	if w == nil {
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	w.Header().Set("X-Page-Offset", strconv.Itoa(query.Offset))
	w.Header().Set("X-Page-Limit", strconv.Itoa(query.Limit))
	hasMore := false
	if query.Limit > 0 {
		hasMore = query.Offset+query.Limit < total
	}
	if hasMore {
		w.Header().Set("X-Has-More", "true")
		w.Header().Set("X-Next-Offset", strconv.Itoa(query.Offset+query.Limit))
	} else {
		w.Header().Set("X-Has-More", "false")
		w.Header().Set("X-Next-Offset", "")
	}
}

func filterProbeSummaries(items []ProbeSummary, q string) []ProbeSummary {
	if q == "" {
		return items
	}
	filtered := make([]ProbeSummary, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID), q) ||
			strings.Contains(strings.ToLower(item.Target), q) ||
			strings.Contains(strings.ToLower(item.Proto), q) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func applyProbeView(items []ProbeSummary, view string) []ProbeSummary {
	if view != "summary" {
		return items
	}
	out := make([]ProbeSummary, 0, len(items))
	for _, item := range items {
		item.Analysis = nil
		out = append(out, item)
	}
	return out
}

func sortProbeSummaries(items []ProbeSummary, sortBy string) {
	switch sortBy {
	case "oldest":
		sort.SliceStable(items, func(i, j int) bool { return items[i].ModTime < items[j].ModTime })
	case "target_asc":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Target) < strings.ToLower(items[j].Target) })
	case "target_desc":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Target) > strings.ToLower(items[j].Target) })
	case "status_asc":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Status < items[j].Status })
	case "status_desc":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Status > items[j].Status })
	default:
		sort.SliceStable(items, func(i, j int) bool { return items[i].ModTime > items[j].ModTime })
	}
}

func filterBenchSummaries(items []BenchSummary, q string) []BenchSummary {
	if q == "" {
		return items
	}
	filtered := make([]BenchSummary, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID), q) ||
			strings.Contains(strings.ToLower(item.Target), q) ||
			strings.Contains(strings.ToLower(strings.Join(item.Protocols, ",")), q) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func applyBenchView(items []BenchSummary, view string) []BenchSummary {
	if view != "summary" {
		return items
	}
	out := make([]BenchSummary, 0, len(items))
	for _, item := range items {
		item.Stats = nil
		out = append(out, item)
	}
	return out
}

func sortBenchSummaries(items []BenchSummary, sortBy string) {
	switch sortBy {
	case "oldest":
		sort.SliceStable(items, func(i, j int) bool { return items[i].ModTime < items[j].ModTime })
	case "target_asc":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Target) < strings.ToLower(items[j].Target) })
	case "target_desc":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Target) > strings.ToLower(items[j].Target) })
	case "concurrency_asc":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Concurrency < items[j].Concurrency })
	case "concurrency_desc":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Concurrency > items[j].Concurrency })
	default:
		sort.SliceStable(items, func(i, j int) bool { return items[i].ModTime > items[j].ModTime })
	}
}

func filterTraceMetadata(items []TraceMetadata, q string) []TraceMetadata {
	if q == "" {
		return items
	}
	filtered := make([]TraceMetadata, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), q) ||
			strings.Contains(strings.ToLower(item.Preview), q) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func sortTraceMetadata(items []TraceMetadata, sortBy string) {
	switch sortBy {
	case "name_desc":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Name) > strings.ToLower(items[j].Name) })
	case "size_asc":
		sort.SliceStable(items, func(i, j int) bool { return items[i].SizeBytes < items[j].SizeBytes })
	case "size_desc":
		sort.SliceStable(items, func(i, j int) bool { return items[i].SizeBytes > items[j].SizeBytes })
	case "newest":
		sort.SliceStable(items, func(i, j int) bool { return items[i].ModifiedAt > items[j].ModifiedAt })
	case "oldest":
		sort.SliceStable(items, func(i, j int) bool { return items[i].ModifiedAt < items[j].ModifiedAt })
	default:
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	}
}

func (s *Server) probeList(query listQuery, items []storage.Item) ([]ProbeSummary, error) {
	signature := probeListSignature(query, items)
	cacheKey := encodeListQuery(query)
	s.cacheMu.RLock()
	cached, ok := s.probeListCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok && cached.signature == signature {
		return cloneProbeSummaries(cached.items), nil
	}
	summaries := make([]ProbeSummary, 0, len(items))
	for _, item := range items {
		summary, err := s.probeSummary(item)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}
	summaries = filterProbeSummaries(summaries, query.Q)
	sortProbeSummaries(summaries, query.Sort)
	summaries = applyProbeView(summaries, query.View)
	s.cacheMu.Lock()
	s.probeListCache[cacheKey] = cachedProbeList{signature: signature, items: cloneProbeSummaries(summaries)}
	s.cacheMu.Unlock()
	return summaries, nil
}

func (s *Server) benchList(query listQuery, items []storage.Item) ([]BenchSummary, error) {
	signature := benchListSignature(query, items)
	cacheKey := encodeListQuery(query)
	s.cacheMu.RLock()
	cached, ok := s.benchListCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok && cached.signature == signature {
		return cloneBenchSummaries(cached.items), nil
	}
	summaries := make([]BenchSummary, 0, len(items))
	for _, item := range items {
		summary, err := s.benchSummary(item)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}
	summaries = filterBenchSummaries(summaries, query.Q)
	sortBenchSummaries(summaries, query.Sort)
	summaries = applyBenchView(summaries, query.View)
	s.cacheMu.Lock()
	s.benchListCache[cacheKey] = cachedBenchList{signature: signature, items: cloneBenchSummaries(summaries)}
	s.cacheMu.Unlock()
	return summaries, nil
}

func (s *Server) traceList(query listQuery, items []TraceMetadata) []TraceMetadata {
	signature := traceListSignature(query, items)
	cacheKey := encodeListQuery(query)
	s.cacheMu.RLock()
	cached, ok := s.traceListCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok && cached.signature == signature {
		return cloneTraceMetadata(cached.items)
	}
	filtered := filterTraceMetadata(items, query.Q)
	sortTraceMetadata(filtered, query.Sort)
	s.cacheMu.Lock()
	s.traceListCache[cacheKey] = cachedTraceList{signature: signature, items: cloneTraceMetadata(filtered)}
	s.cacheMu.Unlock()
	return filtered
}

func encodeListQuery(query listQuery) string {
	return strings.Join([]string{
		query.Q,
		query.Sort,
		query.View,
		strconv.Itoa(query.Limit),
		strconv.Itoa(query.Offset),
	}, "|")
}

func probeListSignature(query listQuery, items []storage.Item) string {
	return encodeListQuery(query) + "|" + storageItemsSignature(items)
}

func benchListSignature(query listQuery, items []storage.Item) string {
	return encodeListQuery(query) + "|" + storageItemsSignature(items)
}

func traceListSignature(query listQuery, items []TraceMetadata) string {
	var builder strings.Builder
	builder.Grow(len(items) * 64)
	builder.WriteString(encodeListQuery(query))
	for _, item := range items {
		builder.WriteString("|")
		builder.WriteString(item.Name)
		builder.WriteString("@")
		builder.WriteString(item.ModifiedAt)
		builder.WriteString("#")
		builder.WriteString(strconv.FormatInt(item.SizeBytes, 10))
	}
	return builder.String()
}

func storageItemsSignature(items []storage.Item) string {
	var builder strings.Builder
	builder.Grow(len(items) * 64)
	for _, item := range items {
		builder.WriteString(item.ID)
		builder.WriteString("@")
		builder.WriteString(item.ModTime.UTC().Format(time.RFC3339Nano))
		builder.WriteString("#")
		builder.WriteString(strconv.FormatInt(item.Size, 10))
		builder.WriteString("|")
	}
	return builder.String()
}

func cloneProbeSummaries(items []ProbeSummary) []ProbeSummary {
	if len(items) == 0 {
		return []ProbeSummary{}
	}
	out := make([]ProbeSummary, 0, len(items))
	for _, item := range items {
		cloned := item
		if item.Analysis != nil {
			cloned.Analysis = cloneMap(item.Analysis)
		}
		cloned.TraceFiles = append([]string(nil), item.TraceFiles...)
		cloned.AnalysisView = cloneProbeAnalysisView(item.AnalysisView)
		out = append(out, cloned)
	}
	return out
}

func cloneProbeAnalysisView(view ProbeAnalysisView) ProbeAnalysisView {
	cloned := view
	if view.Support != nil {
		cloned.Support = make(map[string]probe.SupportEntry, len(view.Support))
		for key, entry := range view.Support {
			cloned.Support[key] = entry
		}
	}
	return cloned
}

func cloneBenchSummaries(items []BenchSummary) []BenchSummary {
	if len(items) == 0 {
		return []BenchSummary{}
	}
	out := make([]BenchSummary, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Protocols = append([]string(nil), item.Protocols...)
		cloned.TraceFiles = append([]string(nil), item.TraceFiles...)
		if item.Stats != nil {
			cloned.Stats = make(map[string]bench.Stats, len(item.Stats))
			for key, value := range item.Stats {
				cloned.Stats[key] = value
			}
		}
		cloned.StatsView = append([]BenchProtocolView(nil), item.StatsView...)
		out = append(out, cloned)
	}
	return out
}

func cloneTraceMetadata(items []TraceMetadata) []TraceMetadata {
	if len(items) == 0 {
		return []TraceMetadata{}
	}
	out := make([]TraceMetadata, len(items))
	copy(out, items)
	return out
}
