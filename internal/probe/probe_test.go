package probe

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/testutil"
)

func supportEntriesFromAnalysis(t *testing.T, analysis map[string]any) map[string]SupportEntry {
	t.Helper()
	support, ok := analysis["support"].(map[string]SupportEntry)
	if !ok {
		t.Fatalf("expected typed support summary, got %#v", analysis)
	}
	return support
}

func supportSummaryFromAnalysis(t *testing.T, analysis map[string]any) SupportSummary {
	t.Helper()
	summary, ok := analysis["support_summary"].(SupportSummary)
	if !ok {
		t.Fatalf("expected typed support rollup, got %#v", analysis)
	}
	return summary
}

func testPlanFromAnalysis(t *testing.T, analysis map[string]any) TestPlan {
	t.Helper()
	plan, ok := analysis["test_plan"].(TestPlan)
	if !ok {
		t.Fatalf("expected typed test plan, got %#v", analysis)
	}
	return plan
}

func responseAnalysisFromResult(t *testing.T, result *Result) ResponseAnalysis {
	t.Helper()
	response, ok := result.Analysis["response"].(ResponseAnalysis)
	if !ok {
		t.Fatalf("expected typed response analysis, got %#v", result.Analysis)
	}
	return response
}

func latencyAnalysisFromResult(t *testing.T, result *Result) LatencyAnalysis {
	t.Helper()
	latency, ok := result.Analysis["latency"].(LatencyAnalysis)
	if !ok {
		t.Fatalf("expected typed latency analysis, got %#v", result.Analysis)
	}
	return latency
}

func streamAnalysisFromResult(t *testing.T, result *Result) StreamAnalysis {
	t.Helper()
	streams, ok := result.Analysis["streams"].(StreamAnalysis)
	if !ok {
		t.Fatalf("expected typed stream analysis, got %#v", result.Analysis)
	}
	return streams
}

func tlsMetadataFromResult(t *testing.T, result *Result) TLSMetadata {
	t.Helper()
	meta, ok := result.TLS.(TLSMetadata)
	if !ok {
		t.Fatalf("expected typed TLS metadata, got %#v", result.TLS)
	}
	return meta
}

func altSvcAnalysisFromResult(t *testing.T, result *Result) AltSvcAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["alt_svc"].(AltSvcAnalysis)
	if !ok {
		t.Fatalf("expected typed alt_svc analysis, got %#v", result.Analysis)
	}
	return analysis
}

func qpackAnalysisFromResult(t *testing.T, result *Result) QPACKAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["qpack"].(QPACKAnalysis)
	if !ok {
		t.Fatalf("expected typed qpack analysis, got %#v", result.Analysis)
	}
	return analysis
}

func lossAnalysisFromResult(t *testing.T, result *Result) LossAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["loss"].(LossAnalysis)
	if !ok {
		t.Fatalf("expected typed loss analysis, got %#v", result.Analysis)
	}
	return analysis
}

func congestionAnalysisFromResult(t *testing.T, result *Result) CongestionAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["congestion"].(CongestionAnalysis)
	if !ok {
		t.Fatalf("expected typed congestion analysis, got %#v", result.Analysis)
	}
	return analysis
}

func versionAnalysisFromResult(t *testing.T, result *Result) VersionAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["version"].(VersionAnalysis)
	if !ok {
		t.Fatalf("expected typed version analysis, got %#v", result.Analysis)
	}
	return analysis
}

func retryAnalysisFromResult(t *testing.T, result *Result) RetryAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["retry"].(RetryAnalysis)
	if !ok {
		t.Fatalf("expected typed retry analysis, got %#v", result.Analysis)
	}
	return analysis
}

func ecnAnalysisFromResult(t *testing.T, result *Result) ECNAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["ecn"].(ECNAnalysis)
	if !ok {
		t.Fatalf("expected typed ecn analysis, got %#v", result.Analysis)
	}
	return analysis
}

func spinBitAnalysisFromResult(t *testing.T, result *Result) SpinBitAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["spin-bit"].(SpinBitAnalysis)
	if !ok {
		t.Fatalf("expected typed spin-bit analysis, got %#v", result.Analysis)
	}
	return analysis
}

func zeroRTTAnalysisFromResult(t *testing.T, result *Result) ZeroRTTAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["0rtt"].(ZeroRTTAnalysis)
	if !ok {
		t.Fatalf("expected typed 0rtt analysis, got %#v", result.Analysis)
	}
	return analysis
}

func migrationAnalysisFromResult(t *testing.T, result *Result) MigrationAnalysis {
	t.Helper()
	analysis, ok := result.Analysis["migration"].(MigrationAnalysis)
	if !ok {
		t.Fatalf("expected typed migration analysis, got %#v", result.Analysis)
	}
	return analysis
}

func TestRunHTTPSProbeRespectsTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Alt-Svc", `h3=":443"; ma=86400`)
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	_, err := Run(srv.URL, config.ProbeConfig{Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected certificate verification failure")
	}

	result, err := Run(srv.URL, config.ProbeConfig{
		Timeout:          2 * time.Second,
		Insecure:         true,
		AllowInsecureTLS: true,
		DefaultStreams:   4,
	})
	if err != nil {
		t.Fatalf("expected insecure probe to succeed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	tlsMeta := tlsMetadataFromResult(t, result)
	if tlsMeta.Cipher == "" {
		t.Fatalf("expected TLS metadata, got %#v", result.TLS)
	}
	if tlsMeta.Version == "" || tlsMeta.LeafCert == nil {
		t.Fatalf("expected rich TLS metadata, got %#v", result.TLS)
	}
	analysis := altSvcAnalysisFromResult(t, result)
	if !analysis.Present {
		t.Fatalf("expected alt_svc present=true, got %#v", analysis)
	}
	response := responseAnalysisFromResult(t, result)
	if response.BodyBytes != int64(5) {
		t.Fatalf("expected body_bytes=5, got %#v", response.BodyBytes)
	}
	latency := latencyAnalysisFromResult(t, result)
	if latency.Samples == 0 {
		t.Fatalf("expected latency samples, got %#v", latency)
	}
	testPlan := testPlanFromAnalysis(t, result.Analysis)
	requested := testPlan.Requested
	if len(requested) == 0 {
		t.Fatalf("expected requested tests, got %#v", testPlan)
	}
}

func TestDoStandardRequestBodyRejectsOversizedBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("a", int(maxProbeResponseBodyBytes)+1)))
	}))
	defer srv.Close()

	status, body, err := doStandardRequestBody(&http.Client{Timeout: 2 * time.Second}, srv.URL)
	if err == nil {
		t.Fatal("expected oversized response body error")
	}
	if status != http.StatusOK {
		t.Fatalf("expected status=%d, got %d", http.StatusOK, status)
	}
	if body != nil {
		t.Fatalf("expected nil body on oversized response, got %d bytes", len(body))
	}
	if !strings.Contains(err.Error(), "response body exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLoopbackProbe(t *testing.T) {
	result, err := Run("triton://loopback/ping", config.ProbeConfig{Timeout: 2 * time.Second, DefaultStreams: 4})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.Proto != "HTTP/3-loopback" {
		t.Fatalf("unexpected proto: %q", result.Proto)
	}
	streams := streamAnalysisFromResult(t, result)
	if streams.Attempted != 4 {
		t.Fatalf("expected attempted=4, got %#v", streams)
	}
	if _, ok := result.Analysis["qpack"].(QPACKAnalysis); ok {
		t.Fatalf("did not expect qpack analysis unless requested, got %#v", result.Analysis)
	}
}

func TestRunStandardH3Probe(t *testing.T) {
	addr, shutdown := testutil.StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "h3")
		w.WriteHeader(http.StatusCreated)
	}))
	defer shutdown()
	traceDir := t.TempDir()

	_, err := Run("h3://"+addr+"/ping", config.ProbeConfig{Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected certificate verification failure")
	}

	result, err := Run("h3://"+addr+"/ping", config.ProbeConfig{
		Timeout:          2 * time.Second,
		Insecure:         true,
		AllowInsecureTLS: true,
		TraceDir:         traceDir,
		DefaultStreams:   3,
		DefaultTests:     []string{"handshake", "tls", "latency", "throughput", "streams", "ecn", "spin-bit", "version", "retry", "qpack", "loss", "congestion", "0rtt", "migration"},
	})
	if err != nil {
		t.Fatalf("expected h3 probe to succeed: %v", err)
	}
	if result.Status != http.StatusCreated {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.Proto == "" {
		t.Fatal("expected protocol string")
	}
	tlsMeta := tlsMetadataFromResult(t, result)
	if tlsMeta.ALPN != "h3" {
		t.Fatalf("expected h3 ALPN, got %#v", result.TLS)
	}
	if tlsMeta.Version != "TLS1.3" {
		t.Fatalf("expected TLS1.3 metadata, got %#v", result.TLS)
	}
	response := responseAnalysisFromResult(t, result)
	if response.StatusClass != 2 {
		t.Fatalf("expected 2xx status class, got %#v", response.StatusClass)
	}
	latency := latencyAnalysisFromResult(t, result)
	if latency.P95 < 0 {
		t.Fatalf("expected non-negative latency p95, got %#v", latency)
	}
	testPlan := testPlanFromAnalysis(t, result.Analysis)
	executed := testPlan.Executed
	if len(executed) == 0 {
		t.Fatalf("expected executed tests, got %#v", testPlan)
	}
	zeroRTT := zeroRTTAnalysisFromResult(t, result)
	if zeroRTT.Mode != "tls-resumption-check" {
		t.Fatalf("expected resumption mode note, got %#v", zeroRTT)
	}
	support := supportEntriesFromAnalysis(t, result.Analysis)
	supportSummary := supportSummaryFromAnalysis(t, result.Analysis)
	if supportSummary.RequestedTests < 1 || supportSummary.Available < 1 {
		t.Fatalf("expected support rollup counts, got %#v", supportSummary)
	}
	zeroRTTSupport, ok := support["0rtt"]
	if !ok || zeroRTTSupport.Coverage != "partial" {
		t.Fatalf("expected partial 0rtt support summary, got %#v", support)
	}
	qpack := qpackAnalysisFromResult(t, result)
	if qpack.Mode != "header-block-estimate" {
		t.Fatalf("expected qpack estimate mode, got %#v", qpack)
	}
	qpackSupport, ok := support["qpack"]
	if !ok || qpackSupport.Coverage != "partial" || qpackSupport.State != "available" {
		t.Fatalf("expected partial available qpack support summary, got %#v", support)
	}
	loss := lossAnalysisFromResult(t, result)
	if loss.Mode != "request-error-signal" {
		t.Fatalf("expected loss analysis, got %#v", result.Analysis)
	}
	lossSupport, ok := support["loss"]
	if !ok || lossSupport.Coverage != "partial" || lossSupport.State != "available" {
		t.Fatalf("expected partial available loss support summary, got %#v", support)
	}
	congestion := congestionAnalysisFromResult(t, result)
	if congestion.Mode != "latency-spread-estimate" {
		t.Fatalf("expected congestion analysis, got %#v", result.Analysis)
	}
	congestionSupport, ok := support["congestion"]
	if !ok || congestionSupport.Coverage != "partial" || congestionSupport.State != "available" {
		t.Fatalf("expected partial available congestion support summary, got %#v", support)
	}
	version := versionAnalysisFromResult(t, result)
	if version.Mode != "protocol-observation" {
		t.Fatalf("expected version analysis, got %#v", result.Analysis)
	}
	versionSupport, ok := support["version"]
	if !ok || versionSupport.Coverage != "partial" || versionSupport.State != "available" {
		t.Fatalf("expected partial available version support summary, got %#v", support)
	}
	retry := retryAnalysisFromResult(t, result)
	if retry.Mode != "handshake-observation" {
		t.Fatalf("expected retry analysis, got %#v", result.Analysis)
	}
	retrySupport, ok := support["retry"]
	if !ok || retrySupport.Coverage != "partial" || retrySupport.State != "available" {
		t.Fatalf("expected partial available retry support summary, got %#v", support)
	}
	ecn := ecnAnalysisFromResult(t, result)
	if ecn.Mode != "metadata-observation" {
		t.Fatalf("expected ecn analysis, got %#v", result.Analysis)
	}
	ecnSupport, ok := support["ecn"]
	if !ok || ecnSupport.Coverage != "partial" || ecnSupport.State != "available" {
		t.Fatalf("expected partial available ecn support summary, got %#v", support)
	}
	spin := spinBitAnalysisFromResult(t, result)
	if spin.Mode != "rtt-sampling-estimate" {
		t.Fatalf("expected spin-bit analysis, got %#v", result.Analysis)
	}
	spinSupport, ok := support["spin-bit"]
	if !ok || spinSupport.Coverage != "partial" || spinSupport.State != "available" {
		t.Fatalf("expected partial available spin-bit support summary, got %#v", support)
	}
	migration := migrationAnalysisFromResult(t, result)
	if migration.Mode != "endpoint-capability-check" {
		t.Fatalf("expected migration endpoint check mode, got %#v", migration)
	}
	if migration.Supported != false {
		t.Fatalf("expected migration supported=false from endpoint contract, got %#v", migration)
	}
	if migration.Message == "" && migration.Error == "" {
		t.Fatalf("expected migration message or error detail, got %#v", migration)
	}
	if len(result.TraceFiles) == 0 {
		t.Fatal("expected trace files to be linked in result")
	}
	hasTrace, err := observability.HasQLOGFiles(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTrace {
		t.Fatal("expected client qlog file")
	}
}

func TestRunProbeReportsSkippedUnsupportedTests(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/migration-test" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"supported":false,"message":"migration unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	result, err := Run(srv.URL, config.ProbeConfig{
		Timeout:          2 * time.Second,
		Insecure:         true,
		AllowInsecureTLS: true,
		DefaultTests:     []string{"latency", "0rtt", "migration"},
	})
	if err != nil {
		t.Fatalf("expected probe to succeed: %v", err)
	}
	testPlan := testPlanFromAnalysis(t, result.Analysis)
	skipped := testPlan.Skipped
	if len(skipped) < 1 {
		t.Fatalf("expected skipped unsupported tests, got %#v", testPlan)
	}
	migration := migrationAnalysisFromResult(t, result)
	if migration.Supported != false {
		t.Fatalf("expected migration supported=false from endpoint contract, got %#v", migration)
	}
	support := supportEntriesFromAnalysis(t, result.Analysis)
	migrationSupport, ok := support["migration"]
	if !ok || migrationSupport.Coverage != "partial" || migrationSupport.State != "available" {
		t.Fatalf("expected partial available migration support summary, got %#v", support)
	}
}

func TestRunLoopbackProbeMigrationReadsContractBody(t *testing.T) {
	result, err := Run("triton://loopback/ping", config.ProbeConfig{
		Timeout:      2 * time.Second,
		DefaultTests: []string{"migration"},
	})
	if err != nil {
		t.Fatal(err)
	}
	migration := migrationAnalysisFromResult(t, result)
	if migration.Supported != false {
		t.Fatalf("expected migration supported=false from loopback endpoint contract, got %#v", migration)
	}
	if migration.Message == "" {
		t.Fatalf("expected migration message from loopback endpoint contract, got %#v", migration)
	}
}

func TestRunProbeSupportMatrixReportsUnavailableAdvancedTests(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	result, err := Run(srv.URL, config.ProbeConfig{
		Timeout:          2 * time.Second,
		Insecure:         true,
		AllowInsecureTLS: true,
		DefaultTests:     []string{"qpack", "loss", "congestion", "retry", "version", "spin-bit", "ecn"},
	})
	if err != nil {
		t.Fatalf("expected probe to succeed: %v", err)
	}
	testPlan := testPlanFromAnalysis(t, result.Analysis)
	skipped := testPlan.Skipped
	if len(skipped) < 5 {
		t.Fatalf("expected skipped advanced tests, got %#v", testPlan)
	}
	executed := testPlan.Executed
	if !containsString(executed, "loss") || !containsString(executed, "congestion") {
		t.Fatalf("expected loss and congestion to execute, got %#v", testPlan)
	}
	support := supportEntriesFromAnalysis(t, result.Analysis)
	_ = supportSummaryFromAnalysis(t, result.Analysis)
	for _, name := range []string{"qpack", "loss", "congestion", "retry", "version", "spin-bit", "ecn"} {
		entry, ok := support[name]
		if !ok {
			t.Fatalf("expected support entry for %s, got %#v", name, support)
		}
		switch name {
		case "qpack":
			if entry.Coverage != "partial" || entry.State != "not_run" {
				t.Fatalf("expected partial not_run qpack support entry, got %#v", entry)
			}
		case "retry", "version", "spin-bit", "ecn":
			if entry.Coverage != "partial" || entry.State != "not_run" {
				t.Fatalf("expected partial not_run support entry for %s, got %#v", name, entry)
			}
		case "loss", "congestion":
			if entry.Coverage != "partial" || entry.State != "available" {
				t.Fatalf("expected partial available support entry for %s, got %#v", name, entry)
			}
		default:
			if entry.Coverage != "unavailable" || entry.State != "unavailable" {
				t.Fatalf("expected unavailable support entry for %s, got %#v", name, entry)
			}
		}
	}
}

func TestRunProbeRejectsInsecureTLSWithoutOptIn(t *testing.T) {
	if _, err := Run("https://example.com", config.ProbeConfig{
		Timeout:  time.Second,
		Insecure: true,
	}); err == nil {
		t.Fatal("expected insecure probe without opt-in to fail")
	}
}
