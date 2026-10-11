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
	if err := m.ApplyMailServer(context.Background(), "mail.ozima.cloud", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.calls, ",") != "mail.prepare,mail.apply,webmail.setup" {
		t.Fatalf("calls = %v, want mail.prepare, mail.apply, then webmail.setup", h.calls)
	}
	if h.args["webmail.setup"][0]["host"] != "mail.ozima.cloud" {
		t.Fatalf("webmail published for %v", h.args["webmail.setup"])
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
		if err := m.ApplyMailServer(context.Background(), bad, nil); !asRefusal(err) {
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
	if err := m.ApplyMailServer(context.Background(), "mail.ozima.cloud", nil); err == nil {
		t.Fatal("prepare failure was ignored")
	}
	for _, c := range h.calls {
		if c == "mail.apply" {
			t.Fatal("configuration was applied after a failed prepare")
		}
	}
}

func TestApplyMailServerPutsEveryDomainsKeyWhereRspamdReadsItFirst(t *testing.T) {
	m, h := mailTestManager()
	h.outputs["mail.dkim-key"] = "p=QUJD"
	if err := m.ApplyMailServer(context.Background(), "mail.ozima.cloud", []string{"ozima.cloud", "example.org"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"mail.prepare", "mail.dkim-key", "mail.dkim-key", "mail.apply", "webmail.setup"}
	if strings.Join(h.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", h.calls, want)
	}
	if h.args["mail.dkim-key"][0]["domain"] != "ozima.cloud" || h.args["mail.dkim-key"][1]["domain"] != "example.org" {
		t.Fatalf("keys prepared for %v", h.args["mail.dkim-key"])
	}
}

func TestApplyMailServerReportsAWebmailFailure(t *testing.T) {
	m, h := mailTestManager()
	h.fail["webmail.setup"] = errors.New("webmail is not installed on this server. Run the panel update, then try again")
	err := m.ApplyMailServer(context.Background(), "mail.ozima.cloud", nil)
	if err == nil || !strings.Contains(err.Error(), "panel update") {
		t.Fatalf("err = %v, want the webmail message", err)
	}
}
