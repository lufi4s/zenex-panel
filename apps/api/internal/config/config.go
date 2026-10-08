// Package config loads API settings from the environment.
// Secrets are never stored in this struct's String output or logs.
package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

type Config struct {
	Env         string // "development" or "production"
	ListenAddr  string
	DatabaseURL string
	TLSCertFile string
	TLSKeyFile  string
}

// TLSEnabled reports whether the server terminates TLS itself.
func (c Config) TLSEnabled() bool { return c.TLSCertFile != "" && c.TLSKeyFile != "" }

// Load reads configuration from ZENEX_* environment variables.
func Load() (Config, error) {
	return loadFrom(os.LookupEnv)
}

func loadFrom(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}

	cfg := Config{
		Env:         get("ZENEX_ENV", "development"),
		ListenAddr:  get("ZENEX_API_ADDR", "127.0.0.1:8080"),
		DatabaseURL: get("ZENEX_DATABASE_URL", ""),
		TLSCertFile: get("ZENEX_TLS_CERT", ""),
		TLSKeyFile:  get("ZENEX_TLS_KEY", ""),
	}

	if cfg.Env != "development" && cfg.Env != "production" {
		return Config{}, fmt.Errorf("ZENEX_ENV must be development or production, got %q", cfg.Env)
	}
	if _, _, err := net.SplitHostPort(cfg.ListenAddr); err != nil {
		return Config{}, fmt.Errorf("ZENEX_API_ADDR invalid: %w", err)
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("ZENEX_TLS_CERT and ZENEX_TLS_KEY must be set together")
	}
	if cfg.Env == "production" {
		if cfg.DatabaseURL == "" {
			return Config{}, fmt.Errorf("ZENEX_DATABASE_URL is required in production")
		}
		if !cfg.TLSEnabled() {
			return Config{}, fmt.Errorf("ZENEX_TLS_CERT and ZENEX_TLS_KEY are required in production")
		}
	}
	return cfg, nil
}
