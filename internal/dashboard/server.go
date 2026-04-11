package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
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
	http  *http.Server
	store *storage.FileStore
	trace string
}

type Options struct {
	Username string
	Password string
	Logger   *observability.ManagedLogger
	TraceDir string
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
		store: store,
		trace: opts.TraceDir,
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
	mux.HandleFunc("/api/v1/status", getOnly(s.handleStatus))
	mux.HandleFunc("/api/v1/probes", getOnly(s.handleProbes))
	mux.HandleFunc("/api/v1/benches", getOnly(s.handleBenches))
	mux.HandleFunc("/api/v1/probes/", getOnly(s.handleProbe))
	mux.HandleFunc("/api/v1/benches/", getOnly(s.handleBench))
	mux.HandleFunc("/api/v1/traces", getOnly(s.handleTraces))
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
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleProbes(w http.ResponseWriter, _ *http.Request) {
	items, err := s.store.List("probes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (s *Server) handleBenches(w http.ResponseWriter, _ *http.Request) {
	items, err := s.store.List("benches")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/probes/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var result probe.Result
	if err := s.store.Load("probes", id, &result); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleBench(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/benches/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var result bench.Result
	if err := s.store.Load("benches", id, &result); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleTraces(w http.ResponseWriter, _ *http.Request) {
	items, err := s.listTraces()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	name := path.Base(strings.TrimPrefix(r.URL.Path, "/api/v1/traces/"))
	if name == "." || name == "" || strings.Contains(name, "/") || filepath.Ext(name) != ".sqlog" {
		http.NotFound(w, r)
		return
	}
	if s.trace == "" {
		http.NotFound(w, r)
		return
	}
	fullPath := filepath.Join(s.trace, name)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/qlog+json-seq")
	http.ServeFile(w, r, fullPath)
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
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, map[string]any{
			"name":         entry.Name(),
			"size_bytes":   info.Size(),
			"modified_at":  info.ModTime().UTC().Format(time.RFC3339),
			"download_url": "/api/v1/traces/" + entry.Name(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["name"].(string) < items[j]["name"].(string)
	})
	return items, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
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
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}
