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
