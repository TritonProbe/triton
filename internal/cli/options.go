package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/config"
)

var fullProbeTestSuite = []string{
	"handshake",
	"tls",
	"latency",
	"throughput",
	"streams",
	"alt-svc",
	"0rtt",
	"migration",
	"ecn",
	"retry",
	"version",
	"qpack",
	"congestion",
	"loss",
	"spin-bit",
}

type serverOptions struct {
	ConfigPath                string
	Listen                    string
	AllowExperimentalH3       bool
	AllowRemoteExperimentalH3 bool
	AllowMixedH3Planes        bool
	ListenH3                  string
	ListenTCP                 string
	CertFile                  string
	KeyFile                   string
	Dashboard                 bool
	DashboardListen           string
	AllowRemoteDashboard      bool
	DashboardUser             string
	DashboardPass             string
	MaxBodyBytes              int64
	AccessLog                 string
	TraceDir                  string
}

func newServerFlagSet(output io.Writer) (*flag.FlagSet, *serverOptions) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(output)
	var opts serverOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Listen, "listen", "", "experimental Triton UDP H3 listen address")
	fs.BoolVar(&opts.AllowExperimentalH3, "allow-experimental-h3", false, "acknowledge and enable the experimental Triton UDP H3 listener")
	fs.BoolVar(&opts.AllowRemoteExperimentalH3, "allow-remote-experimental-h3", false, "allow the experimental Triton UDP H3 listener to bind on non-loopback interfaces")
	fs.BoolVar(&opts.AllowMixedH3Planes, "allow-mixed-h3-planes", false, "allow running real HTTP/3 and experimental Triton UDP H3 listeners together")
	fs.StringVar(&opts.ListenH3, "listen-h3", "", "real HTTP/3 UDP listen address")
	fs.StringVar(&opts.ListenTCP, "listen-tcp", "", "TCP fallback listen address")
	fs.StringVar(&opts.CertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&opts.KeyFile, "key", "", "TLS private key file")
	fs.BoolVar(&opts.Dashboard, "dashboard", true, "enable dashboard")
	fs.StringVar(&opts.DashboardListen, "dashboard-listen", "", "dashboard listen address")
	fs.BoolVar(&opts.AllowRemoteDashboard, "allow-remote-dashboard", false, "allow dashboard binding on non-loopback interfaces")
	fs.StringVar(&opts.DashboardUser, "dashboard-user", "", "dashboard basic auth username")
	fs.StringVar(&opts.DashboardPass, "dashboard-pass", "", "dashboard basic auth password")
	fs.Int64Var(&opts.MaxBodyBytes, "max-body-bytes", 0, "maximum accepted request body size in bytes")
	fs.StringVar(&opts.AccessLog, "access-log", "", "write JSON access logs to this file")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for qlog trace files")
	return fs, &opts
}

func parseServerOptions(args []string) (serverOptions, error) {
	fs, opts := newServerFlagSet(io.Discard)
	if err := fs.Parse(args); err != nil {
		return *opts, err
	}
	return *opts, nil
}

func (o serverOptions) Apply(cfg *config.Config) {
	if o.Listen != "" {
		cfg.Server.Listen = o.Listen
	}
	cfg.Server.AllowExperimentalH3 = cfg.Server.AllowExperimentalH3 || o.AllowExperimentalH3
	cfg.Server.AllowRemoteExperimentalH3 = cfg.Server.AllowRemoteExperimentalH3 || o.AllowRemoteExperimentalH3
	cfg.Server.AllowMixedH3Planes = cfg.Server.AllowMixedH3Planes || o.AllowMixedH3Planes
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
	cfg.Server.AllowRemoteDashboard = cfg.Server.AllowRemoteDashboard || o.AllowRemoteDashboard
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
	ConfigPath       string
	Target           string
	Format           string
	Timeout          time.Duration
	Insecure         bool
	AllowInsecureTLS bool
	TraceDir         string
	Streams          int
	Tests            string
	Full             bool
	ZeroRTT          bool
	Migration        bool
}

func newProbeFlagSet(output io.Writer) (*flag.FlagSet, *probeOptions) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(output)
	var opts probeOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "request timeout")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	fs.BoolVar(&opts.AllowInsecureTLS, "allow-insecure-tls", false, "explicitly allow insecure TLS for lab probing")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for client qlog trace files")
	fs.IntVar(&opts.Streams, "streams", 0, "number of concurrent probe streams/samples")
	fs.StringVar(&opts.Tests, "tests", "", "comma-separated probe tests")
	fs.BoolVar(&opts.Full, "full", false, "run the full available probe suite")
	fs.BoolVar(&opts.ZeroRTT, "0rtt", false, "request 0-RTT probe coverage")
	fs.BoolVar(&opts.Migration, "migration", false, "request migration probe coverage")
	return fs, &opts
}

func parseProbeOptions(args []string) (probeOptions, error) {
	fs, opts := newProbeFlagSet(io.Discard)
	if err := fs.Parse(args); err != nil {
		return *opts, err
	}
	return *opts, nil
}

func (o probeOptions) Apply(cfg *config.Config) {
	if o.Timeout > 0 {
		cfg.Probe.Timeout = o.Timeout
	}
	if o.TraceDir != "" {
		cfg.Probe.TraceDir = o.TraceDir
	}
	if o.Streams > 0 {
		cfg.Probe.DefaultStreams = o.Streams
	}
	if tests := o.selectedTests(cfg.Probe.DefaultTests); len(tests) > 0 {
		cfg.Probe.DefaultTests = tests
	}
	cfg.Probe.AllowInsecureTLS = cfg.Probe.AllowInsecureTLS || o.AllowInsecureTLS
	cfg.Probe.Insecure = cfg.Probe.Insecure || o.Insecure
}

func (o probeOptions) selectedTests(defaults []string) []string {
	if o.Full {
		return append([]string(nil), fullProbeTestSuite...)
	}

	selected := make([]string, 0, len(defaults)+4)
	seen := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(strings.ToLower(value))
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			selected = append(selected, value)
		}
	}

	if o.Tests != "" {
		add(splitCSV(o.Tests)...)
	}
	if o.ZeroRTT {
		add("0rtt")
	}
	if o.Migration {
		add("migration")
	}
	if len(selected) == 0 {
		add(defaults...)
	}
	return selected
}

func (o probeOptions) FormatOrDefault(defaultFormat string) string {
	if o.Format != "" {
		return o.Format
	}
	return defaultFormat
}

type benchOptions struct {
	ConfigPath       string
	Target           string
	Format           string
	Duration         time.Duration
	Concurrency      int
	Protocols        string
	Insecure         bool
	AllowInsecureTLS bool
	TraceDir         string
}

func newBenchFlagSet(output io.Writer) (*flag.FlagSet, *benchOptions) {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.SetOutput(output)
	var opts benchOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.DurationVar(&opts.Duration, "duration", 0, "benchmark duration")
	fs.IntVar(&opts.Concurrency, "concurrency", 0, "concurrent workers")
	fs.StringVar(&opts.Protocols, "protocols", "", "comma-separated protocols")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	fs.BoolVar(&opts.AllowInsecureTLS, "allow-insecure-tls", false, "explicitly allow insecure TLS for lab benchmarking")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for client qlog trace files")
	return fs, &opts
}

func parseBenchOptions(args []string) (benchOptions, error) {
	fs, opts := newBenchFlagSet(io.Discard)
	if err := fs.Parse(args); err != nil {
		return *opts, err
	}
	return *opts, nil
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
	cfg.Bench.AllowInsecureTLS = cfg.Bench.AllowInsecureTLS || o.AllowInsecureTLS
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

func printServerCommandHelp(w io.Writer) {
	fs, _ := newServerFlagSet(w)
	fmt.Fprintln(w, "Usage: triton server [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Supported runtime:")
	fmt.Fprintln(w, "  - HTTPS/TCP test server")
	fmt.Fprintln(w, "  - optional real HTTP/3 via quic-go (--listen-h3)")
	fmt.Fprintln(w, "  - optional embedded dashboard")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Experimental surface:")
	fmt.Fprintln(w, "  - --listen enables the in-repo Triton UDP H3 listener")
	fmt.Fprintln(w, "  - this path is lab-only and requires --allow-experimental-h3")
	fmt.Fprintln(w, "  - mixing --listen with --listen-h3 requires explicit --allow-mixed-h3-planes")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fs.PrintDefaults()
}

func printLabCommandHelp(w io.Writer) {
	fs, _ := newServerFlagSet(w)
	fmt.Fprintln(w, "Usage: triton lab [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Lab-only runtime:")
	fmt.Fprintln(w, "  - runs only the experimental in-repo Triton UDP H3 surface")
	fmt.Fprintln(w, "  - disables supported HTTPS/TCP, real HTTP/3, and dashboard listeners")
	fmt.Fprintln(w, "  - intended for transport research, not production-like service hosting")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Shared server flags:")
	fs.PrintDefaults()
}

func printProbeCommandHelp(w io.Writer) {
	fs, _ := newProbeFlagSet(w)
	fmt.Fprintln(w, "Usage: triton probe [flags] [target]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Supported targets:")
	fmt.Fprintln(w, "  - https://... and h3://...")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Experimental target:")
	fmt.Fprintln(w, "  - triton://... is lab-only and uses the in-repo transport")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Probe fidelity note:")
	fmt.Fprintln(w, "  - advanced checks may be reported as full, observed, or partial")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fs.PrintDefaults()
}

func printBenchCommandHelp(w io.Writer) {
	fs, _ := newBenchFlagSet(w)
	fmt.Fprintln(w, "Usage: triton bench [flags] [target]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Supported comparisons:")
	fmt.Fprintln(w, "  - h1,h2,h3 against https://... targets")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Experimental target:")
	fmt.Fprintln(w, "  - triton://... uses the lab transport and should not be read as internet-facing protocol truth")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fs.PrintDefaults()
}
