package config

import (
	"reflect"
	"testing"
	"time"
)

func TestApplyProbeProfile(t *testing.T) {
	cfg := Default()
	cfg.ProbeProfiles = map[string]ProbeProfile{
		"prod-edge": {
			Target:         "https://example.com",
			Timeout:        5 * time.Second,
			DefaultFormat:  "markdown",
			DefaultTests:   []string{"latency", "streams"},
			DefaultStreams: 8,
			Thresholds: ProbeThresholds{
				MaxLatencyP95MS: 200,
			},
		},
	}

	profile, err := cfg.ApplyProbeProfile("prod-edge")
	if err != nil {
		t.Fatalf("ApplyProbeProfile returned error: %v", err)
	}
	if profile.Target != "https://example.com" {
		t.Fatalf("unexpected profile target: %q", profile.Target)
	}
	if cfg.Probe.Timeout != 5*time.Second || cfg.Probe.DefaultFormat != "markdown" || cfg.Probe.DefaultStreams != 8 {
		t.Fatalf("probe profile not applied: %+v", cfg.Probe)
	}
	if !reflect.DeepEqual(cfg.Probe.DefaultTests, []string{"latency", "streams"}) {
		t.Fatalf("probe profile tests not applied: %+v", cfg.Probe.DefaultTests)
	}
	if cfg.Probe.Thresholds.MaxLatencyP95MS != 200 {
		t.Fatalf("probe thresholds not applied: %+v", cfg.Probe.Thresholds)
	}
}

func TestApplyBenchProfile(t *testing.T) {
	cfg := Default()
	cfg.BenchProfiles = map[string]BenchProfile{
		"staging-api": {
			Target:             "https://example.com",
			DefaultDuration:    4 * time.Second,
			DefaultConcurrency: 3,
			DefaultProtocols:   []string{"h1", "h2", "h3"},
			DefaultFormat:      "json",
			Thresholds: BenchThresholds{
				RequireAllHealthy: true,
			},
		},
	}

	profile, err := cfg.ApplyBenchProfile("staging-api")
	if err != nil {
		t.Fatalf("ApplyBenchProfile returned error: %v", err)
	}
	if profile.Target != "https://example.com" {
		t.Fatalf("unexpected profile target: %q", profile.Target)
	}
	if cfg.Bench.DefaultDuration != 4*time.Second || cfg.Bench.DefaultConcurrency != 3 || cfg.Bench.DefaultFormat != "json" {
		t.Fatalf("bench profile not applied: %+v", cfg.Bench)
	}
	if !reflect.DeepEqual(cfg.Bench.DefaultProtocols, []string{"h1", "h2", "h3"}) {
		t.Fatalf("bench profile protocols not applied: %+v", cfg.Bench.DefaultProtocols)
	}
	if !cfg.Bench.Thresholds.RequireAllHealthy {
		t.Fatalf("bench thresholds not applied: %+v", cfg.Bench.Thresholds)
	}
}
