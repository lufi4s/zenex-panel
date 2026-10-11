package update

import (
	"context"
	"errors"
	"testing"
)

// fakeHelper answers the status calls and records the arguments of panel.update-start.
type fakeHelper struct {
	state     string
	startArgs map[string]string
	started   int
}

func (f *fakeHelper) Output(_ context.Context, op string, _ map[string]string) (string, error) {
	switch op {
	case "panel.version":
		return "abc1234", nil
	case "panel.latest":
		return "def5678", nil
	case "panel.update-status":
		return `{"state":"` + f.state + `","log":""}`, nil
	}
	return "", errors.New("unexpected op " + op)
}

func (f *fakeHelper) Do(_ context.Context, op string, args map[string]string) (string, error) {
	if op == "panel.update-start" {
		f.started++
		f.startArgs = args
	}
	return "", nil
}

func TestStartWithMailPassesTheHostToTheHelper(t *testing.T) {
	h := &fakeHelper{state: "idle"}
	if err := New(h).StartWithMail(context.Background(), "mail.example.com"); err != nil {
		t.Fatal(err)
	}
	if h.started != 1 || h.startArgs["mail_host"] != "mail.example.com" {
		t.Fatalf("start called %d times with %v", h.started, h.startArgs)
	}
}

func TestPlainStartSendsNoMailHost(t *testing.T) {
	h := &fakeHelper{state: "idle"}
	if err := New(h).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.startArgs["mail_host"]; ok {
		t.Fatalf("plain update sent a mail host: %v", h.startArgs)
	}
}

func TestStartRefusesWhileAnUpdateRuns(t *testing.T) {
	h := &fakeHelper{state: "running"}
	if err := New(h).StartWithMail(context.Background(), "mail.example.com"); !errors.Is(err, ErrRunning) {
		t.Fatalf("err = %v, want ErrRunning", err)
	}
	if h.started != 0 {
		t.Fatal("update started although one is running")
	}
}
