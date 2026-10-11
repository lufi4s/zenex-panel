package helper

import (
	"context"
	"strings"
	"testing"
)

func TestUpdateStartPassesTheMailHostToTheInstaller(t *testing.T) {
	f := &fakeExec{}
	o := &Ops{Exec: f, Paths: DefaultPaths()}
	if _, err := o.Do(context.Background(), "panel.update-start", map[string]string{"mail_host": "mail.example.com"}); err != nil {
		t.Fatal(err)
	}
	var run string
	for _, c := range f.calls {
		if strings.HasPrefix(c, binSystemdRun+" ") {
			run = c
		}
	}
	if !strings.Contains(run, "--setenv=ZENEX_MAIL_HOST=mail.example.com") || !strings.Contains(run, updateScript) {
		t.Fatalf("systemd-run call = %q", run)
	}
}

func TestUpdateStartWithoutMailHostSetsNoEnvironment(t *testing.T) {
	f := &fakeExec{}
	o := &Ops{Exec: f, Paths: DefaultPaths()}
	if _, err := o.Do(context.Background(), "panel.update-start", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "ZENEX_MAIL_HOST") {
			t.Fatalf("plain update sets the mail host: %q", c)
		}
	}
}

func TestUpdateStartRejectsABadMailHostBeforeRunningAnything(t *testing.T) {
	for _, bad := range []string{"localhost", "mail.example.com; rm -rf /", "Mail.Example.com", "a b.com", "mail.example.com\nX=1"} {
		f := &fakeExec{}
		o := &Ops{Exec: f, Paths: DefaultPaths()}
		if _, err := o.Do(context.Background(), "panel.update-start", map[string]string{"mail_host": bad}); err == nil {
			t.Errorf("host %q accepted", bad)
		}
		if len(f.calls) != 0 {
			t.Errorf("programs ran for host %q: %v", bad, f.calls)
		}
	}
}
