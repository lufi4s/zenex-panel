package config

import "testing"

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
	if cfg.Env != "development" || cfg.ListenAddr != "127.0.0.1:8080" || cfg.TLSEnabled() {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRejectsBadEnv(t *testing.T) {
	if _, err := loadFrom(lookupMap(map[string]string{"ZENEX_ENV": "staging"})); err == nil {
		t.Fatal("expected error for unknown ZENEX_ENV")
	}
}

func TestProductionRequiresDatabaseAndTLS(t *testing.T) {
	if _, err := loadFrom(lookupMap(map[string]string{"ZENEX_ENV": "production"})); err == nil {
		t.Fatal("production without database must fail")
	}
	if _, err := loadFrom(lookupMap(map[string]string{
		"ZENEX_ENV": "production", "ZENEX_DATABASE_URL": "postgres://x",
	})); err == nil {
		t.Fatal("production without TLS must fail")
	}
	cfg, err := loadFrom(lookupMap(map[string]string{
		"ZENEX_ENV": "production", "ZENEX_DATABASE_URL": "postgres://x",
		"ZENEX_TLS_CERT": "/c.pem", "ZENEX_TLS_KEY": "/k.pem",
	}))
	if err != nil || !cfg.TLSEnabled() {
		t.Fatalf("valid production config rejected: %v", err)
	}
}

func TestTLSPairMustBeComplete(t *testing.T) {
	if _, err := loadFrom(lookupMap(map[string]string{"ZENEX_TLS_CERT": "/c.pem"})); err == nil {
		t.Fatal("cert without key accepted")
	}
}

func TestInvalidListenAddr(t *testing.T) {
	if _, err := loadFrom(lookupMap(map[string]string{"ZENEX_API_ADDR": "not-an-addr"})); err == nil {
		t.Fatal("invalid listen address accepted")
	}
}
