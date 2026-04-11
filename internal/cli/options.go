package cli

import (
	"errors"
	"flag"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/config"
)

type serverOptions struct {
	ConfigPath      string
	Listen          string
	ListenH3        string
	ListenTCP       string
	CertFile        string
	KeyFile         string
	Dashboard       bool
	DashboardListen string
	DashboardUser   string
	DashboardPass   string
	MaxBodyBytes    int64
	AccessLog       string
	TraceDir        string
}

func parseServerOptions(args []string) (serverOptions, error) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	var opts serverOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Listen, "listen", "", "UDP/TLS listen address")
	fs.StringVar(&opts.ListenH3, "listen-h3", "", "real HTTP/3 UDP listen address")
	fs.StringVar(&opts.ListenTCP, "listen-tcp", "", "TCP fallback listen address")
	fs.StringVar(&opts.CertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&opts.KeyFile, "key", "", "TLS private key file")
	fs.BoolVar(&opts.Dashboard, "dashboard", true, "enable dashboard")
	fs.StringVar(&opts.DashboardListen, "dashboard-listen", "", "dashboard listen address")
	fs.StringVar(&opts.DashboardUser, "dashboard-user", "", "dashboard basic auth username")
	fs.StringVar(&opts.DashboardPass, "dashboard-pass", "", "dashboard basic auth password")
	fs.Int64Var(&opts.MaxBodyBytes, "max-body-bytes", 0, "maximum accepted request body size in bytes")
	fs.StringVar(&opts.AccessLog, "access-log", "", "write JSON access logs to this file")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for qlog trace files")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

func (o serverOptions) Apply(cfg *config.Config) {
	if o.Listen != "" {
		cfg.Server.Listen = o.Listen
	}
	if o.ListenH3 != "" {
		cfg.Server.ListenH3 = o.ListenH3
	}
	if o.ListenTCP != "" {
		cfg.Server.ListenTCP = o.ListenTCP
	}
	if o.CertFile != "" {
		cfg.Server.CertFile = o.CertFile
	}
	if o.KeyFile != "" {
		cfg.Server.KeyFile = o.KeyFile
	}
	if o.DashboardListen != "" {
		cfg.Server.DashboardListen = o.DashboardListen
	}
	if o.DashboardUser != "" {
		cfg.Server.DashboardUser = o.DashboardUser
	}
	if o.DashboardPass != "" {
		cfg.Server.DashboardPass = o.DashboardPass
	}
	if o.MaxBodyBytes > 0 {
		cfg.Server.MaxBodyBytes = o.MaxBodyBytes
	}
	if o.AccessLog != "" {
		cfg.Server.AccessLog = o.AccessLog
	}
	if o.TraceDir != "" {
		cfg.Server.TraceDir = o.TraceDir
	}
	cfg.Server.Dashboard = o.Dashboard
}

type probeOptions struct {
	ConfigPath string
	Target     string
	Format     string
	Timeout    time.Duration
	Insecure   bool
	TraceDir   string
}

func parseProbeOptions(args []string) (probeOptions, error) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var opts probeOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "request timeout")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for client qlog trace files")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

func (o probeOptions) Apply(cfg *config.Config) {
	if o.Timeout > 0 {
		cfg.Probe.Timeout = o.Timeout
	}
	if o.TraceDir != "" {
		cfg.Probe.TraceDir = o.TraceDir
	}
	cfg.Probe.Insecure = cfg.Probe.Insecure || o.Insecure
}

func (o probeOptions) FormatOrDefault(defaultFormat string) string {
	if o.Format != "" {
		return o.Format
	}
	return defaultFormat
}

type benchOptions struct {
	ConfigPath  string
	Target      string
	Format      string
	Duration    time.Duration
	Concurrency int
	Protocols   string
	Insecure    bool
	TraceDir    string
}

func parseBenchOptions(args []string) (benchOptions, error) {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	var opts benchOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.DurationVar(&opts.Duration, "duration", 0, "benchmark duration")
	fs.IntVar(&opts.Concurrency, "concurrency", 0, "concurrent workers")
	fs.StringVar(&opts.Protocols, "protocols", "", "comma-separated protocols")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for client qlog trace files")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

func (o benchOptions) Apply(cfg *config.Config) {
	if o.Duration > 0 {
		cfg.Bench.DefaultDuration = o.Duration
	}
	if o.Concurrency > 0 {
		cfg.Bench.DefaultConcurrency = o.Concurrency
	}
	if o.Protocols != "" {
		cfg.Bench.DefaultProtocols = splitCSV(o.Protocols)
	}
	if o.TraceDir != "" {
		cfg.Bench.TraceDir = o.TraceDir
	}
	cfg.Bench.Insecure = cfg.Bench.Insecure || o.Insecure
}

func (o benchOptions) FormatOrDefault(defaultFormat string) string {
	if o.Format != "" {
		return o.Format
	}
	return defaultFormat
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func validateFormat(format string) error {
	switch format {
	case "", "table", "json", "yaml", "markdown":
		return nil
	default:
		return errors.New("unsupported output format")
	}
}
