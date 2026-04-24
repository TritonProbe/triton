package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/dashboard"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/server"
	"github.com/tritonprobe/triton/internal/storage"
)

type App struct {
	version   string
	buildTime string
	stdout    io.Writer
}

func NewApp(version, buildTime string) *App {
	return &App{
		version:   version,
		buildTime: buildTime,
		stdout:    os.Stdout,
	}
}

func (a *App) SetStdout(w io.Writer) {
	if w != nil {
		a.stdout = w
	}
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}

	switch args[0] {
	case "version", "--version", "-v":
		_, _ = fmt.Fprintf(a.stdout, "triton %s (%s)\n", a.version, a.buildTime)
		return nil
	case "help", "--help", "-h":
		a.printHelp()
		return nil
	case "server":
		return a.runServer(args[1:])
	case "lab":
		return a.runLab(args[1:])
	case "probe":
		return a.runProbe(args[1:])
	case "bench":
		return a.runBench(args[1:])
	case "check":
		return a.runCheck(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a *App) printHelp() {
	fmt.Fprintln(a.stdout, "Triton - HTTP test server and benchmarking platform")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "Supported product path:")
	fmt.Fprintln(a.stdout, "  server  HTTPS/TCP test server with optional real HTTP/3 via quic-go and optional dashboard")
	fmt.Fprintln(a.stdout, "  probe   Supported targets are https://... and h3://...")
	fmt.Fprintln(a.stdout, "  bench   Supported comparisons run against normal HTTPS targets")
	fmt.Fprintln(a.stdout, "  check   Reusable probe/bench verdict flow for profiles and CI")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "Experimental surface:")
	fmt.Fprintln(a.stdout, "  lab     In-repo Triton UDP H3 transport for lab-only transport research")
	fmt.Fprintln(a.stdout, "  note    triton://... targets and --listen are experimental and should not be treated as production-stable")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "Commands:")
	fmt.Fprintln(a.stdout, "  server   Run the supported server runtime")
	fmt.Fprintln(a.stdout, "  lab      Run only the experimental lab runtime")
	fmt.Fprintln(a.stdout, "  probe    Probe a target and store structured results")
	fmt.Fprintln(a.stdout, "  bench    Benchmark one target across protocols")
	fmt.Fprintln(a.stdout, "  check    Run a combined product-style verification flow")
	fmt.Fprintln(a.stdout, "  version  Print version information")
}

func loadBaseConfig(path string) (config.Config, *storage.FileStore, error) {
	cfg, err := config.LoadUnvalidated(path)
	if err != nil {
		return config.Config{}, nil, err
	}
	store, err := storage.NewFileStore(cfg.Storage.ResultsDir, cfg.Storage.MaxResults, cfg.Storage.Retention)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, store, nil
}

func requireTarget(args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			if strings.Contains(arg, "=") {
				continue
			}
			if flagTakesValue(arg) && i+1 < len(args) {
				i++
			}
			continue
		}
		return arg, nil
	}
	return "", errors.New("target argument is required")
}

func flagTakesValue(arg string) bool {
	switch arg {
	case "-config", "--config",
		"-profile", "--profile",
		"-probe-profile", "--probe-profile",
		"-bench-profile", "--bench-profile",
		"-target", "--target",
		"-format", "--format",
		"-report-out", "--report-out",
		"-report-format", "--report-format",
		"-summary-out", "--summary-out",
		"-junit-out", "--junit-out",
		"-timeout", "--timeout",
		"-streams", "--streams",
		"-tests", "--tests",
		"-duration", "--duration",
		"-concurrency", "--concurrency",
		"-protocols", "--protocols",
		"-threshold-status-min", "--threshold-status-min",
		"-threshold-status-max", "--threshold-status-max",
		"-threshold-total-ms", "--threshold-total-ms",
		"-threshold-latency-p95-ms", "--threshold-latency-p95-ms",
		"-threshold-stream-p95-ms", "--threshold-stream-p95-ms",
		"-threshold-stream-success-rate", "--threshold-stream-success-rate",
		"-threshold-coverage-ratio", "--threshold-coverage-ratio",
		"-threshold-max-error-rate", "--threshold-max-error-rate",
		"-threshold-min-req-per-sec", "--threshold-min-req-per-sec",
		"-threshold-max-p95-ms", "--threshold-max-p95-ms",
		"-listen", "--listen",
		"-listen-h3", "--listen-h3",
		"-listen-tcp", "--listen-tcp",
		"-cert", "--cert",
		"-key", "--key",
		"-dashboard-listen", "--dashboard-listen",
		"-dashboard-user", "--dashboard-user",
		"-dashboard-pass", "--dashboard-pass",
		"-max-body-bytes", "--max-body-bytes",
		"-access-log", "--access-log",
		"-trace-dir", "--trace-dir":
		return true
	default:
		return false
	}
}

func (a *App) runServer(args []string) error {
	if hasHelpFlag(args) {
		printServerCommandHelp(a.stdout)
		return nil
	}
	opts, err := parseServerOptions(args)
	if err != nil {
		return err
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Apply(&cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	srv, err := server.NewWithDashboardDefaults(cfg.Server, cfg.Storage.ResultsDir, store, cfg.Bench, cfg.Probe)
	if err != nil {
		return err
	}
	return srv.Run()
}

func (a *App) runLab(args []string) error {
	if hasHelpFlag(args) {
		printLabCommandHelp(a.stdout)
		return nil
	}
	opts, err := parseServerOptions(args)
	if err != nil {
		return err
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Apply(&cfg)

	cfg.Server.AllowExperimentalH3 = true
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "127.0.0.1:4433"
	}
	cfg.Server.ListenTCP = ""
	cfg.Server.ListenH3 = ""
	cfg.Server.Dashboard = false
	cfg.Server.AllowRemoteDashboard = false
	cfg.Server.DashboardUser = ""
	cfg.Server.DashboardPass = ""
	if err := cfg.Validate(); err != nil {
		return err
	}

	srv, err := server.NewWithDashboardDefaults(cfg.Server, cfg.Storage.ResultsDir, store, cfg.Bench, cfg.Probe)
	if err != nil {
		return err
	}
	return srv.Run()
}

func (a *App) runProbe(args []string) error {
	if hasHelpFlag(args) {
		printProbeCommandHelp(a.stdout)
		return nil
	}
	opts, err := parseProbeOptions(args)
	if err != nil {
		return err
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	var profile config.ProbeProfile
	if opts.Profile != "" {
		profile, err = cfg.ApplyProbeProfile(opts.Profile)
		if err != nil {
			return err
		}
	}
	opts.Apply(&cfg)
	target, err := resolveTarget(opts.Target, args, profile.Target)
	if err != nil {
		return err
	}
	if err := cfg.ValidateClient(); err != nil {
		return err
	}
	if err := validateReportFormat(opts.ReportFormat); err != nil {
		return err
	}
	result, err := probe.Run(target, cfg.Probe)
	if err != nil {
		return err
	}
	if err := store.SaveProbe(result.ID, result); err != nil {
		return err
	}
	if err := store.SaveProbeSummary(result.ID, dashboard.BuildProbeSummary(*result)); err != nil {
		return err
	}
	if err := renderOutput(a.stdout, opts.FormatOrDefault(cfg.Probe.DefaultFormat), result); err != nil {
		return err
	}
	if opts.ReportOut != "" {
		if err := writeProbeReport(opts.ReportOut, opts.ReportFormat, probeReportOptions{
			ProfileName: opts.Profile,
			ReportName:  firstNonEmpty(profile.ReportName, opts.Profile),
			Result:      *result,
			Thresholds:  cfg.Probe.Thresholds,
		}); err != nil {
			return err
		}
	}
	if violations := evaluateProbeThresholds(*result, cfg.Probe.Thresholds); len(violations) > 0 {
		return fmt.Errorf("probe thresholds failed: %s", strings.Join(violations, "; "))
	}
	return nil
}

func (a *App) runBench(args []string) error {
	if hasHelpFlag(args) {
		printBenchCommandHelp(a.stdout)
		return nil
	}
	opts, err := parseBenchOptions(args)
	if err != nil {
		return err
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	var profile config.BenchProfile
	if opts.Profile != "" {
		profile, err = cfg.ApplyBenchProfile(opts.Profile)
		if err != nil {
			return err
		}
	}
	opts.Apply(&cfg)
	target, err := resolveTarget(opts.Target, args, profile.Target)
	if err != nil {
		return err
	}
	if err := cfg.ValidateClient(); err != nil {
		return err
	}
	if err := validateReportFormat(opts.ReportFormat); err != nil {
		return err
	}
	result, err := bench.Run(target, cfg.Bench)
	if err != nil {
		return err
	}
	if err := store.SaveBench(result.ID, result); err != nil {
		return err
	}
	if err := store.SaveBenchSummary(result.ID, dashboard.BuildBenchSummary(*result)); err != nil {
		return err
	}
	if err := renderOutput(a.stdout, opts.FormatOrDefault(cfg.Bench.DefaultFormat), result); err != nil {
		return err
	}
	if opts.ReportOut != "" {
		if err := writeBenchReport(opts.ReportOut, opts.ReportFormat, benchReportOptions{
			ProfileName: opts.Profile,
			ReportName:  firstNonEmpty(profile.ReportName, opts.Profile),
			Result:      *result,
			Thresholds:  cfg.Bench.Thresholds,
		}); err != nil {
			return err
		}
	}
	if violations := evaluateBenchThresholds(*result, cfg.Bench.Thresholds); len(violations) > 0 {
		return fmt.Errorf("bench thresholds failed: %s", strings.Join(violations, "; "))
	}
	return nil
}

func (a *App) runCheck(args []string) error {
	if hasHelpFlag(args) {
		printCheckCommandHelp(a.stdout)
		return nil
	}
	opts, err := parseCheckOptions(args)
	if err != nil {
		return err
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	probeProfileName, benchProfileName, err := resolveCheckProfiles(cfg, opts)
	if err != nil {
		return err
	}
	runProbe := !opts.SkipProbe
	runBench := !opts.SkipBench
	if probeProfileName != "" || benchProfileName != "" {
		runProbe = runProbe && probeProfileName != ""
		runBench = runBench && benchProfileName != ""
	}
	if !runProbe && !runBench {
		return errors.New("check has nothing to run")
	}
	if err := validateReportFormat(opts.ReportFormat); err != nil {
		return err
	}

	result := CheckResult{
		GeneratedAt: time.Now().UTC(),
		Profile:     opts.Profile,
		Passed:      true,
	}

	if runProbe {
		probeCfg := cfg
		var profile config.ProbeProfile
		if probeProfileName != "" {
			profile, err = probeCfg.ApplyProbeProfile(probeProfileName)
			if err != nil {
				return err
			}
		}
		target, targetErr := resolveCheckTarget(opts.Target, profile.Target)
		check := &CheckProbeResult{Profile: probeProfileName}
		if targetErr != nil {
			check.Error = targetErr.Error()
		} else if err := probeCfg.ValidateClient(); err != nil {
			check.Error = err.Error()
		} else {
			check.Result, err = probe.Run(target, probeCfg.Probe)
			if err != nil {
				check.Error = err.Error()
			} else {
				check.Violations = evaluateProbeThresholds(*check.Result, probeCfg.Probe.Thresholds)
				check.Passed = len(check.Violations) == 0
				if saveErr := store.SaveProbe(check.Result.ID, check.Result); saveErr != nil {
					check.Error = saveErr.Error()
				} else if saveErr := store.SaveProbeSummary(check.Result.ID, dashboard.BuildProbeSummary(*check.Result)); saveErr != nil {
					check.Error = saveErr.Error()
				}
			}
		}
		if check.Error != "" {
			check.Passed = false
			check.Violations = append(check.Violations, check.Error)
		}
		result.Probe = check
		if !check.Passed {
			result.Passed = false
			result.Failures = append(result.Failures, prefixViolations("probe", check.Violations)...)
		}
	}

	if runBench {
		benchCfg := cfg
		var profile config.BenchProfile
		if benchProfileName != "" {
			profile, err = benchCfg.ApplyBenchProfile(benchProfileName)
			if err != nil {
				return err
			}
		}
		target, targetErr := resolveCheckTarget(opts.Target, profile.Target)
		check := &CheckBenchResult{Profile: benchProfileName}
		if targetErr != nil {
			check.Error = targetErr.Error()
		} else if err := benchCfg.ValidateClient(); err != nil {
			check.Error = err.Error()
		} else {
			check.Result, err = bench.Run(target, benchCfg.Bench)
			if err != nil {
				check.Error = err.Error()
			} else {
				check.Violations = evaluateBenchThresholds(*check.Result, benchCfg.Bench.Thresholds)
				check.Passed = len(check.Violations) == 0
				if saveErr := store.SaveBench(check.Result.ID, check.Result); saveErr != nil {
					check.Error = saveErr.Error()
				} else if saveErr := store.SaveBenchSummary(check.Result.ID, dashboard.BuildBenchSummary(*check.Result)); saveErr != nil {
					check.Error = saveErr.Error()
				}
			}
		}
		if check.Error != "" {
			check.Passed = false
			check.Violations = append(check.Violations, check.Error)
		}
		result.Bench = check
		if !check.Passed {
			result.Passed = false
			result.Failures = append(result.Failures, prefixViolations("bench", check.Violations)...)
		}
	}

	if err := renderOutput(a.stdout, opts.FormatOrDefault("table"), result); err != nil {
		return err
	}
	if opts.ReportOut != "" {
		if err := writeCheckReport(opts.ReportOut, opts.ReportFormat, result); err != nil {
			return err
		}
	}
	if opts.SummaryOut != "" {
		if err := writeCheckSummary(opts.SummaryOut, result); err != nil {
			return err
		}
	}
	if opts.JUnitOut != "" {
		if err := writeCheckJUnitReport(opts.JUnitOut, result); err != nil {
			return err
		}
	}
	if !result.Passed {
		return fmt.Errorf("check failed: %s", strings.Join(result.Failures, "; "))
	}
	return nil
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func resolveTarget(explicit string, args []string, profileTarget string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if target, err := requireTarget(args); err == nil {
		return target, nil
	}
	if profileTarget != "" {
		return profileTarget, nil
	}
	return "", errors.New("target argument is required")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveCheckProfiles(cfg config.Config, opts checkOptions) (string, string, error) {
	probeProfileName := opts.ProbeProfile
	benchProfileName := opts.BenchProfile
	if opts.Profile != "" {
		if probeProfileName == "" {
			if _, ok := cfg.ProbeProfiles[opts.Profile]; ok {
				probeProfileName = opts.Profile
			}
		}
		if benchProfileName == "" {
			if _, ok := cfg.BenchProfiles[opts.Profile]; ok {
				benchProfileName = opts.Profile
			}
		}
		if probeProfileName == "" && benchProfileName == "" {
			return "", "", fmt.Errorf("unknown check profile %q", opts.Profile)
		}
	}
	return probeProfileName, benchProfileName, nil
}

func resolveCheckTarget(explicit, profileTarget string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if profileTarget != "" {
		return profileTarget, nil
	}
	return "", errors.New("target is required for check")
}

func prefixViolations(prefix string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, prefix+": "+value)
	}
	return out
}
