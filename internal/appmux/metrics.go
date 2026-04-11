package appmux

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	startedAt time.Time

	mu           sync.Mutex
	requests     uint64
	requestsByOp map[string]uint64
	statusCodes  map[int]uint64
}

func NewMetrics() *Metrics {
	return &Metrics{
		startedAt:    time.Now().UTC(),
		requestsByOp: make(map[string]uint64),
		statusCodes:  make(map[int]uint64),
	}
}

func (m *Metrics) middleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		m.record(r.URL.Path, rec.status)
	})
}

func (m *Metrics) record(path string, status int) {
	route := canonicalRoute(path)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests++
	m.requestsByOp[route]++
	m.statusCodes[status]++
}

func (m *Metrics) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	lines := []string{
		"# HELP triton_requests_total Total HTTP requests handled by Triton.",
		"# TYPE triton_requests_total counter",
		fmt.Sprintf("triton_requests_total %d", m.totalRequests()),
		"# HELP triton_requests_by_route_total Requests grouped by canonical route.",
		"# TYPE triton_requests_by_route_total counter",
	}
	lines = append(lines, m.routeMetrics()...)
	lines = append(lines,
		"# HELP triton_responses_by_status_total Responses grouped by status code.",
		"# TYPE triton_responses_by_status_total counter",
	)
	lines = append(lines, m.statusMetrics()...)
	lines = append(lines,
		"# HELP triton_uptime_seconds Process uptime in seconds.",
		"# TYPE triton_uptime_seconds gauge",
		fmt.Sprintf("triton_uptime_seconds %.0f", time.Since(m.startedAt).Seconds()),
	)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}

func (m *Metrics) totalRequests() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

func (m *Metrics) routeMetrics() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	routes := make([]string, 0, len(m.requestsByOp))
	for route := range m.requestsByOp {
		routes = append(routes, route)
	}
	sort.Strings(routes)

	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, fmt.Sprintf("triton_requests_by_route_total{route=%q} %d", route, m.requestsByOp[route]))
	}
	return out
}

func (m *Metrics) statusMetrics() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	codes := make([]int, 0, len(m.statusCodes))
	for code := range m.statusCodes {
		codes = append(codes, code)
	}
	sort.Ints(codes)

	out := make([]string, 0, len(codes))
	for _, code := range codes {
		out = append(out, fmt.Sprintf("triton_responses_by_status_total{status=%q} %d", strconv.Itoa(code), m.statusCodes[code]))
	}
	return out
}

func canonicalRoute(path string) string {
	switch {
	case path == "" || path == "/":
		return "/"
	case strings.HasPrefix(path, "/download/"):
		return "/download/:size"
	case strings.HasPrefix(path, "/delay/"):
		return "/delay/:ms"
	case strings.HasPrefix(path, "/redirect/"):
		return "/redirect/:n"
	case strings.HasPrefix(path, "/streams/"):
		return "/streams/:n"
	case strings.HasPrefix(path, "/headers/"):
		return "/headers/:n"
	case strings.HasPrefix(path, "/status/"):
		return "/status/:code"
	case strings.HasPrefix(path, "/drip/"):
		return "/drip/:size/:delay"
	default:
		return path
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
