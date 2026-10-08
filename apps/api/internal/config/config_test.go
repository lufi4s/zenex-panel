package config

import (
	"testing"
)

func lookupMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := loadFrom(lookupMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "development" || cfg.ListenAddr != "127.0.0.1:8080" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRejectsBadEnv(t *testing.T) {
	_, err := loadFrom(lookupMap(map[string]string{"ZENEX_ENV": "staging"}))
	if err == nil {
		t.Fatal("expected error for unknown ZENEX_ENV")
	}
}

func TestProductionRequiresDatabase(t *testing.T) {
	_, err := loadFrom(lookupMap(map[string]string{"ZENEX_ENV": "production"}))
	if err == nil {
		t.Fatal("production without ZENEX_DATABASE_URL must fail")
	}
	_, err = loadFrom(lookupMap(map[string]string{
		"ZENEX_ENV":          "production",
		"ZENEX_DATABASE_URL": "postgres://localhost/zenex",
	}))
	if err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
}

func TestInvalidListenAddr(t *testing.T) {
	_, err := loadFrom(lookupMap(map[string]string{"ZENEX_API_ADDR": "not-an-addr"}))
	if err == nil {
		t.Fatal("invalid listen address accepted")
	}
}
