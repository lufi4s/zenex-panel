package main

import (
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/config"
)

func TestMailServerSettingsComeFromTheEnvironment(t *testing.T) {
	s := mailServerSettings(config.Config{MailDBPassword: "abc123"})
	if s.DBPass != "abc123" || s.DBUser != "zenex_mail" || s.DBHost != "127.0.0.1" || s.DBName != "zenex_panel" || s.MaxMB != 64 {
		t.Fatalf("settings = %+v", s)
	}
}
