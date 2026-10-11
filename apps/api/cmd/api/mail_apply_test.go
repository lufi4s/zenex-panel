package main

import (
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/config"
)

func TestMailServerSettingsComeFromTheEnvironment(t *testing.T) {
	s := mailServerSettings(config.Config{MailHost: "mail.example.com", MailDBPassword: "abc123"})
	if s.Host != "mail.example.com" || s.DBPass != "abc123" || s.DBUser != "zenex_mail" || s.DBHost != "127.0.0.1" || s.MaxMB != 64 {
		t.Fatalf("settings = %+v", s)
	}
}
