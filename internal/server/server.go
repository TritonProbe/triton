package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	quichttp3 "github.com/quic-go/quic-go/http3"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/dashboard"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/quic/transport"
	"github.com/tritonprobe/triton/internal/storage"
)

type Server struct {
	cfg       config.ServerConfig
	logger    *observability.ManagedLogger
	https     *http.Server
	h3real    *quichttp3.Server
	dashboard *dashboard.Server
	udp       *transport.Listener
	h3        *h3.UDPServer
}

func New(cfg config.ServerConfig, dataDir string, store *storage.FileStore) (*Server, error) {
	defaults := config.Default()
	return NewWithDashboardDefaults(cfg, dataDir, store, defaults.Bench, defaults.Probe)
}

func NewWithDashboardDefaults(cfg config.ServerConfig, dataDir string, store *storage.FileStore, benchDefaults config.BenchConfig, probeDefaults config.ProbeConfig) (*Server, error) {
	if cfg.Dashboard && cfg.AllowRemoteDashboard && (cfg.CertFile == "" || cfg.KeyFile == "") {
		return nil, errors.New("remote dashboard requires explicit tls certificate and key files")
	}
	certFile, keyFile, err := ensureCertificate(cfg, dataDir)
	if err != nil {
		return nil, err
	}

	logger, err := observability.NewLogger(cfg.AccessLog)
	if err != nil {
		return nil, err
	}

	cfg.CertFile = certFile
	cfg.KeyFile = keyFile

	handler := buildHandler(cfg, dataDir, logger.Logger)
	var h3real *quichttp3.Server
	if cfg.ListenH3 != "" {
		h3real = &quichttp3.Server{
			Addr:      cfg.ListenH3,
			Handler:   handler,
			TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
			Logger:    logger.Logger,
			QUICConfig: &quic.Config{
				Tracer: observability.NewQLOGTracer(cfg.TraceDir),
			},
		}
	}
	var srv *http.Server
	if cfg.ListenTCP != "" {
		srv = &http.Server{
			Addr:         cfg.ListenTCP,
			Handler:      withAltSvc(handler, h3real),
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				NextProtos: []string{"h2", "http/1.1"},
			},
		}
	}

	var udpListener *transport.Listener
	var h3Server *h3.UDPServer
	if cfg.Listen != "" {
		udpListener, err = transport.ListenQUIC(cfg.Listen)
		if err != nil {
			return nil, err
		}
		udpListener.SetAutoEcho(false)
		h3Server = h3.NewUDPServer(udpListener, handler)
	}

	var dash *dashboard.Server
	if cfg.Dashboard {
		dash = dashboard.New(cfg.DashboardListen, store, dashboard.Options{
			Username: cfg.DashboardUser,
			Password: cfg.DashboardPass,
			Logger:   logger,
			TraceDir: cfg.TraceDir,
			Config:   dashboardConfigSnapshot(cfg),
			CertFile: cfg.CertFile,
			KeyFile:  cfg.KeyFile,
			UseTLS:   cfg.AllowRemoteDashboard,
			Bench:    benchDefaults,
			Probe:    probeDefaults,
		})
	}

	return &Server{cfg: cfg, logger: logger, https: srv, h3real: h3real, dashboard: dash, udp: udpListener, h3: h3Server}, nil
}

func buildHandler(cfg config.ServerConfig, dataDir string, logger *slog.Logger) http.Handler {
	mux := appmux.NewWithOptions(appmux.Options{
		MaxBodyBytes:         cfg.MaxBodyBytes,
		SupportedProtocols:   supportedProtocols(cfg),
		Capabilities:         capabilityFlags(cfg),
		ExperimentalFeatures: experimentalFeatures(cfg),
		DeploymentProfile:    deploymentProfile(cfg),
		Stability:            stabilityLevel(cfg),
		HealthCheck:          runtimeHealthCheck(cfg, dataDir),
		ReadinessCheck:       runtimeReadyCheck(cfg, dataDir),
	})
	return observability.WithRequestID(
		observability.WithAccessLog(
			logger,
			"server",
			withSecurityHeaders(
				newRateLimiter(cfg.RateLimit).middleware(mux),
			),
		),
	)
}

func supportedProtocols(cfg config.ServerConfig) []string {
	protocols := []string{"http/1.1", "h2"}
	if cfg.ListenH3 != "" {
		protocols = append(protocols, "h3")
	}
	if cfg.Listen != "" && cfg.AllowExperimentalH3 {
		protocols = append(protocols, "triton-h3")
	}
	return protocols
}

func capabilityFlags(cfg config.ServerConfig) []string {
	flags := []string{"http1", "http2", "probe-storage", "bench-storage", "healthz", "readyz", "metrics"}
	if cfg.Dashboard {
		flags = append(flags, "dashboard")
	}
	if cfg.ListenH3 != "" {
		flags = append(flags, "http3")
	}
	if cfg.TraceDir != "" {
		flags = append(flags, "qlog")
	}
	return flags
}

func experimentalFeatures(cfg config.ServerConfig) []string {
	features := make([]string, 0, 1)
	if cfg.Listen != "" && cfg.AllowExperimentalH3 {
		features = append(features, "triton-udp-h3")
	}
	return features
}

func deploymentProfile(cfg config.ServerConfig) string {
	if len(experimentalFeatures(cfg)) > 0 {
		if cfg.ListenH3 != "" || cfg.ListenTCP != "" {
			return "mixed"
		}
		return "experimental"
	}
	if cfg.ListenH3 != "" {
		return "http3"
	}
	return "standard"
}

func stabilityLevel(cfg config.ServerConfig) string {
	if len(experimentalFeatures(cfg)) > 0 {
		return "mixed-stability"
	}
	return "stable"
}

func (s *Server) Run() error {
	errCh := make(chan error, 4)
	defer func() {
		if s.logger != nil {
			if err := s.logger.Close(); err != nil {
				log.Printf("logger close failed: %v", err)
			}
		}
	}()

	for _, line := range startupSummaryLines(s.cfg) {
		log.Printf("%s", line)
	}

	go func() {
		if s.https == nil {
			return
		}
		log.Printf("server listening on https://%s", s.cfg.ListenTCP)
		if err := s.https.ListenAndServeTLS(s.cfg.CertFile, s.cfg.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if s.h3 != nil {
		go func() {
			log.Printf("experimental h3 listener on udp://%s", s.cfg.Listen)
			if err := s.h3.Serve(); err != nil {
				errCh <- err
			}
		}()
	}
	if s.h3real != nil {
		go func() {
			log.Printf("http/3 listener on udp://%s", s.cfg.ListenH3)
			if err := s.h3real.ListenAndServeTLS(s.cfg.CertFile, s.cfg.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	if s.dashboard != nil {
		go func() {
			log.Printf("dashboard listening on %s://%s", s.dashboard.Scheme(), s.cfg.DashboardListen)
			if err := s.dashboard.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		log.Printf("shutting down after %s", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var shutdownErr error
	if s.udp != nil {
		if err := s.udp.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close udp listener: %w", err))
		}
	}
	if s.h3 != nil {
		if err := s.h3.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close experimental h3 listener: %w", err))
		}
	}
	if s.h3real != nil {
		if err := s.h3real.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown http/3 server: %w", err))
		}
	}
	if s.dashboard != nil {
		if err := s.dashboard.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown dashboard: %w", err))
		}
	}
	if s.https != nil {
		if err := s.https.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown https server: %w", err))
		}
	}
	return shutdownErr
}

func startupSummaryLines(cfg config.ServerConfig) []string {
	lines := []string{
		fmt.Sprintf("triton server startup: %s", strings.Join(activeListenersSummary(cfg), "; ")),
		fmt.Sprintf(
			"transport profile=%s stable=[%s] experimental=[%s]",
			deploymentProfile(cfg),
			strings.Join(stableTransportPlanes(cfg), ","),
			strings.Join(experimentalTransportPlanes(cfg), ","),
		),
	}
	if cfg.ListenH3 != "" && cfg.Listen != "" && cfg.AllowMixedH3Planes {
		lines = append(lines, "mixed-plane mode enabled by explicit allow_mixed_h3_planes=true (real http/3 + experimental udp h3)")
	}
	if len(experimentalFeatures(cfg)) > 0 {
		lines = append(lines, "warning: experimental Triton UDP H3 is enabled; the supported production-like path is HTTPS/TCP plus optional real HTTP/3 via quic-go")
		lines = append(lines, "warning: treat the Triton UDP H3 listener as lab-only research and do not read it as a production-stable transport surface")
	}
	return lines
}

func stableTransportPlanes(cfg config.ServerConfig) []string {
	planes := make([]string, 0, 2)
	if cfg.ListenTCP != "" {
		planes = append(planes, "https-tcp")
	}
	if cfg.ListenH3 != "" {
		planes = append(planes, "http3-quic")
	}
	if len(planes) == 0 {
		return []string{"none"}
	}
	return planes
}

func experimentalTransportPlanes(cfg config.ServerConfig) []string {
	planes := make([]string, 0, 1)
	if cfg.Listen != "" && cfg.AllowExperimentalH3 {
		planes = append(planes, "triton-udp-h3")
	}
	if len(planes) == 0 {
		return []string{"none"}
	}
	return planes
}

func activeListenersSummary(cfg config.ServerConfig) []string {
	parts := make([]string, 0, 4)
	if cfg.ListenTCP != "" {
		parts = append(parts, fmt.Sprintf("https/tcp=%s", cfg.ListenTCP))
	}
	if cfg.ListenH3 != "" {
		parts = append(parts, fmt.Sprintf("http3/quic=%s", cfg.ListenH3))
	}
	if cfg.Listen != "" {
		parts = append(parts, fmt.Sprintf("experimental-h3/udp=%s", cfg.Listen))
	}
	if cfg.Dashboard {
		parts = append(parts, fmt.Sprintf("dashboard=%s", cfg.DashboardListen))
	}
	if len(parts) == 0 {
		return []string{"no listeners configured"}
	}
	return parts
}

func dashboardConfigSnapshot(cfg config.ServerConfig) map[string]any {
	return map[string]any{
		"listeners": map[string]any{
			"https_tcp":       cfg.ListenTCP,
			"http3_quic":      cfg.ListenH3,
			"experimental_h3": cfg.Listen,
			"dashboard":       cfg.DashboardListen,
		},
		"dashboard": map[string]any{
			"enabled":                cfg.Dashboard,
			"allow_remote_dashboard": cfg.AllowRemoteDashboard,
			"auth_enabled":           cfg.DashboardUser != "" && cfg.DashboardPass != "",
			"tls_enabled":            cfg.AllowRemoteDashboard,
			"transport":              dashboardTransport(cfg),
		},
		"limits": map[string]any{
			"max_body_bytes": cfg.MaxBodyBytes,
			"rate_limit":     cfg.RateLimit,
		},
		"timeouts_ms": map[string]any{
			"read":  cfg.ReadTimeout.Milliseconds(),
			"write": cfg.WriteTimeout.Milliseconds(),
			"idle":  cfg.IdleTimeout.Milliseconds(),
		},
		"observability": map[string]any{
			"trace_enabled":        cfg.TraceDir != "",
			"trace_dir_configured": cfg.TraceDir != "",
			"access_log_enabled":   cfg.AccessLog != "",
		},
		"tls": map[string]any{
			"cert_configured": cfg.CertFile != "",
			"key_configured":  cfg.KeyFile != "",
		},
	}
}

func dashboardTransport(cfg config.ServerConfig) string {
	if cfg.AllowRemoteDashboard {
		return "https"
	}
	return "http"
}

func runtimeHealthCheck(cfg config.ServerConfig, dataDir string) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateRuntimeTLSFiles(cfg); err != nil {
			return err
		}
		if err := ensureDirectoryAccessible(dataDir, false); err != nil {
			return fmt.Errorf("storage directory unavailable: %w", err)
		}
		if cfg.TraceDir != "" {
			if err := ensureDirectoryAccessible(cfg.TraceDir, false); err != nil {
				return fmt.Errorf("trace directory unavailable: %w", err)
			}
		}
		return nil
	}
}

func runtimeReadyCheck(cfg config.ServerConfig, dataDir string) func(context.Context) error {
	health := runtimeHealthCheck(cfg, dataDir)
	return func(ctx context.Context) error {
		if err := health(ctx); err != nil {
			return err
		}
		if err := ensureDirectoryAccessible(dataDir, true); err != nil {
			return fmt.Errorf("storage directory not writable: %w", err)
		}
		if cfg.TraceDir != "" {
			if err := ensureDirectoryAccessible(cfg.TraceDir, true); err != nil {
				return fmt.Errorf("trace directory not writable: %w", err)
			}
		}
		return nil
	}
}

func validateRuntimeTLSFiles(cfg config.ServerConfig) error {
	needsTLS := cfg.ListenTCP != "" || cfg.ListenH3 != "" || (cfg.Dashboard && cfg.AllowRemoteDashboard)
	if !needsTLS {
		return nil
	}
	for _, path := range []string{cfg.CertFile, cfg.KeyFile} {
		if path == "" {
			return errors.New("tls materials are not configured")
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot access tls material %s: %w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("tls material %s is a directory", path)
		}
	}
	if _, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err != nil {
		return fmt.Errorf("invalid tls material pair: %w", err)
	}
	return nil
}

func ensureDirectoryAccessible(path string, writable bool) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if !writable {
		return nil
	}
	file, err := os.CreateTemp(path, ".triton-ready-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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

func withAltSvc(next http.Handler, h3server *quichttp3.Server) http.Handler {
	if h3server == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h3server.SetQUICHeaders(w.Header())
		next.ServeHTTP(w, r)
	})
}
