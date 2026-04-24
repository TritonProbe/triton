package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	cfg, err := LoadUnvalidated(path)
	if err != nil {
		return Config{}, err
	}
	return cfg, cfg.Validate()
}

func LoadUnvalidated(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		// #nosec G304 -- config path is an explicit operator-provided input.
		if data, err := os.ReadFile(path); err == nil {
			decoder := yaml.NewDecoder(bytes.NewReader(data))
			decoder.KnownFields(true)
			if err := decoder.Decode(&cfg); err != nil {
				return Config{}, err
			}
		} else if !os.IsNotExist(err) {
			return Config{}, err
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) error {
	var errs []error
	setString("TRITON_SERVER_LISTEN", &cfg.Server.Listen)
	errs = append(errs, setBool("TRITON_SERVER_ALLOW_EXPERIMENTAL_H3", &cfg.Server.AllowExperimentalH3))
	errs = append(errs, setBool("TRITON_SERVER_ALLOW_REMOTE_EXPERIMENTAL_H3", &cfg.Server.AllowRemoteExperimentalH3))
	errs = append(errs, setBool("TRITON_SERVER_ALLOW_MIXED_H3_PLANES", &cfg.Server.AllowMixedH3Planes))
	setString("TRITON_SERVER_LISTEN_H3", &cfg.Server.ListenH3)
	setString("TRITON_SERVER_LISTEN_TCP", &cfg.Server.ListenTCP)
	setString("TRITON_SERVER_TLS_CERT", &cfg.Server.CertFile)
	setString("TRITON_SERVER_TLS_KEY", &cfg.Server.KeyFile)
	setString("TRITON_SERVER_DASHBOARD_LISTEN", &cfg.Server.DashboardListen)
	errs = append(errs, setBool("TRITON_SERVER_ALLOW_REMOTE_DASHBOARD", &cfg.Server.AllowRemoteDashboard))
	setString("TRITON_SERVER_DASHBOARD_USER", &cfg.Server.DashboardUser)
	setString("TRITON_SERVER_DASHBOARD_PASS", &cfg.Server.DashboardPass)
	errs = append(errs, setBool("TRITON_DASHBOARD_ENABLED", &cfg.Server.Dashboard))
	errs = append(errs, setDuration("TRITON_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout))
	errs = append(errs, setDuration("TRITON_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout))
	errs = append(errs, setDuration("TRITON_SERVER_IDLE_TIMEOUT", &cfg.Server.IdleTimeout))
	errs = append(errs, setInt64("TRITON_SERVER_MAX_BODY_BYTES", &cfg.Server.MaxBodyBytes))
	errs = append(errs, setInt("TRITON_SERVER_RATE_LIMIT", &cfg.Server.RateLimit))
	setString("TRITON_SERVER_ACCESS_LOG", &cfg.Server.AccessLog)
	setString("TRITON_SERVER_TRACE_DIR", &cfg.Server.TraceDir)
	setString("TRITON_STORAGE_RESULTS_DIR", &cfg.Storage.ResultsDir)
	errs = append(errs, setInt("TRITON_STORAGE_MAX_RESULTS", &cfg.Storage.MaxResults))
	errs = append(errs, setDuration("TRITON_STORAGE_RETENTION", &cfg.Storage.Retention))
	errs = append(errs, setDuration("TRITON_PROBE_TIMEOUT", &cfg.Probe.Timeout))
	errs = append(errs, setBool("TRITON_PROBE_INSECURE", &cfg.Probe.Insecure))
	errs = append(errs, setBool("TRITON_PROBE_ALLOW_INSECURE_TLS", &cfg.Probe.AllowInsecureTLS))
	setString("TRITON_PROBE_TRACE_DIR", &cfg.Probe.TraceDir)
	setString("TRITON_PROBE_DEFAULT_FORMAT", &cfg.Probe.DefaultFormat)
	setString("TRITON_PROBE_DOWNLOAD_SIZE", &cfg.Probe.DownloadSize)
	setString("TRITON_PROBE_UPLOAD_SIZE", &cfg.Probe.UploadSize)
	errs = append(errs, setInt("TRITON_PROBE_DEFAULT_STREAMS", &cfg.Probe.DefaultStreams))
	setCSV("TRITON_PROBE_DEFAULT_TESTS", &cfg.Probe.DefaultTests)
	errs = append(errs, setDuration("TRITON_BENCH_WARMUP", &cfg.Bench.Warmup))
	errs = append(errs, setDuration("TRITON_BENCH_DEFAULT_DURATION", &cfg.Bench.DefaultDuration))
	errs = append(errs, setInt("TRITON_BENCH_DEFAULT_CONCURRENCY", &cfg.Bench.DefaultConcurrency))
	setCSV("TRITON_BENCH_DEFAULT_PROTOCOLS", &cfg.Bench.DefaultProtocols)
	errs = append(errs, setBool("TRITON_BENCH_INSECURE", &cfg.Bench.Insecure))
	errs = append(errs, setBool("TRITON_BENCH_ALLOW_INSECURE_TLS", &cfg.Bench.AllowInsecureTLS))
	setString("TRITON_BENCH_TRACE_DIR", &cfg.Bench.TraceDir)
	return errors.Join(compactErrors(errs)...)
}

func setString(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func setBool(key string, target *bool) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s: invalid boolean %q", key, value)
		}
		*target = parsed
	}
	return nil
}

func setInt(key string, target *int) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s: invalid integer %q", key, value)
		}
		*target = parsed
	}
	return nil
}

func setInt64(key string, target *int64) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid int64 %q", key, value)
		}
		*target = parsed
	}
	return nil
}

func setDuration(key string, target *time.Duration) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s: invalid duration %q", key, value)
		}
		*target = parsed
	}
	return nil
}

func setCSV(key string, target *[]string) {
	if value, ok := os.LookupEnv(key); ok {
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			*target = out
		}
	}
}

func compactErrors(errs []error) []error {
	out := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			out = append(out, err)
		}
	}
	return out
}
