package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("GATEWAY_MODE", "")
	t.Setenv("DND_API_VERSION", "")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeAll || cfg.HTTPAddr != "127.0.0.1:8080" || cfg.DNDVersion != "2014" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestFromEnvRejectsBadMode(t *testing.T) {
	t.Setenv("GATEWAY_MODE", "both")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected invalid mode")
	}
}
