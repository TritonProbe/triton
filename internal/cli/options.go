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
	ListenTCP       string
	CertFile        string
	KeyFile         string
	Dashboard       bool
	DashboardListen string
}

func parseServerOptions(args []string) (serverOptions, error) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	var opts serverOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Listen, "listen", "", "UDP/TLS listen address")
	fs.StringVar(&opts.ListenTCP, "listen-tcp", "", "TCP fallback listen address")
	fs.StringVar(&opts.CertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&opts.KeyFile, "key", "", "TLS private key file")
	fs.BoolVar(&opts.Dashboard, "dashboard", true, "enable dashboard")
	fs.StringVar(&opts.DashboardListen, "dashboard-listen", "", "dashboard listen address")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

func (o serverOptions) Apply(cfg *config.Config) {
	if o.Listen != "" {
		cfg.Server.Listen = o.Listen
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
	cfg.Server.Dashboard = o.Dashboard
}

type probeOptions struct {
	ConfigPath string
	Target     string
	Format     string
	Timeout    time.Duration
	Insecure   bool
}

func parseProbeOptions(args []string) (probeOptions, error) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var opts probeOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "request timeout")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

func (o probeOptions) Apply(cfg *config.Config) {
	if o.Timeout > 0 {
		cfg.Probe.Timeout = o.Timeout
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
