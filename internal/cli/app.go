package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/config"
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
	fmt.Fprintln(a.stdout, "Commands:")
	fmt.Fprintln(a.stdout, "  server   Run the test server with dashboard and TCP fallback")
	fmt.Fprintln(a.stdout, "  probe    Probe an external endpoint and store structured results")
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
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg, nil
		}
	}
	return "", errors.New("target argument is required")
}

func (a *App) runServer(args []string) error {
	opts, err := parseServerOptions(args)
	if err != nil {
		return err
	}
	cfg, store, err := loadBaseConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Apply(&cfg)
	srv, err := server.New(cfg.Server, cfg.Storage.ResultsDir, store)
	if err != nil {
		return err
	}
	return srv.Run()
}

func (a *App) runProbe(args []string) error {
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
	result, err := probe.Run(target, cfg.Probe)
	if err != nil {
		return err
	}
	if err := store.SaveProbe(result.ID, result); err != nil {
		return err
	}
	return renderOutput(a.stdout, opts.FormatOrDefault(cfg.Probe.DefaultFormat), result)
}

func (a *App) runBench(args []string) error {
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
	result, err := bench.Run(target, cfg.Bench)
	if err != nil {
		return err
	}
	if err := store.SaveBench(result.ID, result); err != nil {
		return err
	}
	return renderOutput(a.stdout, opts.FormatOrDefault("table"), result)
}
