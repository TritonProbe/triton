package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/probe"
)

const (
	maxDashboardBenchDuration    = 10 * time.Second
	maxDashboardBenchConcurrency = 128
	maxDashboardProbeTimeout     = 30 * time.Second
	maxDashboardProbeStreams     = 64
)

type benchActionRequest struct {
	Target      string   `json:"target"`
	Protocols   []string `json:"protocols"`
	Duration    string   `json:"duration"`
	Concurrency int      `json:"concurrency"`
	InsecureTLS bool     `json:"insecure_tls"`
}

type benchActionResponse struct {
	Status  string       `json:"status"`
	Message string       `json:"message"`
	Result  bench.Result `json:"result"`
	Summary BenchSummary `json:"summary"`
}

type probeActionRequest struct {
	Target      string   `json:"target"`
	Tests       []string `json:"tests"`
	Timeout     string   `json:"timeout"`
	Streams     int      `json:"streams"`
	InsecureTLS bool     `json:"insecure_tls"`
}

type probeActionResponse struct {
	Status  string       `json:"status"`
	Message string       `json:"message"`
	Result  probe.Result `json:"result"`
	Summary ProbeSummary `json:"summary"`
}

func (s *Server) handleBenchAction(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	decoder.DisallowUnknownFields()

	var req benchActionRequest
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid bench request", err)
		return
	}

	target, err := normalizeBenchActionTarget(req.Target)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	protocols, err := normalizeBenchActionProtocols(req.Protocols, s.benchDefaults.DefaultProtocols)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	duration, err := normalizeBenchActionDuration(req.Duration, s.benchDefaults.DefaultDuration)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	concurrency, err := normalizeBenchActionConcurrency(req.Concurrency, s.benchDefaults.DefaultConcurrency)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	cfg := s.benchDefaults
	cfg.Warmup = 0
	cfg.DefaultDuration = duration
	cfg.DefaultConcurrency = concurrency
	cfg.DefaultProtocols = protocols
	cfg.TraceDir = s.trace
	cfg.Insecure = req.InsecureTLS
	cfg.AllowInsecureTLS = req.InsecureTLS

	result, err := bench.Run(target, cfg)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("bench failed: %v", err), nil)
		return
	}
	if err := s.store.SaveBench(result.ID, result); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to save bench result", err)
		return
	}
	summary := BuildBenchSummary(*result)
	if err := s.store.SaveBenchSummary(result.ID, summary); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to save bench summary", err)
		return
	}

	writeJSON(w, benchActionResponse{
		Status:  "ok",
		Message: "bench completed",
		Result:  *result,
		Summary: summary,
	})
}

func (s *Server) handleProbeAction(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	decoder.DisallowUnknownFields()

	var req probeActionRequest
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid probe request", err)
		return
	}

	target, err := normalizeBenchActionTarget(req.Target)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tests := normalizeProbeActionTests(req.Tests, s.probeDefaults.DefaultTests)
	timeout, err := normalizeProbeActionTimeout(req.Timeout, s.probeDefaults.Timeout)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	streams, err := normalizeProbeActionStreams(req.Streams, s.probeDefaults.DefaultStreams)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	cfg := s.probeDefaults
	cfg.Timeout = timeout
	cfg.DefaultTests = tests
	cfg.DefaultStreams = streams
	cfg.TraceDir = s.trace
	cfg.Insecure = req.InsecureTLS
	cfg.AllowInsecureTLS = req.InsecureTLS

	result, err := probe.Run(target, cfg)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("probe failed: %v", err), nil)
		return
	}
	if err := s.store.SaveProbe(result.ID, result); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to save probe result", err)
		return
	}
	summary := BuildProbeSummary(*result)
	if err := s.store.SaveProbeSummary(result.ID, summary); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to save probe summary", err)
		return
	}

	writeJSON(w, probeActionResponse{
		Status:  "ok",
		Message: "probe completed",
		Result:  *result,
		Summary: summary,
	})
}

func normalizeBenchActionTarget(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("target is invalid")
	}
	switch parsed.Scheme {
	case "http", "https", "h3", "triton":
	default:
		return "", fmt.Errorf("target scheme must be http, https, h3, or triton")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("target host is required")
	}
	return parsed.String(), nil
}

func normalizeBenchActionProtocols(requested, defaults []string) ([]string, error) {
	values := requested
	if len(values) == 0 {
		values = defaults
	}
	if len(values) == 0 {
		values = []string{"h1", "h2"}
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		protocol := strings.ToLower(strings.TrimSpace(value))
		if protocol == "" {
			continue
		}
		switch protocol {
		case "h1", "h2", "h3":
		default:
			return nil, fmt.Errorf("unsupported protocol %q", value)
		}
		if _, ok := seen[protocol]; ok {
			continue
		}
		seen[protocol] = struct{}{}
		out = append(out, protocol)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one protocol is required")
	}
	return out, nil
}

func normalizeBenchActionDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		if fallback <= 0 {
			return 3 * time.Second, nil
		}
		return fallback, nil
	}
	duration, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("duration is invalid")
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	if duration > maxDashboardBenchDuration {
		return 0, fmt.Errorf("duration must be %s or less", maxDashboardBenchDuration)
	}
	return duration, nil
}

func normalizeBenchActionConcurrency(value, fallback int) (int, error) {
	if value == 0 {
		if fallback <= 0 {
			return 4, nil
		}
		return fallback, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("concurrency must be positive")
	}
	if value > maxDashboardBenchConcurrency {
		return 0, fmt.Errorf("concurrency must be %d or less", maxDashboardBenchConcurrency)
	}
	return value, nil
}

func normalizeProbeActionTests(requested, defaults []string) []string {
	values := requested
	if len(values) == 0 {
		values = defaults
	}
	if len(values) == 0 {
		values = []string{"handshake", "tls", "latency", "throughput", "streams", "alt-svc"}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		test := strings.ToLower(strings.TrimSpace(value))
		if test == "" {
			continue
		}
		if _, ok := seen[test]; ok {
			continue
		}
		seen[test] = struct{}{}
		out = append(out, test)
	}
	if len(out) == 0 {
		return []string{"handshake", "tls", "latency", "throughput", "streams", "alt-svc"}
	}
	return out
}

func normalizeProbeActionTimeout(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		if fallback <= 0 {
			return 10 * time.Second, nil
		}
		return fallback, nil
	}
	timeout, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("timeout is invalid")
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("timeout must be positive")
	}
	if timeout > maxDashboardProbeTimeout {
		return 0, fmt.Errorf("timeout must be %s or less", maxDashboardProbeTimeout)
	}
	return timeout, nil
}

func normalizeProbeActionStreams(value, fallback int) (int, error) {
	if value == 0 {
		if fallback <= 0 {
			return 5, nil
		}
		return fallback, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("streams must be positive")
	}
	if value > maxDashboardProbeStreams {
		return 0, fmt.Errorf("streams must be %d or less", maxDashboardProbeStreams)
	}
	return value, nil
}
