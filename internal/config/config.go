package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Probe   ProbeConfig   `yaml:"probe"`
	Bench   BenchConfig   `yaml:"bench"`
	Storage StorageConfig `yaml:"storage"`
}

type ServerConfig struct {
	Listen          string        `yaml:"listen"`
	ListenTCP       string        `yaml:"listen_tcp"`
	CertFile        string        `yaml:"cert"`
	KeyFile         string        `yaml:"key"`
	Dashboard       bool          `yaml:"dashboard"`
	DashboardListen string        `yaml:"dashboard_listen"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	RateLimit       int           `yaml:"rate_limit"`
	TraceDir        string        `yaml:"trace_dir"`
	AccessLog       string        `yaml:"access_log"`
}

type ProbeConfig struct {
	Timeout        time.Duration `yaml:"timeout"`
	Insecure       bool          `yaml:"insecure"`
	DefaultTests   []string      `yaml:"default_tests"`
	DefaultFormat  string        `yaml:"default_format"`
	DownloadSize   string        `yaml:"download_size"`
	UploadSize     string        `yaml:"upload_size"`
	DefaultStreams int           `yaml:"default_streams"`
}

type BenchConfig struct {
	Warmup             time.Duration `yaml:"warmup"`
	DefaultDuration    time.Duration `yaml:"default_duration"`
	DefaultConcurrency int           `yaml:"default_concurrency"`
	DefaultProtocols   []string      `yaml:"default_protocols"`
}

type StorageConfig struct {
	ResultsDir string        `yaml:"results_dir"`
	MaxResults int           `yaml:"max_results"`
	Retention  time.Duration `yaml:"retention"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Listen:          ":4433",
			ListenTCP:       ":8443",
			Dashboard:       true,
			DashboardListen: "127.0.0.1:9090",
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     30 * time.Second,
		},
		Probe: ProbeConfig{
			Timeout:        10 * time.Second,
			DefaultTests:   []string{"handshake", "tls", "latency"},
			DefaultFormat:  "table",
			DownloadSize:   "1MB",
			UploadSize:     "1MB",
			DefaultStreams: 10,
		},
		Bench: BenchConfig{
			Warmup:             2 * time.Second,
			DefaultDuration:    10 * time.Second,
			DefaultConcurrency: 10,
			DefaultProtocols:   []string{"h1", "h2"},
		},
		Storage: StorageConfig{
			ResultsDir: filepath.Clean("./triton-data"),
			MaxResults: 1000,
			Retention:  30 * 24 * time.Hour,
		},
	}
}

func (c Config) Validate() error {
	if err := validateListen(c.Server.Listen, "server.listen"); err != nil {
		return err
	}
	if err := validateListen(c.Server.ListenTCP, "server.listen_tcp"); err != nil {
		return err
	}
	if c.Server.Dashboard {
		if err := validateListen(c.Server.DashboardListen, "server.dashboard_listen"); err != nil {
			return err
		}
	}
	if c.Server.ReadTimeout <= 0 || c.Server.WriteTimeout <= 0 || c.Server.IdleTimeout <= 0 {
		return errors.New("server timeouts must be positive")
	}
	if c.Storage.ResultsDir == "" {
		return errors.New("storage.results_dir cannot be empty")
	}
	if c.Storage.MaxResults <= 0 {
		return errors.New("storage.max_results must be positive")
	}
	if c.Storage.Retention <= 0 {
		return errors.New("storage.retention must be positive")
	}
	if c.Probe.Timeout <= 0 {
		return errors.New("probe.timeout must be positive")
	}
	if c.Bench.Warmup < 0 || c.Bench.DefaultDuration <= 0 || c.Bench.DefaultConcurrency <= 0 {
		return errors.New("bench defaults are invalid")
	}
	if (c.Server.CertFile == "") != (c.Server.KeyFile == "") {
		return errors.New("server cert and key must be provided together")
	}
	for _, path := range []string{c.Server.CertFile, c.Server.KeyFile} {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("cannot access %s: %w", path, err)
		}
	}
	return nil
}

func validateListen(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("%s must be in host:port form: %w", field, err)
	}
	if host != "" && strings.Contains(host, " ") {
		return fmt.Errorf("%s host is invalid", field)
	}
	if port == "" {
		return fmt.Errorf("%s port is required", field)
	}
	return nil
}
