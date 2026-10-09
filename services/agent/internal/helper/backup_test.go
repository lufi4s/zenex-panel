package helper

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// scriptExec records every command and lets a test decide each result.
type scriptExec struct {
	calls []string
	argv  [][]string
	onRun func(bin string, args []string) (executor.Result, error)
}

func (s *scriptExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	s.calls = append(s.calls, bin)
	s.argv = append(s.argv, append([]string{bin}, args...))
	if s.onRun != nil {
		return s.onRun(bin, args)
	}
	return executor.Result{}, nil
}

func (s *scriptExec) RunInput(ctx context.Context, bin string, args []string, _ string, timeout time.Duration) (executor.Result, error) {
	return s.Run(ctx, bin, args, timeout)
}

func (s *scriptExec) RunEnv(ctx context.Context, bin string, args []string, _ []string, timeout time.Duration) (executor.Result, error) {
	return s.Run(ctx, bin, args, timeout)
}

// testOps builds an Ops whose web, backup and caddy directories live in a temp dir.
func testOps(t *testing.T, ex commandRunner) (*Ops, string) {
	t.Helper()
	root := t.TempDir()
	o := &Ops{Exec: ex, Paths: Paths{
		WebRoot:        filepath.Join(root, "www"),
		PHPPoolDir:     filepath.Join(root, "pool", "%s"),
		CaddyAvailable: filepath.Join(root, "caddy-available"),
		CaddyEnabled:   filepath.Join(root, "caddy-enabled"),
		LogDir:         filepath.Join(root, "log"),
		BackupDir:      filepath.Join(root, "backups"),
	}}
	return o, root
}

func backupArgs(o *Ops, root string, overrides map[string]string) map[string]string {
	args := map[string]string{
		"user":     "zx_shop",
		"database": "zx_shop",
		"docroot":  o.docRoot("zx_shop"),
		"output":   filepath.Join(root, "backups", "zx_shop", "shop-1.tar.gz"),
	}
	for k, v := range overrides {
		args[k] = v
	}
	return args
}

func TestBackupCreateRejectsInvalidArguments(t *testing.T) {
	cases := map[string]func(root string, o *Ops) map[string]string{
		"system account": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"user": "root"})
		},
		"database of another site": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"database": "zx_other"})
		},
		"database with SQL": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"database": "zx_shop`;DROP"})
		},
		"docroot outside site": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"docroot": filepath.Join(root, "etc")})
		},
		"docroot of another site": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"docroot": filepath.Join(root, "www", "zx_other", "htdocs")})
		},
		"output outside backup root": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": filepath.Join(root, "elsewhere", "shop.tar.gz")})
		},
		"output in another site's folder": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": filepath.Join(root, "backups", "zx_other", "shop.tar.gz")})
		},
		"output with wrong suffix": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": filepath.Join(root, "backups", "zx_shop", "shop.zip")})
		},
		"output is only the suffix": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": filepath.Join(root, "backups", "zx_shop", ".tar.gz")})
		},
		"traversal in output": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": filepath.Join(root, "backups", "zx_shop", "..", "zx_other", "x.tar.gz")})
		},
		"traversal string in output": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": filepath.Join(root, "backups", "zx_shop") + "/../x.tar.gz"})
		},
		"relative output": func(root string, o *Ops) map[string]string {
			return backupArgs(o, root, map[string]string{"output": "shop.tar.gz"})
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			f := &scriptExec{}
			o, root := testOps(t, f)
			if _, err := o.Do(context.Background(), "backup.create", build(root, o)); err == nil {
				t.Fatal("invalid input accepted")
			}
			if len(f.calls) != 0 {
				t.Fatalf("commands ran despite invalid input: %v", f.calls)
			}
		})
	}
}

func TestBackupCreateSequence(t *testing.T) {
	f := &scriptExec{onRun: func(bin string, args []string) (executor.Result, error) {
		if bin == binTar {
			// Stand in for tar writing the archive.
			if err := os.WriteFile(args[1], []byte("archive-bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return executor.Result{}, nil
	}}
	o, root := testOps(t, f)
	args := backupArgs(o, root, nil)
	output := args["output"]
	dir := filepath.Dir(output)
	sqlPath := filepath.Join(dir, "database.sql")

	res, err := o.Do(context.Background(), "backup.create", args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != output+" 13" {
		t.Fatalf("output = %q", res.Output)
	}
	if len(f.argv) != 2 {
		t.Fatalf("expected dump then archive, got %d commands: %v", len(f.argv), f.argv)
	}

	dump := strings.Join(f.argv[0], " ")
	for _, want := range []string{
		binMariadbDump, "-uroot", "--protocol=socket", "--single-transaction", "--quick", "--routines",
		"--result-file=" + sqlPath, "zx_shop",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump command missing %q: %s", want, dump)
		}
	}

	wantTar := []string{binTar, "-czf", output, "-C", o.homeDir("zx_shop"), "htdocs", "-C", dir, "database.sql"}
	if strings.Join(f.argv[1], "\x00") != strings.Join(wantTar, "\x00") {
		t.Fatalf("archive command = %v, want %v", f.argv[1], wantTar)
	}

	if _, err := os.Stat(sqlPath); !os.IsNotExist(err) {
		t.Fatalf("temporary SQL file left behind: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(output)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("archive mode = %v, %v", info, err)
		}
		dinfo, err := os.Stat(dir)
		if err != nil || dinfo.Mode().Perm() != 0o700 {
			t.Fatalf("backup folder mode = %v, %v", dinfo, err)
		}
	}
}

func TestBackupCreateDumpFailureLeavesNoFiles(t *testing.T) {
	f := &scriptExec{onRun: func(bin string, _ []string) (executor.Result, error) {
		if bin == binMariadbDump {
			return executor.Result{ExitCode: 2, Stderr: "access denied"}, nil
		}
		return executor.Result{}, nil
	}}
	o, root := testOps(t, f)
	args := backupArgs(o, root, nil)
	if _, err := o.Do(context.Background(), "backup.create", args); err == nil {
		t.Fatal("dump failure reported as success")
	}
	if len(f.calls) != 1 {
		t.Fatalf("archive ran after failed dump: %v", f.calls)
	}
	assertDirEmpty(t, filepath.Dir(args["output"]))
}

func TestBackupCreateArchiveFailureRemovesPartialFiles(t *testing.T) {
	f := &scriptExec{onRun: func(bin string, args []string) (executor.Result, error) {
		if bin == binTar {
			if err := os.WriteFile(args[1], []byte("partial"), 0o600); err != nil {
				t.Fatal(err)
			}
			return executor.Result{ExitCode: 2, Stderr: "disk full"}, nil
		}
		return executor.Result{}, nil
	}}
	o, root := testOps(t, f)
	args := backupArgs(o, root, nil)
	if _, err := o.Do(context.Background(), "backup.create", args); err == nil {
		t.Fatal("archive failure reported as success")
	}
	assertDirEmpty(t, filepath.Dir(args["output"]))
}

func TestBackupCreateRefusesToOverwrite(t *testing.T) {
	f := &scriptExec{}
	o, root := testOps(t, f)
	args := backupArgs(o, root, nil)
	if err := os.MkdirAll(filepath.Dir(args["output"]), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(args["output"], []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Do(context.Background(), "backup.create", args); err == nil {
		t.Fatal("existing backup overwritten")
	}
	if len(f.calls) != 0 {
		t.Fatalf("commands ran over an existing backup: %v", f.calls)
	}
	if b, _ := os.ReadFile(args["output"]); string(b) != "old" {
		t.Fatal("existing backup changed")
	}
}

func TestBackupDeleteValidation(t *testing.T) {
	o, root := testOps(t, &scriptExec{})
	dir := filepath.Join(root, "backups", "zx_shop")
	for _, p := range []string{
		filepath.Join(root, "etc", "passwd.tar.gz"),
		filepath.Join(dir, "x.zip"),
		dir + "/../x.tar.gz",
		dir + "/../../x.tar.gz",
		"",
	} {
		if _, err := o.Do(context.Background(), "backup.delete", map[string]string{"path": p}); err == nil {
			t.Errorf("path %q accepted", p)
		}
	}
}

func TestBackupDeleteRemovesArchiveAndIgnoresMissing(t *testing.T) {
	o, root := testOps(t, &scriptExec{})
	dir := filepath.Join(root, "backups", "zx_shop")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "shop-1.tar.gz")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Do(context.Background(), "backup.delete", map[string]string{"path": p}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("archive not removed")
	}
	if _, err := o.Do(context.Background(), "backup.delete", map[string]string{"path": p}); err != nil {
		t.Fatalf("missing archive reported as error: %v", err)
	}
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // the folder was never created, which also means nothing was left behind
	}
	if len(entries) != 0 {
		t.Fatalf("files left behind in %s: %d", dir, len(entries))
	}
}
