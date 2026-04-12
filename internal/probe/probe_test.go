package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/testutil"
)

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
		Timeout:        2 * time.Second,
		Insecure:       true,
		DefaultStreams: 4,
	})
	if err != nil {
		t.Fatalf("expected insecure probe to succeed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.TLS["cipher"] == "" {
		t.Fatalf("expected TLS metadata, got %#v", result.TLS)
	}
	if result.TLS["version"] == "" || result.TLS["leaf_cert"] == nil {
		t.Fatalf("expected rich TLS metadata, got %#v", result.TLS)
	}
	analysis, ok := result.Analysis["alt_svc"].(map[string]any)
	if !ok {
		t.Fatalf("expected alt_svc analysis, got %#v", result.Analysis)
	}
	if analysis["present"] != true {
		t.Fatalf("expected alt_svc present=true, got %#v", analysis)
	}
	response, ok := result.Analysis["response"].(map[string]any)
	if !ok {
		t.Fatalf("expected response analysis, got %#v", result.Analysis)
	}
	if response["body_bytes"] != int64(5) {
		t.Fatalf("expected body_bytes=5, got %#v", response["body_bytes"])
	}
	latency, ok := result.Analysis["latency"].(map[string]any)
	if !ok {
		t.Fatalf("expected latency analysis, got %#v", result.Analysis)
	}
	if latency["samples"].(int) == 0 {
		t.Fatalf("expected latency samples, got %#v", latency)
	}
	testPlan, ok := result.Analysis["test_plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected test plan, got %#v", result.Analysis)
	}
	requested, ok := testPlan["requested"].([]string)
	if !ok || len(requested) == 0 {
		t.Fatalf("expected requested tests, got %#v", testPlan)
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
	streams, ok := result.Analysis["streams"].(map[string]any)
	if !ok {
		t.Fatalf("expected stream analysis, got %#v", result.Analysis)
	}
	if streams["attempted"].(int) != 4 {
		t.Fatalf("expected attempted=4, got %#v", streams)
	}
	if _, ok := result.Analysis["qpack"].(map[string]any); ok {
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
		Timeout:        2 * time.Second,
		Insecure:       true,
		TraceDir:       traceDir,
		DefaultStreams: 3,
		DefaultTests:   []string{"handshake", "tls", "latency", "throughput", "streams", "ecn", "spin-bit", "version", "retry", "qpack", "loss", "congestion", "0rtt", "migration"},
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
	if result.TLS["alpn"] != "h3" {
		t.Fatalf("expected h3 ALPN, got %#v", result.TLS)
	}
	if result.TLS["version"] != "TLS1.3" {
		t.Fatalf("expected TLS1.3 metadata, got %#v", result.TLS)
	}
	response, ok := result.Analysis["response"].(map[string]any)
	if !ok {
		t.Fatalf("expected response analysis, got %#v", result.Analysis)
	}
	if response["status_class"] != 2 {
		t.Fatalf("expected 2xx status class, got %#v", response["status_class"])
	}
	latency, ok := result.Analysis["latency"].(map[string]any)
	if !ok {
		t.Fatalf("expected latency analysis, got %#v", result.Analysis)
	}
	if latency["p95"].(float64) < 0 {
		t.Fatalf("expected non-negative latency p95, got %#v", latency)
	}
	testPlan, ok := result.Analysis["test_plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected test plan, got %#v", result.Analysis)
	}
	executed, ok := testPlan["executed"].([]string)
	if !ok || len(executed) == 0 {
		t.Fatalf("expected executed tests, got %#v", testPlan)
	}
	zeroRTT, ok := result.Analysis["0rtt"].(map[string]any)
	if !ok {
		t.Fatalf("expected 0rtt analysis, got %#v", result.Analysis)
	}
	if zeroRTT["mode"] != "tls-resumption-check" {
		t.Fatalf("expected resumption mode note, got %#v", zeroRTT)
	}
	support, ok := result.Analysis["support"].(map[string]any)
	if !ok {
		t.Fatalf("expected support summary, got %#v", result.Analysis)
	}
	supportSummary, ok := result.Analysis["support_summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected support rollup, got %#v", result.Analysis)
	}
	if supportSummary["requested_tests"].(int) < 1 || supportSummary["available"].(int) < 1 {
		t.Fatalf("expected support rollup counts, got %#v", supportSummary)
	}
	zeroRTTSupport, ok := support["0rtt"].(map[string]any)
	if !ok || zeroRTTSupport["coverage"] != "partial" {
		t.Fatalf("expected partial 0rtt support summary, got %#v", support)
	}
	qpack, ok := result.Analysis["qpack"].(map[string]any)
	if !ok {
		t.Fatalf("expected qpack analysis, got %#v", result.Analysis)
	}
	if qpack["mode"] != "header-block-estimate" {
		t.Fatalf("expected qpack estimate mode, got %#v", qpack)
	}
	qpackSupport, ok := support["qpack"].(map[string]any)
	if !ok || qpackSupport["coverage"] != "partial" || qpackSupport["state"] != "available" {
		t.Fatalf("expected partial available qpack support summary, got %#v", support)
	}
	loss, ok := result.Analysis["loss"].(map[string]any)
	if !ok || loss["mode"] != "request-error-signal" {
		t.Fatalf("expected loss analysis, got %#v", result.Analysis)
	}
	lossSupport, ok := support["loss"].(map[string]any)
	if !ok || lossSupport["coverage"] != "partial" || lossSupport["state"] != "available" {
		t.Fatalf("expected partial available loss support summary, got %#v", support)
	}
	congestion, ok := result.Analysis["congestion"].(map[string]any)
	if !ok || congestion["mode"] != "latency-spread-estimate" {
		t.Fatalf("expected congestion analysis, got %#v", result.Analysis)
	}
	congestionSupport, ok := support["congestion"].(map[string]any)
	if !ok || congestionSupport["coverage"] != "partial" || congestionSupport["state"] != "available" {
		t.Fatalf("expected partial available congestion support summary, got %#v", support)
	}
	version, ok := result.Analysis["version"].(map[string]any)
	if !ok || version["mode"] != "protocol-observation" {
		t.Fatalf("expected version analysis, got %#v", result.Analysis)
	}
	versionSupport, ok := support["version"].(map[string]any)
	if !ok || versionSupport["coverage"] != "partial" || versionSupport["state"] != "available" {
		t.Fatalf("expected partial available version support summary, got %#v", support)
	}
	retry, ok := result.Analysis["retry"].(map[string]any)
	if !ok || retry["mode"] != "handshake-observation" {
		t.Fatalf("expected retry analysis, got %#v", result.Analysis)
	}
	retrySupport, ok := support["retry"].(map[string]any)
	if !ok || retrySupport["coverage"] != "partial" || retrySupport["state"] != "available" {
		t.Fatalf("expected partial available retry support summary, got %#v", support)
	}
	ecn, ok := result.Analysis["ecn"].(map[string]any)
	if !ok || ecn["mode"] != "metadata-observation" {
		t.Fatalf("expected ecn analysis, got %#v", result.Analysis)
	}
	ecnSupport, ok := support["ecn"].(map[string]any)
	if !ok || ecnSupport["coverage"] != "partial" || ecnSupport["state"] != "available" {
		t.Fatalf("expected partial available ecn support summary, got %#v", support)
	}
	spin, ok := result.Analysis["spin-bit"].(map[string]any)
	if !ok || spin["mode"] != "rtt-sampling-estimate" {
		t.Fatalf("expected spin-bit analysis, got %#v", result.Analysis)
	}
	spinSupport, ok := support["spin-bit"].(map[string]any)
	if !ok || spinSupport["coverage"] != "partial" || spinSupport["state"] != "available" {
		t.Fatalf("expected partial available spin-bit support summary, got %#v", support)
	}
	migration, ok := result.Analysis["migration"].(map[string]any)
	if !ok {
		t.Fatalf("expected migration analysis, got %#v", result.Analysis)
	}
	if migration["mode"] != "endpoint-capability-check" {
		t.Fatalf("expected migration endpoint check mode, got %#v", migration)
	}
	if migration["supported"] != false {
		t.Fatalf("expected migration supported=false from endpoint contract, got %#v", migration)
	}
	if migration["message"] == "" {
		t.Fatalf("expected migration message from endpoint contract, got %#v", migration)
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
		Timeout:      2 * time.Second,
		Insecure:     true,
		DefaultTests: []string{"latency", "0rtt", "migration"},
	})
	if err != nil {
		t.Fatalf("expected probe to succeed: %v", err)
	}
	testPlan, ok := result.Analysis["test_plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected test plan, got %#v", result.Analysis)
	}
	skipped, ok := testPlan["skipped"].([]map[string]any)
	if !ok || len(skipped) < 1 {
		t.Fatalf("expected skipped unsupported tests, got %#v", testPlan)
	}
	if _, ok := result.Analysis["migration"].(map[string]any); !ok {
		t.Fatalf("expected migration capability analysis, got %#v", result.Analysis)
	}
	migration := result.Analysis["migration"].(map[string]any)
	if migration["supported"] != false {
		t.Fatalf("expected migration supported=false from endpoint contract, got %#v", migration)
	}
	support, ok := result.Analysis["support"].(map[string]any)
	if !ok {
		t.Fatalf("expected support summary, got %#v", result.Analysis)
	}
	migrationSupport, ok := support["migration"].(map[string]any)
	if !ok || migrationSupport["coverage"] != "partial" || migrationSupport["state"] != "unavailable" {
		t.Fatalf("expected partial unavailable migration support summary, got %#v", support)
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
	migration, ok := result.Analysis["migration"].(map[string]any)
	if !ok {
		t.Fatalf("expected migration analysis, got %#v", result.Analysis)
	}
	if migration["supported"] != false {
		t.Fatalf("expected migration supported=false from loopback endpoint contract, got %#v", migration)
	}
	if migration["message"] == "" {
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
		Timeout:      2 * time.Second,
		Insecure:     true,
		DefaultTests: []string{"qpack", "loss", "congestion", "retry", "version", "spin-bit", "ecn"},
	})
	if err != nil {
		t.Fatalf("expected probe to succeed: %v", err)
	}
	testPlan, ok := result.Analysis["test_plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected test plan, got %#v", result.Analysis)
	}
	skipped, ok := testPlan["skipped"].([]map[string]any)
	if !ok || len(skipped) < 5 {
		t.Fatalf("expected skipped advanced tests, got %#v", testPlan)
	}
	executed, ok := testPlan["executed"].([]string)
	if !ok || !containsString(executed, "loss") || !containsString(executed, "congestion") {
		t.Fatalf("expected loss and congestion to execute, got %#v", testPlan)
	}
	support, ok := result.Analysis["support"].(map[string]any)
	if !ok {
		t.Fatalf("expected support summary, got %#v", result.Analysis)
	}
	if _, ok := result.Analysis["support_summary"].(map[string]any); !ok {
		t.Fatalf("expected support rollup, got %#v", result.Analysis)
	}
	for _, name := range []string{"qpack", "loss", "congestion", "retry", "version", "spin-bit", "ecn"} {
		entry, ok := support[name].(map[string]any)
		if !ok {
			t.Fatalf("expected support entry for %s, got %#v", name, support)
		}
		switch name {
		case "qpack":
			if entry["coverage"] != "partial" || entry["state"] != "not_run" {
				t.Fatalf("expected partial not_run qpack support entry, got %#v", entry)
			}
		case "retry", "version", "spin-bit", "ecn":
			if entry["coverage"] != "partial" || entry["state"] != "not_run" {
				t.Fatalf("expected partial not_run support entry for %s, got %#v", name, entry)
			}
		case "loss", "congestion":
			if entry["coverage"] != "partial" || entry["state"] != "available" {
				t.Fatalf("expected partial available support entry for %s, got %#v", name, entry)
			}
		default:
			if entry["coverage"] != "unavailable" || entry["state"] != "unavailable" {
				t.Fatalf("expected unavailable support entry for %s, got %#v", name, entry)
			}
		}
	}
}
