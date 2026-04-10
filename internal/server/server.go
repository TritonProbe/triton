package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/dashboard"
	"github.com/tritonprobe/triton/internal/storage"
)

type Server struct {
	cfg       config.ServerConfig
	https     *http.Server
	dashboard *dashboard.Server
}

func New(cfg config.ServerConfig, dataDir string, store *storage.FileStore) (*Server, error) {
	certFile, keyFile, err := ensureCertificate(cfg, dataDir)
	if err != nil {
		return nil, err
	}

	mux := NewMux()
	srv := &http.Server{
		Addr:         cfg.ListenTCP,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"h2", "http/1.1"},
		},
	}

	var dash *dashboard.Server
	if cfg.Dashboard {
		dash = dashboard.New(cfg.DashboardListen, store)
	}

	cfg.CertFile = certFile
	cfg.KeyFile = keyFile
	return &Server{cfg: cfg, https: srv, dashboard: dash}, nil
}

func (s *Server) Run() error {
	errCh := make(chan error, 2)

	go func() {
		log.Printf("server listening on https://%s", s.cfg.ListenTCP)
		if err := s.https.ListenAndServeTLS(s.cfg.CertFile, s.cfg.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

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

	select {
	case sig := <-sigCh:
		log.Printf("shutting down after %s", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if s.dashboard != nil {
		_ = s.dashboard.Shutdown(ctx)
	}
	return s.https.Shutdown(ctx)
}
