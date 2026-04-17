package appmux

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/buildinfo"
)

type Options struct {
	MaxBodyBytes         int64
	Metrics              *Metrics
	SupportedProtocols   []string
	Capabilities         []string
	ExperimentalFeatures []string
	DeploymentProfile    string
	Stability            string
	Version              string
	BuildTime            string
}

func New() http.Handler {
	return NewWithOptions(Options{
		MaxBodyBytes: 1 << 20,
		Metrics:      NewMetrics(),
	})
}

func NewWithOptions(opts Options) http.Handler {
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 1 << 20
	}
	if opts.Metrics == nil {
		opts.Metrics = NewMetrics()
	}
	if len(opts.SupportedProtocols) == 0 {
		opts.SupportedProtocols = []string{"http/1.1", "h2"}
	}
	if len(opts.Capabilities) == 0 {
		opts.Capabilities = []string{"http1", "http2", "dashboard", "probe-storage", "bench-storage", "healthz", "readyz", "metrics"}
	}
	if opts.DeploymentProfile == "" {
		opts.DeploymentProfile = "standard"
	}
	if opts.Stability == "" {
		opts.Stability = "stable"
	}
	if opts.Version == "" {
		opts.Version = buildinfo.Version
	}
	if opts.BuildTime == "" {
		opts.BuildTime = buildinfo.BuildTime
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", methodHandler(func(w http.ResponseWriter, r *http.Request) {
		handleRoot(w, r, opts)
	}, http.MethodGet))
	mux.HandleFunc("/healthz", methodHandler(handleHealth, http.MethodGet))
	mux.HandleFunc("/readyz", methodHandler(handleReady, http.MethodGet))
	mux.HandleFunc("/metrics", methodHandler(opts.Metrics.handleMetrics, http.MethodGet))
	mux.HandleFunc("/ping", methodHandler(handlePing, http.MethodGet))
	mux.HandleFunc("/echo", methodHandler(func(w http.ResponseWriter, r *http.Request) {
		handleEcho(w, r, opts)
	}, http.MethodGet, http.MethodPost))
	mux.HandleFunc("/download/", methodHandler(handleDownload, http.MethodGet))
	mux.HandleFunc("/upload", methodHandler(func(w http.ResponseWriter, r *http.Request) {
		handleUpload(w, r, opts)
	}, http.MethodPost))
	mux.HandleFunc("/delay/", methodHandler(handleDelay, http.MethodGet))
	mux.HandleFunc("/redirect/", methodHandler(handleRedirect, http.MethodGet))
	mux.HandleFunc("/streams/", methodHandler(handleStreams, http.MethodGet))
	mux.HandleFunc("/headers/", methodHandler(handleHeaders, http.MethodGet))
	mux.HandleFunc("/status/", methodHandler(handleStatus, http.MethodGet))
	mux.HandleFunc("/drip/", methodHandler(handleDrip, http.MethodGet))
	mux.HandleFunc("/tls-info", methodHandler(handleTLSInfo, http.MethodGet))
	mux.HandleFunc("/quic-info", methodHandler(handleQUICInfo, http.MethodGet))
	mux.HandleFunc("/migration-test", methodHandler(handleMigration, http.MethodGet))
	mux.HandleFunc("/.well-known/triton", methodHandler(func(w http.ResponseWriter, r *http.Request) {
		handleCapabilities(w, r, opts)
	}, http.MethodGet))
	return opts.Metrics.middleware(mux)
}

func handleRoot(w http.ResponseWriter, _ *http.Request, opts Options) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":               "triton",
		"mode":               "server",
		"version":            opts.Version,
		"build_time":         opts.BuildTime,
		"capabilities":       append([]string(nil), opts.Capabilities...),
		"protocols":          append([]string(nil), opts.SupportedProtocols...),
		"deployment_profile": opts.DeploymentProfile,
		"stability":          opts.Stability,
	})
}

func handlePing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "pong")
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}

func handleEcho(w http.ResponseWriter, r *http.Request, opts Options) {
	body, err := readLimitedBody(w, r, opts.MaxBodyBytes)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"query":   r.URL.RawQuery,
		"headers": r.Header,
		"body":    body,
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	size, err := parseTailInt(r.URL.Path, "/download/")
	if err != nil || size <= 0 {
		http.Error(w, "invalid size", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	chunk := []byte("TRITONPROBE")
	remaining := size
	for remaining > 0 {
		writeLen := len(chunk)
		if writeLen > remaining {
			writeLen = remaining
		}
		if _, err := w.Write(chunk[:writeLen]); err != nil {
			return
		}
		remaining -= writeLen
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request, opts Options) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, opts.MaxBodyBytes)
	n, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		handleBodyReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bytes":       n,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

func handleDelay(w http.ResponseWriter, r *http.Request) {
	ms, err := parseTailInt(r.URL.Path, "/delay/")
	if err != nil || ms < 0 {
		http.Error(w, "invalid delay", http.StatusBadRequest)
		return
	}
	if !sleepWithContext(r.Context(), time.Duration(ms)*time.Millisecond) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delay_ms": ms})
}

func handleHeaders(w http.ResponseWriter, r *http.Request) {
	n, err := parseTailInt(r.URL.Path, "/headers/")
	if err != nil || n < 0 {
		http.Error(w, "invalid header count", http.StatusBadRequest)
		return
	}
	for i := 0; i < n; i++ {
		w.Header().Set(fmt.Sprintf("X-Triton-%d", i), fmt.Sprintf("value-%d", i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"headers": n})
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	n, err := parseTailInt(r.URL.Path, "/redirect/")
	if err != nil || n < 0 {
		http.Error(w, "invalid redirect count", http.StatusBadRequest)
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"redirects": 0, "final": true})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/redirect/%d", n-1), http.StatusFound)
}

func handleStreams(w http.ResponseWriter, r *http.Request) {
	n, err := parseTailInt(r.URL.Path, "/streams/")
	if err != nil || n < 0 {
		http.Error(w, "invalid stream count", http.StatusBadRequest)
		return
	}
	streams := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		streams = append(streams, map[string]any{
			"id":           i + 1,
			"scheduled_ms": i * 25,
			"state":        "simulated",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requested": n,
		"streams":   streams,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	code, err := parseTailInt(r.URL.Path, "/status/")
	if err != nil || code < 100 || code > 599 {
		http.Error(w, "invalid status code", http.StatusBadRequest)
		return
	}
	writeJSON(w, code, map[string]any{"status": code})
}

func handleDrip(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/drip/"), "/")
	if len(parts) != 2 {
		http.Error(w, "expected /drip/:size/:delay", http.StatusBadRequest)
		return
	}
	size, err1 := strconv.Atoi(parts[0])
	delay, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || size < 0 || delay < 0 {
		http.Error(w, "invalid drip parameters", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	flusher, _ := w.(http.Flusher)
	for i := 0; i < size; i++ {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			http.Error(w, "failed to generate drip payload", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte{'a' + (b[0] % 26)})
		if flusher != nil {
			flusher.Flush()
		}
		if !sleepWithContext(r.Context(), time.Duration(delay)*time.Millisecond) {
			return
		}
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func handleTLSInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{"tls": false}
	if r.TLS != nil {
		info = map[string]any{
			"tls":        true,
			"version":    tlsVersion(r.TLS.Version),
			"cipher":     tls.CipherSuiteName(r.TLS.CipherSuite),
			"alpn":       r.TLS.NegotiatedProtocol,
			"server":     r.TLS.ServerName,
			"peer_certs": len(r.TLS.PeerCertificates),
		}
	}
	writeJSON(w, http.StatusOK, info)
}

func handleQUICInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": false,
		"message":   "custom QUIC engine not implemented yet in this scaffold",
	})
}

func handleMigration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": false,
		"message":   "connection migration requires QUIC transport",
	})
}

func handleCapabilities(w http.ResponseWriter, _ *http.Request, opts Options) {
	payload := map[string]any{
		"name":               "triton",
		"version":            opts.Version,
		"build_time":         opts.BuildTime,
		"protocols":          append([]string(nil), opts.SupportedProtocols...),
		"deployment_profile": opts.DeploymentProfile,
		"stability":          opts.Stability,
		"endpoints":          []string{"/healthz", "/readyz", "/metrics", "/ping", "/echo", "/download/:size", "/upload", "/delay/:ms", "/streams/:n", "/headers/:n", "/redirect/:n", "/status/:code", "/drip/:size/:delay", "/tls-info", "/quic-info", "/migration-test", "/.well-known/triton"},
	}
	if len(opts.ExperimentalFeatures) > 0 {
		payload["experimental"] = append([]string(nil), opts.ExperimentalFeatures...)
	}
	writeJSON(w, http.StatusOK, payload)
}

func methodHandler(next http.HandlerFunc, methods ...string) http.HandlerFunc {
	allowed := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		allowed[method] = struct{}{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := allowed[r.Method]; !ok {
			w.Header().Set("Allow", strings.Join(methods, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func readLimitedBody(w http.ResponseWriter, r *http.Request, limit int64) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handleBodyReadError(w, err)
		return "", err
	}
	return string(body), nil
}

func handleBodyReadError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "failed to read request body", http.StatusBadRequest)
}

func parseTailInt(path, prefix string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(path, prefix))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func tlsVersion(v uint16) string {
	switch v {
	case 0x0304:
		return "TLS1.3"
	case 0x0303:
		return "TLS1.2"
	default:
		return fmt.Sprintf("0x%x", v)
	}
}
