package manage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Turning mail on must create the DKIM key even before the mail server is installed.
func TestDKIMKeyDoesNotNeedTheMailServer(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.outputs["mail.dkim-key"] = "p=QUJD"
	value, err := m.MailDKIMValue(context.Background(), "ozima.cloud")
	if err != nil {
		t.Fatalf("DKIM without a mail server = %v", err)
	}
	if value != "p=QUJD" {
		t.Fatalf("value = %q", value)
	}
	if got := h.args["mail.dkim-key"][0]["domain"]; got != "ozima.cloud" {
		t.Fatalf("domain sent = %q", got)
	}
}

func mailTestManager() (*Manager, *scriptedHelper) {
	m, _, h := newFeatureTest("ready")
	m.MailServer = MailServerSettings{DBHost: "127.0.0.1", DBName: "zenex_panel", DBUser: "zenex_mail", DBPass: "s3cret-pass", MaxMB: 64}
	return m, h
}

func TestApplyMailServerPreparesThenAppliesForTheHost(t *testing.T) {
	m, h := mailTestManager()
	if err := m.ApplyMailServer(context.Background(), "mail.ozima.cloud"); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 2 || h.calls[0] != "mail.prepare" || h.calls[1] != "mail.apply" {
		t.Fatalf("calls = %v, want mail.prepare then mail.apply", h.calls)
	}
	if h.args["mail.prepare"][0]["host"] != "mail.ozima.cloud" {
		t.Fatalf("prepare args = %v", h.args["mail.prepare"])
	}
	files := h.args["mail.apply"][0]["files"]
	if !strings.Contains(files, "myhostname = mail.ozima.cloud") {
		t.Fatal("the configuration does not use the chosen host")
	}
	if h.args["mail.apply"][0]["dbpass"] != "s3cret-pass" {
		t.Fatal("the database password is not sent for redaction")
	}
}

func TestApplyMailServerRefusesABadHostBeforeTouchingTheServer(t *testing.T) {
	m, h := mailTestManager()
	for _, bad := range []string{"localhost", "Mail.Example.com", "mail.example.com; reboot", ""} {
		if err := m.ApplyMailServer(context.Background(), bad); !asRefusal(err) {
			t.Errorf("host %q: err = %v, want refusal", bad, err)
		}
	}
	if len(h.calls) != 0 {
		t.Fatalf("helper ran for a bad host: %v", h.calls)
	}
}

func TestApplyMailServerStopsIfPreparingFails(t *testing.T) {
	m, h := mailTestManager()
	h.fail["mail.prepare"] = errors.New("firewall refused")
	if err := m.ApplyMailServer(context.Background(), "mail.ozima.cloud"); err == nil {
		t.Fatal("prepare failure was ignored")
	}
	for _, c := range h.calls {
		if c == "mail.apply" {
			t.Fatal("configuration was applied after a failed prepare")
		}
	}
}
