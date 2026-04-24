package config

import (
	"strings"
	"testing"

	"github.com/tritonprobe/triton/internal/testutil"
)

func TestDefaultValidate(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
}

func TestDashboardAuthRequiresPair(t *testing.T) {
	cfg := Default()
	cfg.Server.DashboardUser = "admin"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected auth pair validation error")
	}
}

func TestListenH3Validation(t *testing.T) {
	cfg := Default()
	cfg.Server.ListenH3 = "bad"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected listen_h3 validation error")
	}
}

func TestExperimentalListenIsOptional(t *testing.T) {
	cfg := Default()
	cfg.Server.Listen = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config with experimental listener disabled should validate: %v", err)
	}
}

func TestExperimentalListenRequiresExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Server.Listen = "127.0.0.1:4433"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected experimental listen without opt-in to fail validation")
	}

	cfg.Server.AllowExperimentalH3 = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected experimental listen with opt-in to validate: %v", err)
	}
}

func TestExperimentalListenRemoteBindRequiresExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Server.Listen = "0.0.0.0:4433"
	cfg.Server.AllowExperimentalH3 = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected remote experimental listen without opt-in to fail validation")
	}

	cfg.Server.AllowRemoteExperimentalH3 = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected remote experimental listen with opt-in to validate: %v", err)
	}
}

func TestMixedH3PlanesRequireExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Server.Listen = "127.0.0.1:4433"
	cfg.Server.AllowExperimentalH3 = true
	cfg.Server.ListenH3 = ":4434"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected mixed real/experimental h3 listeners without opt-in to fail validation")
	}

	cfg.Server.AllowMixedH3Planes = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected mixed h3 listeners with opt-in to validate: %v", err)
	}
}

func TestValidateRequiresAtLeastOneServerListener(t *testing.T) {
	cfg := Default()
	cfg.Server.Listen = ""
	cfg.Server.ListenH3 = ""
	cfg.Server.ListenTCP = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when all server listeners are disabled")
	}
}

func TestDashboardRemoteBindRequiresExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Server.DashboardListen = "0.0.0.0:9090"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected remote dashboard bind to be rejected without opt-in")
	}

	cfg.Server.AllowRemoteDashboard = true
	cfg.Server.DashboardUser = "admin"
	cfg.Server.DashboardPass = "secret"
	cfg.Server.CertFile, cfg.Server.KeyFile = testutil.GenerateSelfSignedCertFiles(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected remote dashboard bind with opt-in to validate: %v", err)
	}
}

func TestRemoteDashboardRequiresAuth(t *testing.T) {
	cfg := Default()
	cfg.Server.AllowRemoteDashboard = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected remote dashboard without auth to be rejected")
	}
}

func TestRemoteDashboardRequiresExplicitTLSMaterial(t *testing.T) {
	cfg := Default()
	cfg.Server.DashboardListen = "0.0.0.0:9090"
	cfg.Server.AllowRemoteDashboard = true
	cfg.Server.DashboardUser = "admin"
	cfg.Server.DashboardPass = "secret"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected remote dashboard without explicit certs to be rejected")
	}
	if !strings.Contains(err.Error(), "requires explicit server.cert and server.key") {
		t.Fatalf("expected explicit cert/key validation error, got %v", err)
	}
}

func TestTLSPairMustLoadSuccessfully(t *testing.T) {
	cfg := Default()
	cfg.Server.CertFile, cfg.Server.KeyFile = testutil.GenerateSelfSignedCertFiles(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid generated tls pair to validate: %v", err)
	}

	cfg.Server.CertFile = cfg.Server.KeyFile
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected mismatched tls pair to fail validation")
	}
}

func TestProbeInsecureRequiresExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Probe.Insecure = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected insecure probe TLS without opt-in to fail")
	}
	cfg.Probe.AllowInsecureTLS = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected insecure probe TLS with opt-in to validate: %v", err)
	}
}

func TestBenchInsecureRequiresExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Bench.Insecure = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected insecure bench TLS without opt-in to fail")
	}
	cfg.Bench.AllowInsecureTLS = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected insecure bench TLS with opt-in to validate: %v", err)
	}
}

func TestValidateProfiles(t *testing.T) {
	cfg := Default()
	cfg.ProbeProfiles = map[string]ProbeProfile{
		"prod-edge": {
			Target:        "https://example.com",
			DefaultFormat: "markdown",
			Thresholds: ProbeThresholds{
				RequireStatusMin: 200,
				RequireStatusMax: 299,
				MinCoverageRatio: 0.75,
			},
		},
	}
	cfg.BenchProfiles = map[string]BenchProfile{
		"staging-api": {
			Target:        "https://example.com",
			DefaultFormat: "json",
			Thresholds: BenchThresholds{
				MaxErrorRate: 0.10,
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected profiles to validate: %v", err)
	}
}

func TestValidateRejectsBrokenProfile(t *testing.T) {
	cfg := Default()
	cfg.ProbeProfiles = map[string]ProbeProfile{
		"broken": {
			Target: "",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid profile validation error")
	}
}
