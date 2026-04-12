package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
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
		http: &http.Server{Addr: addr, Handler: observability.WithRequestID(
			observability.WithAccessLog(
				logger.Logger,
				"dashboard",
				withSecurityHeaders(
					withOptionalBasicAuth(mux, opts),
				),
			),
		)},
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
	writeJSON(w, map[string]any{
		"status": "ok",
		"dashboard": map[string]any{
			"started_at":     s.startedAt.Format(time.RFC3339),
			"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
			"trace_enabled":  s.trace != "",
		},
		"storage": map[string]any{
			"probes":  len(probes),
			"benches": len(benches),
			"traces":  len(traces),
		},
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.config)
}

func (s *Server) handleProbes(w http.ResponseWriter, _ *http.Request) {
	items, err := s.store.List("probes")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list probes", err)
		return
	}
	writeJSON(w, items)
}

func (s *Server) handleBenches(w http.ResponseWriter, _ *http.Request) {
	items, err := s.store.List("benches")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list benches", err)
		return
	}
	writeJSON(w, items)
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

func (s *Server) handleTraces(w http.ResponseWriter, _ *http.Request) {
	items, err := s.listTraces()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list traces", err)
		return
	}
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

func (s *Server) listTraces() ([]map[string]any, error) {
	if s.trace == "" {
		return []map[string]any{}, nil
	}
	entries, err := os.ReadDir(s.trace)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}

	items := make([]map[string]any, 0, len(entries))
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
		return items[i]["name"].(string) < items[j]["name"].(string)
	})
	return items, nil
}

func (s *Server) traceMetadata(name string) (map[string]any, error) {
	if name == "." || name == "" || strings.Contains(name, "/") || filepath.Ext(name) != ".sqlog" {
		return nil, os.ErrNotExist
	}
	if s.trace == "" {
		return nil, os.ErrNotExist
	}
	fullPath := filepath.Join(s.trace, name)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, err
	}
	preview, err := readTracePreview(fullPath)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name":         name,
		"size_bytes":   info.Size(),
		"modified_at":  info.ModTime().UTC().Format(time.RFC3339),
		"download_url": "/api/v1/traces/" + name,
		"meta_url":     "/api/v1/traces/meta/" + name,
		"preview":      preview,
	}, nil
}

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
	payload := map[string]any{
		"status": "error",
		"error": map[string]any{
			"code":    status,
			"message": message,
		},
	}
	if err != nil {
		payload["error"].(map[string]any)["detail"] = err.Error()
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
	_, _ = w.Write(data)
}

func withOptionalBasicAuth(next http.Handler, opts Options) http.Handler {
	if opts.Username == "" && opts.Password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != opts.Username || pass != opts.Password {
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
