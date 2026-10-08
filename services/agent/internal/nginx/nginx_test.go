package nginx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

type call struct {
	bin  string
	args []string
}

// fakeExec records every call and returns scripted results in order.
type fakeExec struct {
	calls   []call
	results []executor.Result
	err     error
}

func (f *fakeExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	f.calls = append(f.calls, call{bin, args})
	if f.err != nil {
		return executor.Result{}, f.err
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r, nil
}

func TestReloadRefusedWhenConfigInvalid(t *testing.T) {
	ex := &fakeExec{results: []executor.Result{{ExitCode: 1, Stderr: "unexpected }"}}}

	err := Reload(context.Background(), ex)
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("Reload error = %v, want ErrConfigInvalid", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("expected only nginx -t to run, got %d calls", len(ex.calls))
	}
	for _, c := range ex.calls {
		if c.bin == binSystemctl {
			t.Fatal("systemctl reload was called after failed validation")
		}
	}
}

func TestReloadRunsAfterValidConfig(t *testing.T) {
	ex := &fakeExec{results: []executor.Result{{ExitCode: 0}, {ExitCode: 0}}}

	if err := Reload(context.Background(), ex); err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}
	if len(ex.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(ex.calls))
	}
	if ex.calls[0].bin != binNginx || strings.Join(ex.calls[0].args, " ") != "-t" {
		t.Errorf("first call = %s %v, want nginx -t", ex.calls[0].bin, ex.calls[0].args)
	}
	if ex.calls[1].bin != binSystemctl || strings.Join(ex.calls[1].args, " ") != "reload nginx" {
		t.Errorf("second call = %s %v, want systemctl reload nginx", ex.calls[1].bin, ex.calls[1].args)
	}
}

func TestTestPropagatesExecutorError(t *testing.T) {
	ex := &fakeExec{err: executor.ErrTimeout}
	if err := Test(context.Background(), ex); !errors.Is(err, executor.ErrTimeout) {
		t.Fatalf("Test error = %v, want wrapped ErrTimeout", err)
	}
}
