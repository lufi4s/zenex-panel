package helper

import (
	"context"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

func TestWPUpdateRejectsInvalidArguments(t *testing.T) {
	cases := map[string]map[string]string{
		"system account":          {"user": "root", "path": "/var/www/root/htdocs"},
		"docroot of another site": {"user": "zx_shop", "path": "/var/www/zx_other/htdocs"},
		"docroot traversal":       {"user": "zx_shop", "path": "/var/www/zx_shop/htdocs/../../../etc"},
		"missing docroot":         {"user": "zx_shop"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			f := &scriptExec{}
			o, _ := testOps(t, f)
			if _, err := o.Do(context.Background(), "wp.update", args); err == nil {
				t.Fatal("invalid input accepted")
			}
			if len(f.calls) != 0 {
				t.Fatalf("commands ran despite invalid input: %v", f.argv)
			}
		})
	}
}

func TestWPUpdateRunsCoreThenPluginsThenThemesAsSiteAccount(t *testing.T) {
	f := &scriptExec{}
	o, _ := testOps(t, f)
	docroot := o.docRoot("zx_shop")
	if _, err := o.Do(context.Background(), "wp.update", map[string]string{"user": "zx_shop", "path": docroot}); err != nil {
		t.Fatal(err)
	}
	if len(f.argv) != 3 {
		t.Fatalf("expected three update commands, got %d: %v", len(f.argv), f.argv)
	}
	wantSteps := []string{"core update --quiet", "plugin update --all --quiet", "theme update --all --quiet"}
	for i, argv := range f.argv {
		if argv[0] != binRunuser || argv[1] != "-u" || argv[2] != "zx_shop" {
			t.Fatalf("step %d not run as site account: %v", i, argv)
		}
		joined := strings.Join(argv, " ")
		if !strings.Contains(joined, binWP+" --path="+docroot+" "+wantSteps[i]) {
			t.Errorf("step %d = %q, want %q", i, joined, wantSteps[i])
		}
		if strings.Contains(joined, "--skip-plugins") {
			t.Errorf("step %d must not skip plugins", i)
		}
	}
}

func TestWPUpdateIgnoresNonZeroExitWhenNothingToUpdate(t *testing.T) {
	f := &scriptExec{onRun: func(_ string, args []string) (executor.Result, error) {
		if strings.Contains(strings.Join(args, " "), "plugin update") {
			return executor.Result{ExitCode: 1, Stdout: "No plugins updated."}, nil
		}
		return executor.Result{}, nil
	}}
	o, _ := testOps(t, f)
	res, err := o.Do(context.Background(), "wp.update", map[string]string{"user": "zx_shop", "path": o.docRoot("zx_shop")})
	if err != nil {
		t.Fatalf("nothing-to-update exit code failed the op: %v", err)
	}
	if res.Output != "" {
		t.Fatalf("output = %q, want empty", res.Output)
	}
	if len(f.calls) != 3 {
		t.Fatalf("later steps skipped after a non-zero exit: %d calls", len(f.calls))
	}
}

func TestWPUpdateFailsWhenWPCLICannotRun(t *testing.T) {
	f := &scriptExec{onRun: func(_ string, args []string) (executor.Result, error) {
		if strings.Contains(strings.Join(args, " "), "core update") {
			return executor.Result{ExitCode: 127, Stderr: "wp: not found"}, nil
		}
		return executor.Result{}, nil
	}}
	o, _ := testOps(t, f)
	if _, err := o.Do(context.Background(), "wp.update", map[string]string{"user": "zx_shop", "path": o.docRoot("zx_shop")}); err == nil {
		t.Fatal("missing wp binary reported as success")
	}
	if len(f.calls) != 1 {
		t.Fatalf("later steps ran after wp could not start: %d calls", len(f.calls))
	}
}

func TestWPUpdateTimeoutIsReported(t *testing.T) {
	f := &scriptExec{onRun: func(_ string, _ []string) (executor.Result, error) {
		return executor.Result{}, executor.ErrTimeout
	}}
	o, _ := testOps(t, f)
	if _, err := o.Do(context.Background(), "wp.update", map[string]string{"user": "zx_shop", "path": o.docRoot("zx_shop")}); err == nil {
		t.Fatal("timeout reported as success")
	}
}
