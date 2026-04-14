package dashboard

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	http      *http.Server
	store     *storage.FileStore
	trace     string
	config    map[string]any
	startedAt time.Time
}

type TraceMetadata struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ModifiedAt  string `json:"modified_at"`
	DownloadURL string `json:"download_url"`
	MetaURL     string `json:"meta_url"`
	Preview     string `json:"preview"`
}

type ProbeSummary struct {
	ID           string            `json:"id"`
	Target       string            `json:"target"`
	Timestamp    string            `json:"timestamp"`
	Status       int               `json:"status"`
	Proto        string            `json:"proto"`
	Duration     string            `json:"duration"`
	ModTime      string            `json:"mod_time"`
	Size         int64             `json:"size"`
	Analysis     map[string]any    `json:"analysis,omitempty"`
	AnalysisView ProbeAnalysisView `json:"analysis_view,omitempty"`
	TraceFiles   []string          `json:"trace_files,omitempty"`
}

type ProbeAnalysisView struct {
	Response       *probe.ResponseAnalysis       `json:"response,omitempty"`
	Latency        *probe.LatencyAnalysis        `json:"latency,omitempty"`
	Streams        *probe.StreamAnalysis         `json:"streams,omitempty"`
	AltSvc         *probe.AltSvcAnalysis         `json:"alt_svc,omitempty"`
	ZeroRTT        *probe.ZeroRTTAnalysis        `json:"0rtt,omitempty"`
	Migration      *probe.MigrationAnalysis      `json:"migration,omitempty"`
	QPACK          *probe.QPACKAnalysis          `json:"qpack,omitempty"`
	Loss           *probe.LossAnalysis           `json:"loss,omitempty"`
	Congestion     *probe.CongestionAnalysis     `json:"congestion,omitempty"`
	Version        *probe.VersionAnalysis        `json:"version,omitempty"`
	Retry          *probe.RetryAnalysis          `json:"retry,omitempty"`
	ECN            *probe.ECNAnalysis            `json:"ecn,omitempty"`
	SpinBit        *probe.SpinBitAnalysis        `json:"spin-bit,omitempty"`
	Support        map[string]probe.SupportEntry `json:"support,omitempty"`
	SupportSummary *probe.SupportSummary         `json:"support_summary,omitempty"`
	TestPlan       *probe.TestPlan               `json:"test_plan,omitempty"`
}

type BenchSummary struct {
	ID          string                 `json:"id"`
	Target      string                 `json:"target"`
	Timestamp   string                 `json:"timestamp"`
	Duration    string                 `json:"duration"`
	Concurrency int                    `json:"concurrency"`
	Protocols   []string               `json:"protocols"`
	Summary     bench.Summary          `json:"summary"`
	Stats       map[string]bench.Stats `json:"stats"`
	StatsView   []BenchProtocolView    `json:"stats_view,omitempty"`
	ModTime     string                 `json:"mod_time"`
	Size        int64                  `json:"size"`
	TraceFiles  []string               `json:"trace_files,omitempty"`
}

type BenchProtocolView struct {
	Protocol string      `json:"protocol"`
	Stats    bench.Stats `json:"stats"`
}

type StatusResponse struct {
	Status    string          `json:"status"`
	Dashboard DashboardStatus `json:"dashboard"`
	Storage   StorageStatus   `json:"storage"`
}

type DashboardStatus struct {
	StartedAt     string `json:"started_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	TraceEnabled  bool   `json:"trace_enabled"`
}

type StorageStatus struct {
	Probes  int `json:"probes"`
	Benches int `json:"benches"`
	Traces  int `json:"traces"`
}

type APIErrorResponse struct {
	Status string         `json:"status"`
	Error  APIErrorDetail `json:"error"`
}

type APIErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type listQuery struct {
	Limit int
	Q     string
	Sort  string
}

type Options struct {
	Username string
	Password string
	Logger   *observability.ManagedLogger
	TraceDir string
	Config   map[string]any
}

func New(addr string, store *storage.FileStore, opts Options) *Server {
	mux := http.NewServeMux()
	logger := opts.Logger
	if logger == nil {
		logger = &observability.ManagedLogger{}
	}
	s := &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           observability.WithRequestID(observability.WithAccessLog(logger.Logger, "dashboard", withSecurityHeaders(withOptionalBasicAuth(mux, opts)))),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		store:     store,
		trace:     opts.TraceDir,
		config:    cloneMap(opts.Config),
		startedAt: time.Now().UTC(),
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch {
		case r.URL.Path == "/" || r.URL.Path == "/index.html":
			serveAsset(w, "assets/index.html", "text/html; charset=utf-8")
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
			switch path.Ext(name) {
			case ".css":
				serveAsset(w, name, "text/css; charset=utf-8")
			case ".js":
				serveAsset(w, name, "application/javascript; charset=utf-8")
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/v1/", s.handleAPINotFound)
	mux.HandleFunc("/api/v1/status", getOnly(s.handleStatus))
	mux.HandleFunc("/api/v1/config", getOnly(s.handleConfig))
	mux.HandleFunc("/api/v1/probes", getOnly(s.handleProbes))
	mux.HandleFunc("/api/v1/benches", getOnly(s.handleBenches))
	mux.HandleFunc("/api/v1/probes/", getOnly(s.handleProbe))
	mux.HandleFunc("/api/v1/benches/", getOnly(s.handleBench))
	mux.HandleFunc("/api/v1/traces", getOnly(s.handleTraces))
	mux.HandleFunc("/api/v1/traces/meta/", getOnly(s.handleTraceMeta))
	mux.HandleFunc("/api/v1/traces/", getOnly(s.handleTrace))
	return s
}

func (s *Server) Run() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	probes, err := s.store.List("probes")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list probes", err)
		return
	}
	benches, err := s.store.List("benches")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list benches", err)
		return
	}
	traces, err := s.listTraces()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list traces", err)
		return
	}
	writeJSON(w, StatusResponse{
		Status: "ok",
		Dashboard: DashboardStatus{
			StartedAt:     s.startedAt.Format(time.RFC3339),
			UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
			TraceEnabled:  s.trace != "",
		},
		Storage: StorageStatus{
			Probes:  len(probes),
			Benches: len(benches),
			Traces:  len(traces),
		},
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.config)
}

func (s *Server) handleProbes(w http.ResponseWriter, r *http.Request) {
	query := parseListQuery(r)
	items, err := s.store.List("probes")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list probes", err)
		return
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
	summaries = applyLimit(summaries, query.Limit)
	writeJSON(w, summaries)
}

func (s *Server) handleBenches(w http.ResponseWriter, r *http.Request) {
	query := parseListQuery(r)
	items, err := s.store.List("benches")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list benches", err)
		return
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
	summaries = applyLimit(summaries, query.Limit)
	writeJSON(w, summaries)
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/probes/")
	if id == "" {
		writeAPIError(w, http.StatusNotFound, "probe id is required", nil)
		return
	}
	var result probe.Result
	if err := s.store.Load("probes", id, &result); err != nil {
		writeAPIError(w, http.StatusNotFound, "probe result not found", err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleBench(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/benches/")
	if id == "" {
		writeAPIError(w, http.StatusNotFound, "bench id is required", nil)
		return
	}
	var result bench.Result
	if err := s.store.Load("benches", id, &result); err != nil {
		writeAPIError(w, http.StatusNotFound, "bench result not found", err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	query := parseListQuery(r)
	items, err := s.listTraces()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list traces", err)
		return
	}
	items = filterTraceMetadata(items, query.Q)
	sortTraceMetadata(items, query.Sort)
	items = applyLimit(items, query.Limit)
	writeJSON(w, items)
}

func (s *Server) handleTraceMeta(w http.ResponseWriter, r *http.Request) {
	name := path.Base(strings.TrimPrefix(r.URL.Path, "/api/v1/traces/meta/"))
	item, err := s.traceMetadata(name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "trace not found", err)
		return
	}
	writeJSON(w, item)
}

func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	name := path.Base(strings.TrimPrefix(r.URL.Path, "/api/v1/traces/"))
	_, err := s.traceMetadata(name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "trace not found", err)
		return
	}
	fullPath := filepath.Join(s.trace, name)
	w.Header().Set("Content-Type", "application/qlog+json-seq")
	http.ServeFile(w, r, fullPath)
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeAPIError(w, http.StatusNotFound, "api route not found", nil)
}

func (s *Server) listTraces() ([]TraceMetadata, error) {
	if s.trace == "" {
		return []TraceMetadata{}, nil
	}
	entries, err := os.ReadDir(s.trace)
	if err != nil {
		if os.IsNotExist(err) {
			return []TraceMetadata{}, nil
		}
		return nil, err
	}

	items := make([]TraceMetadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sqlog" {
			continue
		}
		item, err := s.traceMetadata(entry.Name())
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Server) traceMetadata(name string) (TraceMetadata, error) {
	if name == "." || name == "" || strings.Contains(name, "/") || filepath.Ext(name) != ".sqlog" {
		return TraceMetadata{}, os.ErrNotExist
	}
	if s.trace == "" {
		return TraceMetadata{}, os.ErrNotExist
	}
	fullPath := filepath.Join(s.trace, name)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		if err == nil {
			err = os.ErrNotExist
		}
		return TraceMetadata{}, err
	}
	preview, err := readTracePreview(fullPath)
	if err != nil {
		return TraceMetadata{}, err
	}
	return TraceMetadata{
		Name:        name,
		SizeBytes:   info.Size(),
		ModifiedAt:  info.ModTime().UTC().Format(time.RFC3339),
		DownloadURL: "/api/v1/traces/" + name,
		MetaURL:     "/api/v1/traces/meta/" + name,
		Preview:     preview,
	}, nil
}

func (s *Server) probeSummary(item storage.Item) (ProbeSummary, error) {
	var result probe.Result
	if err := s.store.Load("probes", item.ID, &result); err != nil {
		return ProbeSummary{}, err
	}
	return ProbeSummary{
		ID:           item.ID,
		Target:       result.Target,
		Timestamp:    result.Timestamp.UTC().Format(time.RFC3339),
		Status:       result.Status,
		Proto:        result.Proto,
		Duration:     result.Duration.String(),
		ModTime:      item.ModTime.UTC().Format(time.RFC3339),
		Size:         item.Size,
		Analysis:     result.Analysis,
		AnalysisView: buildProbeAnalysisView(result.Analysis),
		TraceFiles:   append([]string(nil), result.TraceFiles...),
	}, nil
}

func buildProbeAnalysisView(analysis map[string]any) ProbeAnalysisView {
	if len(analysis) == 0 {
		return ProbeAnalysisView{}
	}
	return ProbeAnalysisView{
		Response:       decodeAnalysis[probe.ResponseAnalysis](analysis["response"]),
		Latency:        decodeAnalysis[probe.LatencyAnalysis](analysis["latency"]),
		Streams:        decodeAnalysis[probe.StreamAnalysis](analysis["streams"]),
		AltSvc:         decodeAnalysis[probe.AltSvcAnalysis](analysis["alt_svc"]),
		ZeroRTT:        decodeAnalysis[probe.ZeroRTTAnalysis](analysis["0rtt"]),
		Migration:      decodeAnalysis[probe.MigrationAnalysis](analysis["migration"]),
		QPACK:          decodeAnalysis[probe.QPACKAnalysis](analysis["qpack"]),
		Loss:           decodeAnalysis[probe.LossAnalysis](analysis["loss"]),
		Congestion:     decodeAnalysis[probe.CongestionAnalysis](analysis["congestion"]),
		Version:        decodeAnalysis[probe.VersionAnalysis](analysis["version"]),
		Retry:          decodeAnalysis[probe.RetryAnalysis](analysis["retry"]),
		ECN:            decodeAnalysis[probe.ECNAnalysis](analysis["ecn"]),
		SpinBit:        decodeAnalysis[probe.SpinBitAnalysis](analysis["spin-bit"]),
		Support:        decodeSupportEntries(analysis["support"]),
		SupportSummary: decodeAnalysis[probe.SupportSummary](analysis["support_summary"]),
		TestPlan:       decodeAnalysis[probe.TestPlan](analysis["test_plan"]),
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
	var result bench.Result
	if err := s.store.Load("benches", item.ID, &result); err != nil {
		return BenchSummary{}, err
	}
	return BenchSummary{
		ID:          item.ID,
		Target:      result.Target,
		Timestamp:   result.Timestamp.UTC().Format(time.RFC3339),
		Duration:    result.Duration.String(),
		Concurrency: result.Concurrency,
		Protocols:   append([]string(nil), result.Protocols...),
		Summary:     result.Summary,
		Stats:       result.Stats,
		StatsView:   buildBenchStatsView(result.Stats),
		ModTime:     item.ModTime.UTC().Format(time.RFC3339),
		Size:        item.Size,
		TraceFiles:  append([]string(nil), result.TraceFiles...),
	}, nil
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func readTracePreview(fullPath string) (string, error) {
	// #nosec G304 -- fullPath is derived from validated trace metadata constrained to s.trace.
	file, err := os.Open(fullPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := make([]byte, 256)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return string(buf[:n]), nil
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

func withOptionalBasicAuth(next http.Handler, opts Options) http.Handler {
	if opts.Username == "" && opts.Password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(opts.Username)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(opts.Password)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="triton-dashboard"`)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", nil)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
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
	limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
	limit := 0
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
	return listQuery{
		Limit: limit,
		Q:     q,
		Sort:  sortBy,
	}
}

func applyLimit[T any](items []T, limit int) []T {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
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
