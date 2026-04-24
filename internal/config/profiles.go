package config

import (
	"fmt"
	"strings"
	"time"
)

type ProbeThresholds struct {
	RequireStatusMin     int     `yaml:"require_status_min,omitempty"`
	RequireStatusMax     int     `yaml:"require_status_max,omitempty"`
	MaxTotalMS           int64   `yaml:"max_total_ms,omitempty"`
	MaxLatencyP95MS      float64 `yaml:"max_latency_p95_ms,omitempty"`
	MaxStreamP95MS       float64 `yaml:"max_stream_p95_ms,omitempty"`
	MinStreamSuccessRate float64 `yaml:"min_stream_success_rate,omitempty"`
	MinCoverageRatio     float64 `yaml:"min_coverage_ratio,omitempty"`
}

type BenchThresholds struct {
	RequireAllHealthy bool    `yaml:"require_all_healthy,omitempty"`
	MaxErrorRate      float64 `yaml:"max_error_rate,omitempty"`
	MinReqPerSec      float64 `yaml:"min_req_per_sec,omitempty"`
	MaxP95MS          float64 `yaml:"max_p95_ms,omitempty"`
}

type ProbeProfile struct {
	Target           string          `yaml:"target"`
	ReportName       string          `yaml:"report_name,omitempty"`
	Timeout          time.Duration   `yaml:"timeout,omitempty"`
	Insecure         bool            `yaml:"insecure,omitempty"`
	AllowInsecureTLS bool            `yaml:"allow_insecure_tls,omitempty"`
	TraceDir         string          `yaml:"trace_dir,omitempty"`
	DefaultTests     []string        `yaml:"default_tests,omitempty"`
	DefaultFormat    string          `yaml:"default_format,omitempty"`
	DefaultStreams   int             `yaml:"default_streams,omitempty"`
	Thresholds       ProbeThresholds `yaml:"thresholds,omitempty"`
}

type BenchProfile struct {
	Target             string          `yaml:"target"`
	ReportName         string          `yaml:"report_name,omitempty"`
	Warmup             time.Duration   `yaml:"warmup,omitempty"`
	DefaultDuration    time.Duration   `yaml:"default_duration,omitempty"`
	DefaultConcurrency int             `yaml:"default_concurrency,omitempty"`
	DefaultProtocols   []string        `yaml:"default_protocols,omitempty"`
	DefaultFormat      string          `yaml:"default_format,omitempty"`
	Insecure           bool            `yaml:"insecure,omitempty"`
	AllowInsecureTLS   bool            `yaml:"allow_insecure_tls,omitempty"`
	TraceDir           string          `yaml:"trace_dir,omitempty"`
	Thresholds         BenchThresholds `yaml:"thresholds,omitempty"`
}

func (c *Config) ApplyProbeProfile(name string) (ProbeProfile, error) {
	if c == nil {
		return ProbeProfile{}, fmt.Errorf("probe profile %q requires config", name)
	}
	profile, ok := c.ProbeProfiles[name]
	if !ok {
		return ProbeProfile{}, fmt.Errorf("unknown probe profile %q", name)
	}
	if profile.Timeout > 0 {
		c.Probe.Timeout = profile.Timeout
	}
	if profile.TraceDir != "" {
		c.Probe.TraceDir = profile.TraceDir
	}
	if len(profile.DefaultTests) > 0 {
		c.Probe.DefaultTests = append([]string(nil), profile.DefaultTests...)
	}
	if profile.DefaultFormat != "" {
		c.Probe.DefaultFormat = profile.DefaultFormat
	}
	if profile.DefaultStreams > 0 {
		c.Probe.DefaultStreams = profile.DefaultStreams
	}
	c.Probe.Insecure = c.Probe.Insecure || profile.Insecure
	c.Probe.AllowInsecureTLS = c.Probe.AllowInsecureTLS || profile.AllowInsecureTLS
	c.Probe.Thresholds = mergeProbeThresholds(c.Probe.Thresholds, profile.Thresholds)
	return profile, nil
}

func (c *Config) ApplyBenchProfile(name string) (BenchProfile, error) {
	if c == nil {
		return BenchProfile{}, fmt.Errorf("bench profile %q requires config", name)
	}
	profile, ok := c.BenchProfiles[name]
	if !ok {
		return BenchProfile{}, fmt.Errorf("unknown bench profile %q", name)
	}
	if profile.Warmup > 0 {
		c.Bench.Warmup = profile.Warmup
	}
	if profile.DefaultDuration > 0 {
		c.Bench.DefaultDuration = profile.DefaultDuration
	}
	if profile.DefaultConcurrency > 0 {
		c.Bench.DefaultConcurrency = profile.DefaultConcurrency
	}
	if len(profile.DefaultProtocols) > 0 {
		c.Bench.DefaultProtocols = append([]string(nil), profile.DefaultProtocols...)
	}
	if profile.DefaultFormat != "" {
		c.Bench.DefaultFormat = profile.DefaultFormat
	}
	if profile.TraceDir != "" {
		c.Bench.TraceDir = profile.TraceDir
	}
	c.Bench.Insecure = c.Bench.Insecure || profile.Insecure
	c.Bench.AllowInsecureTLS = c.Bench.AllowInsecureTLS || profile.AllowInsecureTLS
	c.Bench.Thresholds = mergeBenchThresholds(c.Bench.Thresholds, profile.Thresholds)
	return profile, nil
}

func mergeProbeThresholds(base, override ProbeThresholds) ProbeThresholds {
	if override.RequireStatusMin != 0 {
		base.RequireStatusMin = override.RequireStatusMin
	}
	if override.RequireStatusMax != 0 {
		base.RequireStatusMax = override.RequireStatusMax
	}
	if override.MaxTotalMS != 0 {
		base.MaxTotalMS = override.MaxTotalMS
	}
	if override.MaxLatencyP95MS != 0 {
		base.MaxLatencyP95MS = override.MaxLatencyP95MS
	}
	if override.MaxStreamP95MS != 0 {
		base.MaxStreamP95MS = override.MaxStreamP95MS
	}
	if override.MinStreamSuccessRate != 0 {
		base.MinStreamSuccessRate = override.MinStreamSuccessRate
	}
	if override.MinCoverageRatio != 0 {
		base.MinCoverageRatio = override.MinCoverageRatio
	}
	return base
}

func mergeBenchThresholds(base, override BenchThresholds) BenchThresholds {
	base.RequireAllHealthy = base.RequireAllHealthy || override.RequireAllHealthy
	if override.MaxErrorRate != 0 {
		base.MaxErrorRate = override.MaxErrorRate
	}
	if override.MinReqPerSec != 0 {
		base.MinReqPerSec = override.MinReqPerSec
	}
	if override.MaxP95MS != 0 {
		base.MaxP95MS = override.MaxP95MS
	}
	return base
}

func validateOutputFormat(value, field string) error {
	switch value {
	case "", "table", "json", "yaml", "markdown":
		return nil
	default:
		return fmt.Errorf("%s has unsupported format %q", field, value)
	}
}

func validateProbeThresholds(value ProbeThresholds, field string) error {
	if value.RequireStatusMin != 0 && (value.RequireStatusMin < 100 || value.RequireStatusMin > 599) {
		return fmt.Errorf("%s.require_status_min must be between 100 and 599", field)
	}
	if value.RequireStatusMax != 0 && (value.RequireStatusMax < 100 || value.RequireStatusMax > 599) {
		return fmt.Errorf("%s.require_status_max must be between 100 and 599", field)
	}
	if value.RequireStatusMin != 0 && value.RequireStatusMax != 0 && value.RequireStatusMin > value.RequireStatusMax {
		return fmt.Errorf("%s status range is invalid", field)
	}
	if value.MaxTotalMS < 0 || value.MaxLatencyP95MS < 0 || value.MaxStreamP95MS < 0 {
		return fmt.Errorf("%s latency thresholds must be non-negative", field)
	}
	if value.MinStreamSuccessRate < 0 || value.MinStreamSuccessRate > 1 {
		return fmt.Errorf("%s.min_stream_success_rate must be between 0 and 1", field)
	}
	if value.MinCoverageRatio < 0 || value.MinCoverageRatio > 1 {
		return fmt.Errorf("%s.min_coverage_ratio must be between 0 and 1", field)
	}
	return nil
}

func validateBenchThresholds(value BenchThresholds, field string) error {
	if value.MaxErrorRate < 0 || value.MaxErrorRate > 1 {
		return fmt.Errorf("%s.max_error_rate must be between 0 and 1", field)
	}
	if value.MinReqPerSec < 0 || value.MaxP95MS < 0 {
		return fmt.Errorf("%s throughput and latency thresholds must be non-negative", field)
	}
	return nil
}

func (c Config) validateProfiles() error {
	for name, profile := range c.ProbeProfiles {
		prefix := "probe_profiles." + name
		if strings.TrimSpace(profile.Target) == "" {
			return fmt.Errorf("%s.target cannot be empty", prefix)
		}
		if profile.Timeout < 0 {
			return fmt.Errorf("%s.timeout must be non-negative", prefix)
		}
		if profile.DefaultStreams < 0 {
			return fmt.Errorf("%s.default_streams must be non-negative", prefix)
		}
		if err := validateOutputFormat(profile.DefaultFormat, prefix+".default_format"); err != nil {
			return err
		}
		if err := validateProbeThresholds(profile.Thresholds, prefix+".thresholds"); err != nil {
			return err
		}
	}
	for name, profile := range c.BenchProfiles {
		prefix := "bench_profiles." + name
		if strings.TrimSpace(profile.Target) == "" {
			return fmt.Errorf("%s.target cannot be empty", prefix)
		}
		if profile.Warmup < 0 || profile.DefaultDuration < 0 {
			return fmt.Errorf("%s durations must be non-negative", prefix)
		}
		if profile.DefaultConcurrency < 0 {
			return fmt.Errorf("%s.default_concurrency must be non-negative", prefix)
		}
		if err := validateOutputFormat(profile.DefaultFormat, prefix+".default_format"); err != nil {
			return err
		}
		if err := validateBenchThresholds(profile.Thresholds, prefix+".thresholds"); err != nil {
			return err
		}
	}
	return nil
}
