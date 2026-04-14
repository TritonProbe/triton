package config

import "testing"

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
