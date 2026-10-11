package mail

import (
	"strings"
	"testing"
)

func goodSettings() Settings {
	return Settings{
		Host:   "mail.example.com",
		DBHost: "127.0.0.1",
		DBName: "zenex_panel",
		DBUser: "zenex_mail",
		DBPass: "s3cret-pass",
		MaxMB:  64,
	}
}

func TestValidHostname(t *testing.T) {
	for _, ok := range []string{"mail.example.com", "mx1.ozima.cloud"} {
		if !ValidHostname(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"localhost", "mail_example.com", "-x.example.com", "mail.example.com.", "a b.com", ""} {
		if ValidHostname(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSettingsValidation(t *testing.T) {
	if err := goodSettings().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(*Settings){
		"bad host":          func(s *Settings) { s.Host = "localhost" },
		"missing password":  func(s *Settings) { s.DBPass = "" },
		"space in password": func(s *Settings) { s.DBPass = "two words" },
		"size too small":    func(s *Settings) { s.MaxMB = 0 },
		"size too large":    func(s *Settings) { s.MaxMB = 101 },
	}
	for name, change := range bad {
		s := goodSettings()
		change(&s)
		if s.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func findFile(t *testing.T, files []File, path string) File {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no file %s", path)
	return File{}
}

func TestRenderPostfixMainRelaysOnlyForSignedInUsers(t *testing.T) {
	files, err := Render(goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	main := findFile(t, files, PostfixMain).Content
	for _, want := range []string{
		"myhostname = mail.example.com",
		"virtual_mailbox_domains = pgsql:" + PostfixDomains,
		"virtual_mailbox_maps = pgsql:" + PostfixMailboxes,
		"smtpd_sasl_type = dovecot",
		"smtpd_sasl_auth_enable = yes",
		"smtpd_tls_auth_only = yes",
		"reject_unauth_destination",
		"message_size_limit = 67108864",
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.cf missing %q", want)
		}
	}
	if strings.Contains(main, "permit_mynetworks, reject") {
		t.Error("main.cf lets anyone relay")
	}
}

func TestRenderKeepsDatabasePasswordOutOfMainConfig(t *testing.T) {
	files, err := Render(goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == PostfixDomains || f.Path == PostfixMailboxes || f.Path == DovecotSQL {
			continue
		}
		if strings.Contains(f.Content, "s3cret-pass") {
			t.Errorf("%s contains the database password", f.Path)
		}
	}
}

func TestRenderLookupFilesAreRestrictedAndUseTheRightKeys(t *testing.T) {
	files, err := Render(goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{PostfixDomains, PostfixMailboxes, DovecotSQL} {
		f := findFile(t, files, path)
		if f.Mode != 0o640 {
			t.Errorf("%s mode = %o, want 0640", path, f.Mode)
		}
		if !strings.Contains(f.Content, "s3cret-pass") {
			t.Errorf("%s has no database password", path)
		}
	}
	postfixDomains := findFile(t, files, PostfixDomains).Content
	if !strings.Contains(postfixDomains, "hosts = 127.0.0.1") || !strings.Contains(postfixDomains, "dbname = zenex_panel") {
		t.Errorf("postfix lookup keys wrong:\n%s", postfixDomains)
	}
	if !strings.Contains(postfixDomains, "'%s'") {
		t.Error("domain lookup does not use the domain placeholder")
	}
	mailboxes := findFile(t, files, PostfixMailboxes).Content
	if !strings.Contains(mailboxes, "'%d'") || !strings.Contains(mailboxes, "'%u'") {
		t.Error("mailbox lookup must use the domain and user placeholders")
	}
	dovecot := findFile(t, files, DovecotSQL).Content
	if !strings.Contains(dovecot, "connect = host=127.0.0.1 dbname=zenex_panel") {
		t.Errorf("dovecot connection string wrong:\n%s", dovecot)
	}
}

func TestRenderDovecotPasswordsAreArgon2id(t *testing.T) {
	files, err := Render(goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	dovecot := findFile(t, files, DovecotSQL).Content
	for _, want := range []string{"default_pass_scheme = ARGON2ID", "'{ARGON2ID}' || m.password_hash", "%n", "maildir:/var/mail/vhosts/%d/%n"} {
		if !strings.Contains(dovecot, want) {
			t.Errorf("dovecot sql missing %q", want)
		}
	}
	if strings.Contains(dovecot, "TEXT") || strings.Contains(dovecot, "plain") {
		t.Error("dovecot sql mentions a plain-text password scheme")
	}
}

func TestRenderSubmissionRequiresTLSAndLogin(t *testing.T) {
	files, err := Render(goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	master := findFile(t, files, PostfixMaster).Content
	for _, want := range []string{"submission inet", "smtpd_tls_security_level=encrypt", "smtpd_sasl_auth_enable=yes"} {
		if !strings.Contains(master, want) {
			t.Errorf("master.cf missing %q", want)
		}
	}
}

func TestRenderDKIMKeysPerDomain(t *testing.T) {
	files, err := Render(goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	dkim := findFile(t, files, RspamdDKIM).Content
	if !strings.Contains(dkim, "/var/lib/zenex/dkim/$domain/$selector.key") || !strings.Contains(dkim, "sign_authenticated = true") {
		t.Errorf("rspamd signing config wrong:\n%s", dkim)
	}
}

func TestRenderRefusesInvalidSettings(t *testing.T) {
	s := goodSettings()
	s.Host = "not a host"
	if _, err := Render(s); err == nil {
		t.Fatal("invalid settings rendered")
	}
}
