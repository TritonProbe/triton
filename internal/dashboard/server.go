package dashboard

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"embed"
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

func New(addr string, store *storage.FileStore, opts Options) *Server {
	opts.withDefaults()
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
			TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		},
		store:          store,
		trace:          opts.TraceDir,
		config:         cloneMap(opts.Config),
		version:        opts.Version,
		buildTime:      opts.BuildTime,
		startedAt:      time.Now().UTC(),
		certFile:       opts.CertFile,
		keyFile:        opts.KeyFile,
		useTLS:         opts.UseTLS,
		probeCache:     map[string]cachedProbeSummary{},
		benchCache:     map[string]cachedBenchSummary{},
		probeListCache: map[string]cachedProbeList{},
		benchListCache: map[string]cachedBenchList{},
		traceListCache: map[string]cachedTraceList{},
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
	if s.http == nil {
		return nil
	}
	if s.useTLS {
		if s.certFile == "" || s.keyFile == "" {
			return http.ErrMissingFile
		}
		return s.http.ListenAndServeTLS(s.certFile, s.keyFile)
	}
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) Scheme() string {
	if s == nil || s.http == nil {
		return "http"
	}
	if s.useTLS {
		return "https"
	}
	return "http"
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
			Version:       s.version,
			BuildTime:     s.buildTime,
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
	summaries, err := s.probeList(query, items)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to build probe list", err)
		return
	}
	writeListMetadataHeaders(w, len(summaries), query)
	summaries = applyOffsetLimit(summaries, query.Offset, query.Limit)
	writeJSON(w, summaries)
}

func (s *Server) handleBenches(w http.ResponseWriter, r *http.Request) {
	query := parseListQuery(r)
	items, err := s.store.List("benches")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list benches", err)
		return
	}
	summaries, err := s.benchList(query, items)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to build bench list", err)
		return
	}
	writeListMetadataHeaders(w, len(summaries), query)
	summaries = applyOffsetLimit(summaries, query.Offset, query.Limit)
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
	items = s.traceList(query, items)
	writeListMetadataHeaders(w, len(items), query)
	items = applyOffsetLimit(items, query.Offset, query.Limit)
	writeJSON(w, items)
}

func (s *Server) handleTraceMeta(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimPrefix(r.URL.Path, "/api/v1/traces/meta/")
	if strings.Contains(rawPath, "/") {
		writeAPIError(w, http.StatusNotFound, "trace not found", nil)
		return
	}
	name := path.Base(rawPath)
	item, err := s.traceMetadata(name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "trace not found", err)
		return
	}
	writeJSON(w, item)
}

func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimPrefix(r.URL.Path, "/api/v1/traces/")
	if strings.Contains(rawPath, "/") {
		writeAPIError(w, http.StatusNotFound, "trace not found", nil)
		return
	}
	name := path.Base(rawPath)
	file, info, err := openTraceFile(s.trace, name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "trace not found", err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/qlog+json-seq")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, _ *http.Request) {
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
	file, info, err := openTraceFile(s.trace, name)
	if err != nil {
		return TraceMetadata{}, err
	}
	defer file.Close()
	preview, err := readTracePreview(file)
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
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), microphone=(), payment=(), usb=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
