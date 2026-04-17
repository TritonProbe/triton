package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	fmt.Fprintln(a.stdout, "  version  Print version information")
}

func loadBaseConfig(path string) (config.Config, *storage.FileStore, error) {
	cfg, err := config.Load(path)
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
		"-target", "--target",
		"-format", "--format",
		"-timeout", "--timeout",
		"-streams", "--streams",
		"-tests", "--tests",
		"-duration", "--duration",
		"-concurrency", "--concurrency",
		"-protocols", "--protocols",
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
	srv, err := server.New(cfg.Server, cfg.Storage.ResultsDir, store)
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

	srv, err := server.New(cfg.Server, cfg.Storage.ResultsDir, store)
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
	target := opts.Target
	if target == "" {
		target, err = requireTarget(args)
		if err != nil {
			return err
		}
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Apply(&cfg)
	if err := cfg.Validate(); err != nil {
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
	return renderOutput(a.stdout, opts.FormatOrDefault(cfg.Probe.DefaultFormat), result)
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
	target := opts.Target
	if target == "" {
		target, err = requireTarget(args)
		if err != nil {
			return err
		}
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Apply(&cfg)
	if err := cfg.Validate(); err != nil {
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
	return renderOutput(a.stdout, opts.FormatOrDefault("table"), result)
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}
