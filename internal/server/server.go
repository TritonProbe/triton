package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	certFile, keyFile, err := ensureCertificate(cfg, dataDir)
	if err != nil {
		return nil, err
	}

	logger, err := observability.NewLogger(cfg.AccessLog)
	if err != nil {
		return nil, err
	}

	handler := buildHandler(cfg, logger.Logger)
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
	srv := &http.Server{
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
		})
	}

	cfg.CertFile = certFile
	cfg.KeyFile = keyFile
	return &Server{cfg: cfg, logger: logger, https: srv, h3real: h3real, dashboard: dash, udp: udpListener, h3: h3Server}, nil
}

func buildHandler(cfg config.ServerConfig, logger *slog.Logger) http.Handler {
	mux := appmux.NewWithOptions(appmux.Options{MaxBodyBytes: cfg.MaxBodyBytes})
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

func (s *Server) Run() error {
	errCh := make(chan error, 4)
	defer func() {
		if s.logger != nil {
			_ = s.logger.Close()
		}
	}()

	go func() {
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
			log.Printf("dashboard listening on http://%s", s.cfg.DashboardListen)
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
	if s.udp != nil {
		_ = s.udp.Close()
	}
	if s.h3real != nil {
		_ = s.h3real.Shutdown(ctx)
	}
	if s.dashboard != nil {
		_ = s.dashboard.Shutdown(ctx)
	}
	return s.https.Shutdown(ctx)
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

func withAltSvc(next http.Handler, h3server *quichttp3.Server) http.Handler {
	if h3server == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h3server.SetQUICHeaders(w.Header())
		next.ServeHTTP(w, r)
	})
}
