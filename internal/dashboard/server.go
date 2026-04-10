package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	http  *http.Server
	store *storage.FileStore
}

func New(addr string, store *storage.FileStore) *Server {
	mux := http.NewServeMux()
	s := &Server{
		http:  &http.Server{Addr: addr, Handler: mux},
		store: store,
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/probes", s.handleProbes)
	mux.HandleFunc("/api/v1/benches", s.handleBenches)
	mux.HandleFunc("/api/v1/probes/", s.handleProbe)
	mux.HandleFunc("/api/v1/benches/", s.handleBench)
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
