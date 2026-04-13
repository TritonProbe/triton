package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		// #nosec G304 -- config path is an explicit operator-provided input.
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return Config{}, err
			}
		} else if !os.IsNotExist(err) {
			return Config{}, err
		}
	}

	applyEnv(&cfg)
	return cfg, cfg.Validate()
}

func applyEnv(cfg *Config) {
	setString("TRITON_SERVER_LISTEN", &cfg.Server.Listen)
	setBool("TRITON_SERVER_ALLOW_EXPERIMENTAL_H3", &cfg.Server.AllowExperimentalH3)
	setBool("TRITON_SERVER_ALLOW_REMOTE_EXPERIMENTAL_H3", &cfg.Server.AllowRemoteExperimentalH3)
	setString("TRITON_SERVER_LISTEN_H3", &cfg.Server.ListenH3)
	setString("TRITON_SERVER_LISTEN_TCP", &cfg.Server.ListenTCP)
	setString("TRITON_SERVER_TLS_CERT", &cfg.Server.CertFile)
	setString("TRITON_SERVER_TLS_KEY", &cfg.Server.KeyFile)
	setString("TRITON_SERVER_DASHBOARD_LISTEN", &cfg.Server.DashboardListen)
	setBool("TRITON_SERVER_ALLOW_REMOTE_DASHBOARD", &cfg.Server.AllowRemoteDashboard)
	setString("TRITON_SERVER_DASHBOARD_USER", &cfg.Server.DashboardUser)
	setString("TRITON_SERVER_DASHBOARD_PASS", &cfg.Server.DashboardPass)
	setBool("TRITON_DASHBOARD_ENABLED", &cfg.Server.Dashboard)
	setDuration("TRITON_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout)
	setDuration("TRITON_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout)
	setDuration("TRITON_SERVER_IDLE_TIMEOUT", &cfg.Server.IdleTimeout)
	setInt64("TRITON_SERVER_MAX_BODY_BYTES", &cfg.Server.MaxBodyBytes)
	setInt("TRITON_SERVER_RATE_LIMIT", &cfg.Server.RateLimit)
	setString("TRITON_SERVER_ACCESS_LOG", &cfg.Server.AccessLog)
	setString("TRITON_SERVER_TRACE_DIR", &cfg.Server.TraceDir)
	setString("TRITON_STORAGE_RESULTS_DIR", &cfg.Storage.ResultsDir)
	setInt("TRITON_STORAGE_MAX_RESULTS", &cfg.Storage.MaxResults)
	setDuration("TRITON_STORAGE_RETENTION", &cfg.Storage.Retention)
	setDuration("TRITON_PROBE_TIMEOUT", &cfg.Probe.Timeout)
	setBool("TRITON_PROBE_INSECURE", &cfg.Probe.Insecure)
	setBool("TRITON_PROBE_ALLOW_INSECURE_TLS", &cfg.Probe.AllowInsecureTLS)
	setString("TRITON_PROBE_TRACE_DIR", &cfg.Probe.TraceDir)
	setString("TRITON_PROBE_DEFAULT_FORMAT", &cfg.Probe.DefaultFormat)
	setString("TRITON_PROBE_DOWNLOAD_SIZE", &cfg.Probe.DownloadSize)
	setString("TRITON_PROBE_UPLOAD_SIZE", &cfg.Probe.UploadSize)
	setInt("TRITON_PROBE_DEFAULT_STREAMS", &cfg.Probe.DefaultStreams)
	setCSV("TRITON_PROBE_DEFAULT_TESTS", &cfg.Probe.DefaultTests)
	setDuration("TRITON_BENCH_WARMUP", &cfg.Bench.Warmup)
	setDuration("TRITON_BENCH_DEFAULT_DURATION", &cfg.Bench.DefaultDuration)
	setInt("TRITON_BENCH_DEFAULT_CONCURRENCY", &cfg.Bench.DefaultConcurrency)
	setCSV("TRITON_BENCH_DEFAULT_PROTOCOLS", &cfg.Bench.DefaultProtocols)
	setBool("TRITON_BENCH_INSECURE", &cfg.Bench.Insecure)
	setBool("TRITON_BENCH_ALLOW_INSECURE_TLS", &cfg.Bench.AllowInsecureTLS)
	setString("TRITON_BENCH_TRACE_DIR", &cfg.Bench.TraceDir)
}

func setString(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func setBool(key string, target *bool) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			*target = parsed
		}
	}
}

func setInt(key string, target *int) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}

func setInt64(key string, target *int64) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			*target = parsed
		}
	}
}

func setDuration(key string, target *time.Duration) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := time.ParseDuration(value); err == nil {
			*target = parsed
		}
	}
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
