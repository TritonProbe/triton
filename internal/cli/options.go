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
	Profile          string
	Target           string
	Format           string
	ReportOut        string
	ReportFormat     string
	Timeout          time.Duration
	Insecure         bool
	AllowInsecureTLS bool
	TraceDir         string
	Streams          int
	Tests            string
	Full             bool
	ZeroRTT          bool
	Migration        bool
	RequireStatusMin int
	RequireStatusMax int
	MaxTotalMS       int64
	MaxLatencyP95MS  float64
	MaxStreamP95MS   float64
	MinStreamSuccess float64
	MinCoverageRatio float64
}

func newProbeFlagSet(output io.Writer) (*flag.FlagSet, *probeOptions) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(output)
	var opts probeOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Profile, "profile", "", "named probe profile from config")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.StringVar(&opts.ReportOut, "report-out", "", "write an execution report to this file")
	fs.StringVar(&opts.ReportFormat, "report-format", "markdown", "report format: markdown|json|yaml")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "request timeout")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	fs.BoolVar(&opts.AllowInsecureTLS, "allow-insecure-tls", false, "explicitly allow insecure TLS for lab probing")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for client qlog trace files")
	fs.IntVar(&opts.Streams, "streams", 0, "number of concurrent probe streams/samples")
	fs.StringVar(&opts.Tests, "tests", "", "comma-separated probe tests")
	fs.BoolVar(&opts.Full, "full", false, "run the full available probe suite")
	fs.BoolVar(&opts.ZeroRTT, "0rtt", false, "request 0-RTT probe coverage")
	fs.BoolVar(&opts.Migration, "migration", false, "request migration probe coverage")
	fs.IntVar(&opts.RequireStatusMin, "threshold-status-min", 0, "minimum accepted HTTP status")
	fs.IntVar(&opts.RequireStatusMax, "threshold-status-max", 0, "maximum accepted HTTP status")
	fs.Int64Var(&opts.MaxTotalMS, "threshold-total-ms", 0, "maximum accepted total probe duration in ms")
	fs.Float64Var(&opts.MaxLatencyP95MS, "threshold-latency-p95-ms", 0, "maximum accepted sampled latency p95 in ms")
	fs.Float64Var(&opts.MaxStreamP95MS, "threshold-stream-p95-ms", 0, "maximum accepted stream p95 latency in ms")
	fs.Float64Var(&opts.MinStreamSuccess, "threshold-stream-success-rate", 0, "minimum accepted stream success rate from 0 to 1")
	fs.Float64Var(&opts.MinCoverageRatio, "threshold-coverage-ratio", 0, "minimum accepted probe coverage ratio from 0 to 1")
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
	cfg.Probe.Thresholds = mergeProbeThresholdOptions(cfg.Probe.Thresholds, o)
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
	ConfigPath        string
	Profile           string
	Target            string
	Format            string
	ReportOut         string
	ReportFormat      string
	Duration          time.Duration
	Concurrency       int
	Protocols         string
	Insecure          bool
	AllowInsecureTLS  bool
	TraceDir          string
	RequireAllHealthy bool
	MaxErrorRate      float64
	MinReqPerSec      float64
	MaxP95MS          float64
}

func newBenchFlagSet(output io.Writer) (*flag.FlagSet, *benchOptions) {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.SetOutput(output)
	var opts benchOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Profile, "profile", "", "named bench profile from config")
	fs.StringVar(&opts.Target, "target", "", "target URL")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.StringVar(&opts.ReportOut, "report-out", "", "write an execution report to this file")
	fs.StringVar(&opts.ReportFormat, "report-format", "markdown", "report format: markdown|json|yaml")
	fs.DurationVar(&opts.Duration, "duration", 0, "benchmark duration")
	fs.IntVar(&opts.Concurrency, "concurrency", 0, "concurrent workers")
	fs.StringVar(&opts.Protocols, "protocols", "", "comma-separated protocols")
	fs.BoolVar(&opts.Insecure, "insecure", false, "skip certificate verification")
	fs.BoolVar(&opts.AllowInsecureTLS, "allow-insecure-tls", false, "explicitly allow insecure TLS for lab benchmarking")
	fs.StringVar(&opts.TraceDir, "trace-dir", "", "directory for client qlog trace files")
	fs.BoolVar(&opts.RequireAllHealthy, "threshold-require-all-healthy", false, "fail when any protocol is degraded or failed")
	fs.Float64Var(&opts.MaxErrorRate, "threshold-max-error-rate", 0, "maximum accepted per-protocol error rate from 0 to 1")
	fs.Float64Var(&opts.MinReqPerSec, "threshold-min-req-per-sec", 0, "minimum accepted per-protocol throughput")
	fs.Float64Var(&opts.MaxP95MS, "threshold-max-p95-ms", 0, "maximum accepted per-protocol p95 latency in ms")
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
	cfg.Bench.Thresholds = mergeBenchThresholdOptions(cfg.Bench.Thresholds, o)
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

func validateReportFormat(format string) error {
	switch format {
	case "", "json", "yaml", "markdown":
		return nil
	default:
		return errors.New("unsupported report format")
	}
}

func mergeProbeThresholdOptions(base config.ProbeThresholds, opts probeOptions) config.ProbeThresholds {
	if opts.RequireStatusMin != 0 {
		base.RequireStatusMin = opts.RequireStatusMin
	}
	if opts.RequireStatusMax != 0 {
		base.RequireStatusMax = opts.RequireStatusMax
	}
	if opts.MaxTotalMS != 0 {
		base.MaxTotalMS = opts.MaxTotalMS
	}
	if opts.MaxLatencyP95MS != 0 {
		base.MaxLatencyP95MS = opts.MaxLatencyP95MS
	}
	if opts.MaxStreamP95MS != 0 {
		base.MaxStreamP95MS = opts.MaxStreamP95MS
	}
	if opts.MinStreamSuccess != 0 {
		base.MinStreamSuccessRate = opts.MinStreamSuccess
	}
	if opts.MinCoverageRatio != 0 {
		base.MinCoverageRatio = opts.MinCoverageRatio
	}
	return base
}

func mergeBenchThresholdOptions(base config.BenchThresholds, opts benchOptions) config.BenchThresholds {
	base.RequireAllHealthy = base.RequireAllHealthy || opts.RequireAllHealthy
	if opts.MaxErrorRate != 0 {
		base.MaxErrorRate = opts.MaxErrorRate
	}
	if opts.MinReqPerSec != 0 {
		base.MinReqPerSec = opts.MinReqPerSec
	}
	if opts.MaxP95MS != 0 {
		base.MaxP95MS = opts.MaxP95MS
	}
	return base
}

type checkOptions struct {
	ConfigPath   string
	Profile      string
	ProbeProfile string
	BenchProfile string
	Target       string
	Format       string
	ReportOut    string
	ReportFormat string
	SummaryOut   string
	JUnitOut     string
	SkipProbe    bool
	SkipBench    bool
}

func newCheckFlagSet(output io.Writer) (*flag.FlagSet, *checkOptions) {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(output)
	var opts checkOptions
	fs.StringVar(&opts.ConfigPath, "config", "triton.yaml", "config file path")
	fs.StringVar(&opts.Profile, "profile", "", "shared profile name to resolve from probe_profiles and bench_profiles")
	fs.StringVar(&opts.ProbeProfile, "probe-profile", "", "named probe profile from config")
	fs.StringVar(&opts.BenchProfile, "bench-profile", "", "named bench profile from config")
	fs.StringVar(&opts.Target, "target", "", "optional target override applied to each selected check")
	fs.StringVar(&opts.Format, "format", "", "output format: table|json|yaml|markdown")
	fs.StringVar(&opts.ReportOut, "report-out", "", "write a combined check report to this file")
	fs.StringVar(&opts.ReportFormat, "report-format", "markdown", "report format: markdown|json|yaml")
	fs.StringVar(&opts.SummaryOut, "summary-out", "", "write a compact machine-readable check summary to this file")
	fs.StringVar(&opts.JUnitOut, "junit-out", "", "write a JUnit XML report for the check")
	fs.BoolVar(&opts.SkipProbe, "skip-probe", false, "skip the probe phase")
	fs.BoolVar(&opts.SkipBench, "skip-bench", false, "skip the bench phase")
	return fs, &opts
}

func parseCheckOptions(args []string) (checkOptions, error) {
	fs, opts := newCheckFlagSet(io.Discard)
	if err := fs.Parse(args); err != nil {
		return *opts, err
	}
	return *opts, nil
}

func (o checkOptions) FormatOrDefault(defaultFormat string) string {
	if o.Format != "" {
		return o.Format
	}
	return defaultFormat
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

func printCheckCommandHelp(w io.Writer) {
	fs, _ := newCheckFlagSet(w)
	fmt.Fprintln(w, "Usage: triton check [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Product workflow:")
	fmt.Fprintln(w, "  - run probe and/or bench against a named profile")
	fmt.Fprintln(w, "  - emit one combined verdict for CI and recurring checks")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Profile selection:")
	fmt.Fprintln(w, "  - --profile tries the same name in probe_profiles and bench_profiles")
	fmt.Fprintln(w, "  - --probe-profile and --bench-profile can be set independently")
	fmt.Fprintln(w, "  - without profiles, use --target and base config defaults")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fs.PrintDefaults()
}
