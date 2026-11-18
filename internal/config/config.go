package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	ModeAll  = "all"
	ModeHTTP = "http"
	ModeGRPC = "grpc"
)

// Config is the process configuration. Environment variables override defaults.
type Config struct {
	Mode        string
	HTTPAddr    string
	GRPCAddr    string
	DNDBaseURL  string
	DNDVersion  string
	DiceBaseURL string
}

func Default() Config {
	return Config{
		Mode:        ModeAll,
		HTTPAddr:    "127.0.0.1:8080",
		GRPCAddr:    "127.0.0.1:50051",
		DNDBaseURL:  "https://www.dnd5eapi.co",
		DNDVersion:  "2014",
		DiceBaseURL: "https://randomapi.dev",
	}
}

func FromEnv() (Config, error) {
	cfg := Default()
	cfg.Mode = envOr("GATEWAY_MODE", cfg.Mode)
	cfg.HTTPAddr = envOr("GATEWAY_HTTP_ADDR", cfg.HTTPAddr)
	cfg.GRPCAddr = envOr("DICE_GRPC_ADDR", cfg.GRPCAddr)
	cfg.DNDBaseURL = strings.TrimRight(envOr("DND_BASE_URL", cfg.DNDBaseURL), "/")
	cfg.DNDVersion = envOr("DND_API_VERSION", cfg.DNDVersion)
	cfg.DiceBaseURL = strings.TrimRight(envOr("DICE_BASE_URL", cfg.DiceBaseURL), "/")
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	switch c.Mode {
	case ModeAll, ModeHTTP, ModeGRPC:
	default:
		return fmt.Errorf("GATEWAY_MODE must be all, http, or grpc")
	}
	if c.DNDVersion != "2014" && c.DNDVersion != "2024" {
		return fmt.Errorf("DND_API_VERSION must be 2014 or 2024")
	}
	if c.HTTPAddr == "" || c.GRPCAddr == "" || c.DNDBaseURL == "" || c.DiceBaseURL == "" {
		return fmt.Errorf("listen addresses and upstream base URLs are required")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
