package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

func TestDashboardBasicAuth(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{
		Username: "admin",
		Password: "secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"error"`) {
		t.Fatalf("expected structured JSON auth error, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.SetBasicAuth("admin", "secret")
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected security headers, got %q", got)
	}
	if got := rec.Header().Get(observability.RequestIDHeader); got == "" {
		t.Fatal("expected request id header")
	}
	if srv.http.ReadHeaderTimeout != 5*time.Second || srv.http.ReadTimeout != 15*time.Second || srv.http.WriteTimeout != 15*time.Second || srv.http.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected dashboard timeouts: %+v", srv.http)
	}
}

func TestDashboardStatusRejectsWrongMethod(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"error"`) {
		t.Fatalf("expected structured JSON error, got %q", rec.Body.String())
	}
}

func TestDashboardListsAndServesTraces(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	traceDir := t.TempDir()
	traceFile := filepath.Join(traceDir, "abc_client.sqlog")
	if err := os.WriteFile(traceFile, []byte("trace-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, "zzz_server.sqlog"), []byte("zzz"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{TraceDir: traceDir})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from status, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON status payload: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %#v", payload["status"])
	}
	storagePayload, ok := payload["storage"].(map[string]any)
	if !ok {
		t.Fatalf("expected storage object in status payload, got %#v", payload["storage"])
	}
	if storagePayload["traces"] != float64(2) {
		t.Fatalf("expected two traces in status payload, got %#v", storagePayload["traces"])
	}
	dashboardPayload, ok := payload["dashboard"].(map[string]any)
	if !ok {
		t.Fatalf("expected dashboard object in status payload, got %#v", payload["dashboard"])
	}
	if dashboardPayload["version"] != "dev" || dashboardPayload["build_time"] != "unknown" {
		t.Fatalf("expected default build metadata in status payload, got %#v", dashboardPayload)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc_client.sqlog") {
		t.Fatalf("expected trace listing, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces?q=abc&sort=name_desc&limit=1", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from trace query list, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc_client.sqlog") || strings.Contains(rec.Body.String(), "zzz_server.sqlog") {
		t.Fatalf("expected filtered trace listing, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc_client.sqlog", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "trace-data" {
		t.Fatalf("unexpected trace body: %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/meta/abc_client.sqlog", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from trace metadata, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"preview":"trace-data"`) {
		t.Fatalf("expected preview in trace metadata, got %q", rec.Body.String())
	}
}

func TestDashboardConfigEndpoint(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{
		Config: map[string]any{
			"dashboard": map[string]any{
				"enabled":      true,
				"auth_enabled": true,
			},
			"listeners": map[string]any{
				"dashboard": "127.0.0.1:9090",
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from config, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("config endpoint should not leak secrets, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"auth_enabled":true`) {
		t.Fatalf("expected sanitized config payload, got %q", rec.Body.String())
	}
}

func TestDashboardProbeAndBenchListsReturnSummaries(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbe("probe-1", probe.Result{
		ID:       "probe-1",
		Target:   "https://example.com",
		Status:   http.StatusOK,
		Proto:    "HTTP/2.0",
		Duration: 150 * time.Millisecond,
		Analysis: map[string]any{
			"latency":          map[string]any{"p95": 12.5},
			"support_summary":  probe.SupportSummary{RequestedTests: 3, Available: 1, NotRun: 1, Unavailable: 1, Observed: 1, CoverageRatio: 0.33},
			"fidelity_summary": probe.FidelitySummary{Partial: []string{"qpack"}, Observed: []string{"version"}, Definitions: map[string]probe.FidelityDefinition{"full": {Label: "full", Description: "direct current-path diagnostics"}, "observed": {Label: "observed", Description: "visible protocol/client-layer observation"}, "partial": {Label: "partial", Description: "heuristic, estimate, or capability-check output"}}, PacketLevel: false, Notice: "advanced probe fields are not all packet-level telemetry"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBench("bench-1", bench.Result{
		ID:          "bench-1",
		Target:      "https://example.com",
		Duration:    time.Second,
		Concurrency: 2,
		Protocols:   []string{"h1", "h2"},
		Summary: bench.Summary{
			Protocols:         2,
			HealthyProtocols:  1,
			DegradedProtocols: 1,
			FailedProtocols:   0,
			BestProtocol:      "h1",
			RiskiestProtocol:  "h2",
		},
		Stats: map[string]bench.Stats{
			"h1": {Requests: 10, Latency: bench.Percentiles{P95: 9.5}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from probes list, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"target":"https://example.com"`) || !strings.Contains(rec.Body.String(), `"latency"`) || !strings.Contains(rec.Body.String(), `"fidelity_summary"`) || !strings.Contains(rec.Body.String(), `"packet_level":false`) {
		t.Fatalf("expected enriched probe summary payload, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/benches", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from benches list, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"protocols":["h1","h2"]`) || !strings.Contains(rec.Body.String(), `"latency_ms"`) || !strings.Contains(rec.Body.String(), `"summary"`) || !strings.Contains(rec.Body.String(), `"stats_view"`) {
		t.Fatalf("expected enriched bench summary payload, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes?view=summary", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from summary probes list, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"analysis":{"`) || !strings.Contains(rec.Body.String(), `"analysis_view"`) {
		t.Fatalf("expected summary probe payload to omit raw analysis and keep analysis_view, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/benches?view=summary", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from summary benches list, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"stats":null`) || !strings.Contains(rec.Body.String(), `"stats_view"`) {
		t.Fatalf("expected summary bench payload to omit raw stats and keep stats_view, got %q", rec.Body.String())
	}
}

func TestDashboardListQueryFilteringSortingAndLimit(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbe("probe-a", probe.Result{
		ID:       "probe-a",
		Target:   "https://alpha.example.com",
		Status:   http.StatusOK,
		Proto:    "HTTP/2.0",
		Duration: 120 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := store.SaveProbe("probe-b", probe.Result{
		ID:       "probe-b",
		Target:   "https://beta.example.com",
		Status:   http.StatusBadGateway,
		Proto:    "HTTP/3.0",
		Duration: 240 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBench("bench-a", bench.Result{
		ID:          "bench-a",
		Target:      "https://alpha.example.com",
		Duration:    time.Second,
		Concurrency: 2,
		Protocols:   []string{"h1", "h2"},
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := store.SaveBench("bench-b", bench.Result{
		ID:          "bench-b",
		Target:      "https://beta.example.com",
		Duration:    time.Second,
		Concurrency: 8,
		Protocols:   []string{"h3"},
	}); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes?q=beta&sort=status_desc&limit=1", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from probes query, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"probe-b"`) || strings.Contains(rec.Body.String(), `"probe-a"`) {
		t.Fatalf("expected filtered probe list, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("X-Total-Count"); got != "1" {
		t.Fatalf("expected filtered total count header, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/benches?q=h3&sort=concurrency_desc&limit=1", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from benches query, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"bench-b"`) || strings.Contains(rec.Body.String(), `"bench-a"`) {
		t.Fatalf("expected filtered bench list, got %q", rec.Body.String())
	}
}

func TestDashboardListQueryOffsetPagination(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id     string
		target string
	}{
		{"probe-a", "https://alpha.example.com"},
		{"probe-b", "https://beta.example.com"},
		{"probe-c", "https://gamma.example.com"},
	} {
		if err := store.SaveProbe(item.id, probe.Result{
			ID:       item.id,
			Target:   item.target,
			Status:   http.StatusOK,
			Proto:    "HTTP/2.0",
			Duration: 100 * time.Millisecond,
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	srv := New("127.0.0.1:0", store, Options{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes?sort=target_asc&limit=1&offset=1", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from paginated probes query, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"probe-b"`) || strings.Contains(rec.Body.String(), `"probe-a"`) || strings.Contains(rec.Body.String(), `"probe-c"`) {
		t.Fatalf("expected middle paginated probe item, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("X-Total-Count"); got != "3" {
		t.Fatalf("expected total count 3, got %q", got)
	}
	if got := rec.Header().Get("X-Page-Offset"); got != "1" {
		t.Fatalf("expected page offset 1, got %q", got)
	}
	if got := rec.Header().Get("X-Page-Limit"); got != "1" {
		t.Fatalf("expected page limit 1, got %q", got)
	}
	if got := rec.Header().Get("X-Has-More"); got != "true" {
		t.Fatalf("expected has more true, got %q", got)
	}
	if got := rec.Header().Get("X-Next-Offset"); got != "2" {
		t.Fatalf("expected next offset 2, got %q", got)
	}
}

func TestDashboardListUsesCachedProbeSummary(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbe("probe-cache", probe.Result{
		ID:       "probe-cache",
		Target:   "https://cache.example.com",
		Status:   http.StatusOK,
		Proto:    "HTTP/2.0",
		Duration: 100 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{})
	items, err := store.List("probes")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one stored probe item, got %d", len(items))
	}
	summary, err := srv.probeSummary(items[0])
	if err != nil || summary.ID != "probe-cache" {
		t.Fatalf("expected initial cached probe summary, got summary=%#v err=%v", summary, err)
	}
	if err := os.Remove(items[0].Path); err != nil {
		t.Fatal(err)
	}
	summary, err = srv.probeSummary(items[0])
	if err != nil || summary.ID != "probe-cache" {
		t.Fatalf("expected cached probe summary to survive repeated lookup, got summary=%#v err=%v", summary, err)
	}
}

func TestDashboardListCacheInvalidatesWhenUnderlyingItemsChange(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbe("probe-cache-list", probe.Result{
		ID:       "probe-cache-list",
		Target:   "https://cache-list.example.com",
		Status:   http.StatusOK,
		Proto:    "HTTP/2.0",
		Duration: 100 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes?q=cache&sort=target_asc", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"probe-cache-list"`) {
		t.Fatalf("expected initial cached list response, got code=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(srv.probeListCache) == 0 {
		t.Fatal("expected probe list cache to be populated")
	}

	items, err := store.List("probes")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one stored probe item, got %d", len(items))
	}
	if err := os.Remove(items[0].Path); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes?q=cache&sort=target_asc", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after underlying delete, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"probe-cache-list"`) {
		t.Fatalf("expected stale probe list cache to be invalidated, got %q", rec.Body.String())
	}
}

func TestDashboardProbeListPrefersPersistedSummary(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result := probe.Result{
		ID:        "probe-sidecar",
		Target:    "https://sidecar.example.com",
		Timestamp: time.Now().UTC(),
		Status:    http.StatusOK,
		Proto:     "HTTP/3.0",
		Duration:  150 * time.Millisecond,
		Analysis: map[string]any{
			"fidelity_summary": probe.FidelitySummary{Observed: []string{"version"}, PacketLevel: false},
		},
	}
	if err := store.SaveProbe(result.ID, result); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbeSummary(result.ID, BuildProbeSummary(result)); err != nil {
		t.Fatal(err)
	}

	items, err := store.List("probes")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one stored probe item, got %d", len(items))
	}
	if err := os.WriteFile(items[0].Path, []byte("not-gzip"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from probes list with sidecar summary, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"probe-sidecar"`) || !strings.Contains(rec.Body.String(), `sidecar.example.com`) {
		t.Fatalf("expected persisted summary-backed probe list, got %q", rec.Body.String())
	}
}

func TestDashboardAssetsAndHead(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("unexpected root asset response: code=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodHead, "/assets/app.js", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD asset, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `id="config"`) {
		t.Fatalf("expected config card in dashboard html, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="overview"`) {
		t.Fatalf("expected overview card in dashboard html, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="compare"`) {
		t.Fatalf("expected compare card in dashboard html, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="probe-detail"`) || !strings.Contains(rec.Body.String(), `id="bench-detail"`) || !strings.Contains(rec.Body.String(), `id="trace-detail"`) {
		t.Fatalf("expected detail panels in dashboard html, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `class="stack"`) {
		t.Fatalf("expected stack containers in dashboard html, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for app.js, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "renderProbes") || !strings.Contains(rec.Body.String(), "renderBenches") || !strings.Contains(rec.Body.String(), "renderTraces") || !strings.Contains(rec.Body.String(), "renderProbeDetail") || !strings.Contains(rec.Body.String(), "renderBenchDetail") || !strings.Contains(rec.Body.String(), "renderTraceDetail") || !strings.Contains(rec.Body.String(), "renderOverview") || !strings.Contains(rec.Body.String(), "renderCompare") {
		t.Fatalf("expected typed dashboard renderers in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Real HTTP/3 (quic-go)") || !strings.Contains(rec.Body.String(), "Experimental UDP H3 (lab)") {
		t.Fatalf("expected explicit transport-plane labels in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "skipped") || !strings.Contains(rec.Body.String(), "Top Error") {
		t.Fatalf("expected richer probe/bench renderer hints in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Bench summary") || !strings.Contains(rec.Body.String(), "Healthy") || !strings.Contains(rec.Body.String(), "loadOverview") || !strings.Contains(rec.Body.String(), "loadCompare") {
		t.Fatalf("expected bench summary renderer hints in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "0rtt") || !strings.Contains(rec.Body.String(), "migration") || !strings.Contains(rec.Body.String(), "qpack") || !strings.Contains(rec.Body.String(), "loss") || !strings.Contains(rec.Body.String(), "congestion") || !strings.Contains(rec.Body.String(), "version") || !strings.Contains(rec.Body.String(), "retry") || !strings.Contains(rec.Body.String(), "ecn") || !strings.Contains(rec.Body.String(), "spin") || !strings.Contains(rec.Body.String(), "Coverage") || !strings.Contains(rec.Body.String(), "Support summary") {
		t.Fatalf("expected advanced probe renderer hints in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Fidelity legend") || !strings.Contains(rec.Body.String(), "visible protocol/client-layer observation") || !strings.Contains(rec.Body.String(), "heuristic, estimate, or capability-check output") {
		t.Fatalf("expected fidelity legend renderer hints in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "supportPills") || !strings.Contains(rec.Body.String(), ".coverage") {
		t.Fatalf("expected support coverage renderer hints in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "record-action") || !strings.Contains(rec.Body.String(), "loadDetail") || !strings.Contains(rec.Body.String(), "ensureDetailSelection") || !strings.Contains(rec.Body.String(), "trace-detail") || !strings.Contains(rec.Body.String(), "Open raw trace") {
		t.Fatalf("expected detail panel renderer hints in app.js, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "loadCollection") || !strings.Contains(rec.Body.String(), "X-Total-Count") || !strings.Contains(rec.Body.String(), "pager") {
		t.Fatalf("expected pagination renderer hints in app.js, got %q", rec.Body.String())
	}
}

func TestDashboardTraceErrorsAndNotFound(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/missing.sqlog", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without trace dir, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"error"`) {
		t.Fatalf("expected JSON trace error, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/not-a-trace.txt", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid extension, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/traces", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for traces method violation, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"method not allowed"`) {
		t.Fatalf("expected JSON method error, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown api route, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"api route not found"`) {
		t.Fatalf("expected API not found payload, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/meta/missing.sqlog", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing trace metadata, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"trace not found"`) {
		t.Fatalf("expected trace metadata error payload, got %q", rec.Body.String())
	}
}

func TestDashboardProbeAndBenchDetailsNotFound(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	for _, path := range []string{"/api/v1/probes/missing", "/api/v1/benches/missing"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"status":"error"`) {
			t.Fatalf("expected JSON error for %s, got %q", path, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"detail":"see server logs"`) {
			t.Fatalf("expected sanitized error detail for %s, got %q", path, rec.Body.String())
		}
	}
}
