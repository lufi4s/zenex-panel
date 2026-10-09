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

const testPubKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyOnlyForTests zenex-backup"

// recCall is one command seen by recExec. For sftp, batch holds the batch file content
// as it was while sftp was running, and batchMode its permission bits.
type recCall struct {
	bin       string
	args      []string
	env       []string // extra NAME=VALUE entries passed with RunEnv; nil for Run
	timeout   time.Duration
	batch     string
	batchMode os.FileMode
}

// recExec records every command and lets a test decide each result.
type recExec struct {
	calls []recCall
	onRun func(bin string, args []string) (executor.Result, error)
}

func (r *recExec) Run(_ context.Context, bin string, args []string, timeout time.Duration) (executor.Result, error) {
	return r.record(bin, args, nil, timeout)
}

func (r *recExec) RunToFile(_ context.Context, bin string, args []string, env []string, _ string, timeout time.Duration) (executor.Result, error) {
	return r.record(bin, args, env, timeout)
}

func (r *recExec) RunEnv(_ context.Context, bin string, args []string, env []string, timeout time.Duration) (executor.Result, error) {
	return r.record(bin, args, env, timeout)
}

func (r *recExec) record(bin string, args []string, env []string, timeout time.Duration) (executor.Result, error) {
	c := recCall{bin: bin, args: append([]string(nil), args...), env: append([]string(nil), env...), timeout: timeout}
	if i := indexOfArg(args, "-b"); i >= 0 && i+1 < len(args) {
		if info, err := os.Stat(args[i+1]); err == nil {
			c.batchMode = info.Mode().Perm()
		}
		if b, err := os.ReadFile(args[i+1]); err == nil {
			c.batch = string(b)
		}
	}
	r.calls = append(r.calls, c)
	if r.onRun != nil {
		return r.onRun(bin, args)
	}
	return executor.Result{}, nil
}

func (r *recExec) RunInput(ctx context.Context, bin string, args []string, _ string, timeout time.Duration) (executor.Result, error) {
	return r.Run(ctx, bin, args, timeout)
}

func indexOfArg(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// remoteOps builds an Ops whose key folder and backup folder live in a temp dir.
func remoteOps(t *testing.T, ex commandRunner) (*Ops, string) {
	t.Helper()
	root := t.TempDir()
	keyDir := filepath.Join(root, "zenex-backup")
	o := &Ops{Exec: ex, Paths: Paths{
		WebRoot:      filepath.Join(root, "www"),
		BackupDir:    filepath.Join(root, "backups"),
		BackupKeyDir: keyDir,
	}}
	return o, keyDir
}

// withKey places a stand-in key pair so sftp operations pass the key check.
func withKey(t *testing.T, keyDir string) {
	t.Helper()
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, backupKeyName), []byte("PRIVATE"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeKeygen stands in for ssh-keygen: it writes the pair named by the -f argument.
func fakeKeygen(t *testing.T) func(bin string, args []string) (executor.Result, error) {
	return func(bin string, args []string) (executor.Result, error) {
		if bin != binSSHKeygen {
			return executor.Result{}, nil
		}
		priv := args[len(args)-1]
		if err := os.WriteFile(priv, []byte("PRIVATE"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(priv+".pub", []byte(testPubKey+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return executor.Result{}, nil
	}
}

func sftpTargetArgs(overrides map[string]string) map[string]string {
	args := map[string]string{
		"host":     "files.example.com",
		"port":     "22",
		"username": "backup",
		"path":     "/home/backup/zenex",
	}
	for k, v := range overrides {
		args[k] = v
	}
	return args
}

// ---------------------------------------------------------------------------
// backup.keygen
// ---------------------------------------------------------------------------

func TestBackupKeygenCreatesOnceAndReturnsPublicKey(t *testing.T) {
	ex := &recExec{onRun: fakeKeygen(t)}
	o, keyDir := remoteOps(t, ex)
	priv := filepath.Join(keyDir, backupKeyName)

	res, err := o.Do(context.Background(), "backup.keygen", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != testPubKey {
		t.Fatalf("output = %q, want the public key only", res.Output)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("expected one ssh-keygen run, got %d", len(ex.calls))
	}
	want := []string{binSSHKeygen, "-t", "ed25519", "-N", "", "-C", "zenex-backup", "-f", priv}
	if strings.Join(ex.calls[0].args, "\x00") != strings.Join(want[1:], "\x00") || ex.calls[0].bin != binSSHKeygen {
		t.Fatalf("ssh-keygen argv = %v, want %v", ex.calls[0].args, want)
	}
	if ex.calls[0].timeout != sshKeygenTime {
		t.Fatalf("timeout = %v", ex.calls[0].timeout)
	}

	// The second call must keep the existing pair.
	res2, err := o.Do(context.Background(), "backup.keygen", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("existing key regenerated: %d ssh-keygen runs", len(ex.calls))
	}
	if res2.Output != testPubKey {
		t.Fatalf("second output = %q", res2.Output)
	}

	if runtime.GOOS != "windows" {
		if info, err := os.Stat(keyDir); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("key folder mode = %v, %v", info, err)
		}
		if info, err := os.Stat(priv); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private key mode = %v, %v", info, err)
		}
	}
}

func TestBackupKeygenRejectsArgumentsAndMissingConfig(t *testing.T) {
	ex := &recExec{}
	o, _ := remoteOps(t, ex)
	if _, err := o.Do(context.Background(), "backup.keygen", map[string]string{"extra": "1"}); err == nil {
		t.Fatal("arguments accepted")
	}

	o.Paths.BackupKeyDir = ""
	if _, err := o.Do(context.Background(), "backup.keygen", nil); err == nil {
		t.Fatal("unconfigured key folder accepted")
	}
	if len(ex.calls) != 0 {
		t.Fatalf("commands ran despite invalid input: %v", ex.calls)
	}
}

func TestBackupKeygenFailureLeavesNoPartialKey(t *testing.T) {
	ex := &recExec{onRun: func(bin string, args []string) (executor.Result, error) {
		if err := os.WriteFile(args[len(args)-1], []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
		return executor.Result{ExitCode: 1, Stderr: "disk full"}, nil
	}}
	o, keyDir := remoteOps(t, ex)
	if _, err := o.Do(context.Background(), "backup.keygen", nil); err == nil {
		t.Fatal("failed generation reported as success")
	}
	if _, err := os.Stat(filepath.Join(keyDir, backupKeyName)); !os.IsNotExist(err) {
		t.Fatalf("partial private key left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(keyDir, backupKeyName+".pub")); !os.IsNotExist(err) {
		t.Fatalf("partial public key left behind: %v", err)
	}
}

func TestBackupKeygenRefusesIncompleteOrMalformedPair(t *testing.T) {
	cases := map[string]func(keyDir string){
		"private without public": func(keyDir string) {
			_ = os.WriteFile(filepath.Join(keyDir, backupKeyName), []byte("PRIVATE"), 0o600)
		},
		"malformed public key": func(keyDir string) {
			_ = os.WriteFile(filepath.Join(keyDir, backupKeyName), []byte("PRIVATE"), 0o600)
			_ = os.WriteFile(filepath.Join(keyDir, backupKeyName+".pub"), []byte("not a key\n"), 0o644)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			ex := &recExec{}
			o, keyDir := remoteOps(t, ex)
			if err := os.MkdirAll(keyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			setup(keyDir)
			if _, err := o.Do(context.Background(), "backup.keygen", nil); err == nil {
				t.Fatal("bad key state accepted")
			}
			if len(ex.calls) != 0 {
				t.Fatalf("ssh-keygen ran over an existing key: %v", ex.calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// backup.test
// ---------------------------------------------------------------------------

func TestBackupTestRejectsInvalidTargets(t *testing.T) {
	long := "/" + strings.Repeat("a", 200)
	cases := map[string]map[string]string{
		"host with space":       sftpTargetArgs(map[string]string{"host": "bad host"}),
		"host option injection": sftpTargetArgs(map[string]string{"host": "-oProxyCommand=x"}),
		"host with semicolon":   sftpTargetArgs(map[string]string{"host": "a;rm -rf /"}),
		"host empty":            sftpTargetArgs(map[string]string{"host": ""}),
		"host too long":         sftpTargetArgs(map[string]string{"host": strings.Repeat("a", 254)}),
		"host bracketed IPv6":   sftpTargetArgs(map[string]string{"host": "[2001:db8::1]"}),
		"port zero":             sftpTargetArgs(map[string]string{"port": "0"}),
		"port too high":         sftpTargetArgs(map[string]string{"port": "65536"}),
		"port not a number":     sftpTargetArgs(map[string]string{"port": "abc"}),
		"port with suffix":      sftpTargetArgs(map[string]string{"port": "22;"}),
		"username uppercase":    sftpTargetArgs(map[string]string{"username": "Root"}),
		"username digit first":  sftpTargetArgs(map[string]string{"username": "1user"}),
		"username empty":        sftpTargetArgs(map[string]string{"username": ""}),
		"username with symbol":  sftpTargetArgs(map[string]string{"username": "a;b"}),
		"path relative":         sftpTargetArgs(map[string]string{"path": "backups"}),
		"path traversal":        sftpTargetArgs(map[string]string{"path": "/home/../etc"}),
		"path not cleaned":      sftpTargetArgs(map[string]string{"path": "/home/a/./b"}),
		"path with space":       sftpTargetArgs(map[string]string{"path": "/home/a b"}),
		"path with quote":       sftpTargetArgs(map[string]string{"path": "/home/a\"b"}),
		"path with newline":     sftpTargetArgs(map[string]string{"path": "/home/a\nrm"}),
		"path with glob":        sftpTargetArgs(map[string]string{"path": "/home/*"}),
		"path too long":         sftpTargetArgs(map[string]string{"path": long}),
		"path empty":            sftpTargetArgs(map[string]string{"path": ""}),
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			ex := &recExec{}
			o, keyDir := remoteOps(t, ex)
			withKey(t, keyDir)
			if _, err := o.Do(context.Background(), "backup.test", args); err == nil {
				t.Fatal("invalid target accepted")
			}
			if len(ex.calls) != 0 {
				t.Fatalf("sftp ran despite invalid input: %v", ex.calls)
			}
		})
	}
}

func TestBackupTestAcceptsIPLiterals(t *testing.T) {
	for _, host := range []string{"10.0.0.5", "2001:db8::1"} {
		ex := &recExec{}
		o, keyDir := remoteOps(t, ex)
		withKey(t, keyDir)
		if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"host": host})); err != nil {
			t.Fatalf("host %q rejected: %v", host, err)
		}
	}
}

func TestBackupTestRequiresKey(t *testing.T) {
	ex := &recExec{}
	o, _ := remoteOps(t, ex)
	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(nil)); err == nil {
		t.Fatal("test ran without a key")
	}
	if len(ex.calls) != 0 {
		t.Fatalf("sftp ran without a key: %v", ex.calls)
	}
}

func TestBackupTestArgvAndBatchFile(t *testing.T) {
	var batchSeen string
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)

	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"port": "2222"})); err != nil {
		t.Fatal(err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("expected one sftp run, got %d", len(ex.calls))
	}
	c := ex.calls[0]
	batchSeen = c.batch
	if c.bin != binSFTP {
		t.Fatalf("bin = %s", c.bin)
	}
	if c.timeout != sftpTestTime {
		t.Fatalf("timeout = %v", c.timeout)
	}
	if batchSeen != "cd /home/backup/zenex\nls\n" {
		t.Fatalf("batch = %q", batchSeen)
	}
	if runtime.GOOS != "windows" && c.batchMode != 0o600 {
		t.Fatalf("batch file mode = %v", c.batchMode)
	}

	// Everything except the batch path must match exactly.
	wantPrefix := []string{
		"-i", filepath.Join(keyDir, backupKeyName),
		"-P", "2222",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + filepath.Join(keyDir, "known_hosts"),
		"-o", "ConnectTimeout=10",
		"-b",
	}
	got := c.args
	if len(got) != len(wantPrefix)+2 {
		t.Fatalf("argv length = %d: %v", len(got), got)
	}
	for i, w := range wantPrefix {
		if got[i] != w {
			t.Fatalf("argv[%d] = %q, want %q", i, got[i], w)
		}
	}
	if got[len(got)-1] != "backup@files.example.com" {
		t.Fatalf("destination = %q", got[len(got)-1])
	}
	if !strings.HasPrefix(got[len(wantPrefix)], keyDir) {
		t.Fatalf("batch file outside key folder: %s", got[len(wantPrefix)])
	}

	// The batch file must be removed once sftp has finished.
	entries, err := os.ReadDir(keyDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != backupKeyName {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestSFTPFailureKeywordMapping(t *testing.T) {
	cases := map[string]string{
		"backup@files.example.com: Permission denied (publickey,password).": msgAuth,
		"Authentication failed.": msgAuth,
		"ssh: connect to host files.example.com port 22: Connection refused":           msgConnect,
		"ssh: Could not resolve hostname files.example.com: Name or service not known": msgConnect,
		"Connection timed out during banner exchange":                                  msgConnect,
		"Couldn't canonicalise: No such file or directory":                             msgFolderMissed,
		"Can't change directory: Can't check target: Permission denied":                msgFolderDenied,
		"something unexpected at /secret/internal/path":                                msgTestFailed,
	}
	for stderr, want := range cases {
		t.Run(want, func(t *testing.T) {
			ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
				return executor.Result{ExitCode: 255, Stderr: stderr}, nil
			}}
			o, keyDir := remoteOps(t, ex)
			withKey(t, keyDir)
			_, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(nil))
			if err == nil {
				t.Fatal("failed test reported as success")
			}
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
			if strings.Contains(err.Error(), "/secret") {
				t.Fatal("raw stderr leaked into the error")
			}
		})
	}
}

func TestBackupTestTimeoutReportsShortMessage(t *testing.T) {
	ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{}, executor.ErrTimeout
	}}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	_, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(nil))
	if err == nil || err.Error() != msgTimedOut {
		t.Fatalf("error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// backup.upload
// ---------------------------------------------------------------------------

func uploadArgs(o *Ops, overrides map[string]string) map[string]string {
	args := sftpTargetArgs(map[string]string{
		"file": filepath.Join(o.Paths.BackupDir, "zx_shop", "shop-1.tar.gz"),
	})
	for k, v := range overrides {
		args[k] = v
	}
	return args
}

func writeArchive(t *testing.T, o *Ops) string {
	t.Helper()
	p := filepath.Join(o.Paths.BackupDir, "zx_shop", "shop-1.tar.gz")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBackupUploadUsesPartThenRename(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	local := writeArchive(t, o)

	if _, err := o.Do(context.Background(), "backup.upload", uploadArgs(o, nil)); err != nil {
		t.Fatal(err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("expected one sftp run, got %d", len(ex.calls))
	}
	c := ex.calls[0]
	if c.timeout != sftpUploadTime {
		t.Fatalf("timeout = %v", c.timeout)
	}
	want := "cd /home/backup/zenex\n" +
		"put " + local + " shop-1.tar.gz.part\n" +
		"rename shop-1.tar.gz.part shop-1.tar.gz\n"
	if c.batch != want {
		t.Fatalf("batch = %q, want %q", c.batch, want)
	}
	if c.args[len(c.args)-1] != "backup@files.example.com" {
		t.Fatalf("destination = %q", c.args[len(c.args)-1])
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("local archive removed by upload: %v", err)
	}
}

func TestBackupUploadFailureReturnsShortMessageAndKeepsLocalFile(t *testing.T) {
	ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{ExitCode: 1, Stderr: "Couldn't canonicalise: No such file or directory"}, nil
	}}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	local := writeArchive(t, o)

	_, err := o.Do(context.Background(), "backup.upload", uploadArgs(o, nil))
	if err == nil || err.Error() != msgFolderMissed {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("local archive removed after failed upload: %v", err)
	}
}

func TestBackupUploadDefaultFailureMessage(t *testing.T) {
	ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{ExitCode: 1, Stderr: "weird /secret detail"}, nil
	}}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	writeArchive(t, o)
	_, err := o.Do(context.Background(), "backup.upload", uploadArgs(o, nil))
	if err == nil || err.Error() != msgUploadFailed {
		t.Fatalf("error = %v", err)
	}
}

func TestBackupUploadRejectsInvalidInput(t *testing.T) {
	cases := map[string]func(o *Ops) map[string]string{
		"file outside backup root": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"file": filepath.Join(o.Paths.BackupDir, "..", "etc", "x.tar.gz")})
		},
		"file with wrong suffix": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"file": filepath.Join(o.Paths.BackupDir, "zx_shop", "shop.zip")})
		},
		"file name with space": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"file": filepath.Join(o.Paths.BackupDir, "zx_shop", "my shop.tar.gz")})
		},
		"file name with glob": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"file": filepath.Join(o.Paths.BackupDir, "zx_shop", "*.tar.gz")})
		},
		"file missing": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"file": filepath.Join(o.Paths.BackupDir, "zx_shop", "gone.tar.gz")})
		},
		"bad host": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"host": "h;x"})
		},
		"relative remote path": func(o *Ops) map[string]string {
			return uploadArgs(o, map[string]string{"path": "backups"})
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			ex := &recExec{}
			o, keyDir := remoteOps(t, ex)
			withKey(t, keyDir)
			writeArchive(t, o)
			if _, err := o.Do(context.Background(), "backup.upload", build(o)); err == nil {
				t.Fatal("invalid upload accepted")
			}
			if len(ex.calls) != 0 {
				t.Fatalf("sftp ran despite invalid input: %v", ex.calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// backup.delete (remote mode)
// ---------------------------------------------------------------------------

func remoteDeleteArgs(overrides map[string]string) map[string]string {
	args := sftpTargetArgs(map[string]string{
		"remote": "1",
		"path":   "/home/backup/zenex/zx_shop/shop-1.tar.gz",
	})
	for k, v := range overrides {
		args[k] = v
	}
	return args
}

func TestBackupDeleteRemoteArgsAndMissingFileTolerance(t *testing.T) {
	ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{ExitCode: 1, Stderr: "Couldn't stat remote file: No such file or directory"}, nil
	}}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)

	if _, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(nil)); err != nil {
		t.Fatalf("missing remote file reported as error: %v", err)
	}
	// The manifest is removed first, then the archive.
	if len(ex.calls) != 2 {
		t.Fatalf("expected two sftp runs, got %d", len(ex.calls))
	}
	if ex.calls[0].batch != "rm /home/backup/zenex/zx_shop/shop-1.json\n" {
		t.Fatalf("manifest batch = %q", ex.calls[0].batch)
	}
	if ex.calls[1].batch != "rm /home/backup/zenex/zx_shop/shop-1.tar.gz\n" {
		t.Fatalf("batch = %q", ex.calls[1].batch)
	}
	if ex.calls[1].timeout != sftpDeleteTime {
		t.Fatalf("timeout = %v", ex.calls[1].timeout)
	}
}

func TestBackupDeleteRemoteSuccessAndFailure(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	if _, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(nil)); err != nil {
		t.Fatal(err)
	}

	ex.onRun = func(string, []string) (executor.Result, error) {
		return executor.Result{ExitCode: 255, Stderr: "backup@files.example.com: Permission denied (publickey)."}, nil
	}
	_, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(nil))
	if err == nil || err.Error() != msgAuth {
		t.Fatalf("auth failure error = %v", err)
	}
}

func TestBackupDeleteRemoteRejectsInvalidPaths(t *testing.T) {
	cases := map[string]string{
		"not an archive":     "/home/backup/zenex/zx_shop/shop-1.zip",
		"traversal":          "/home/backup/zenex/../etc/shop-1.tar.gz",
		"relative":           "zx_shop/shop-1.tar.gz",
		"suffix only":        "/home/backup/zenex/.tar.gz",
		"space in name":      "/home/backup/zenex/my shop.tar.gz",
		"glob in name":       "/home/backup/zenex/*.tar.gz",
		"quote in name":      "/home/backup/zenex/a'b.tar.gz",
		"trailing separator": "/home/backup/zenex/zx_shop/",
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			ex := &recExec{}
			o, keyDir := remoteOps(t, ex)
			withKey(t, keyDir)
			if _, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(map[string]string{"path": p})); err == nil {
				t.Fatalf("path %q accepted", p)
			}
			if len(ex.calls) != 0 {
				t.Fatalf("sftp ran despite invalid path: %v", ex.calls)
			}
		})
	}
}

func TestBackupDeleteRejectsUnknownRemoteFlag(t *testing.T) {
	ex := &recExec{}
	o, _ := remoteOps(t, ex)
	if _, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(map[string]string{"remote": "2"})); err == nil {
		t.Fatal("unknown remote flag accepted")
	}
	if len(ex.calls) != 0 {
		t.Fatalf("commands ran: %v", ex.calls)
	}
}

func TestAllowedBinariesIncludeSSHTools(t *testing.T) {
	allowed := AllowedBinaries(nil)
	for _, want := range []string{binSSHKeygen, binSFTP, binSSHPass} {
		found := false
		for _, a := range allowed {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s missing from allowlist", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Password authentication (sshpass)
// ---------------------------------------------------------------------------

const testPassword = "Zx-test-Pass_w0rd!"

// assertPasswordRun checks that a run used sshpass with the password only in its environment.
func assertPasswordRun(t *testing.T, c recCall, password string) {
	t.Helper()
	if c.bin != binSSHPass {
		t.Fatalf("bin = %s, want %s", c.bin, binSSHPass)
	}
	if len(c.args) < 2 || c.args[0] != "-e" || c.args[1] != binSFTP {
		t.Fatalf("argv must start with -e %s, got %v", binSFTP, c.args)
	}
	if strings.Contains(strings.Join(c.args, "\x00"), password) {
		t.Fatal("password found in argv")
	}
	if len(c.env) != 1 || c.env[0] != "SSHPASS="+password {
		t.Fatalf("env = %v, want only SSHPASS for this run", c.env)
	}
	for _, banned := range []string{"-i", "BatchMode=yes"} {
		if indexOfArg(c.args, banned) >= 0 {
			t.Fatalf("password mode must not pass %q: %v", banned, c.args)
		}
	}
}

func TestBackupTestPasswordModeArgvAndEnv(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)

	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"password": testPassword})); err != nil {
		t.Fatal(err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("expected one run, got %d", len(ex.calls))
	}
	c := ex.calls[0]
	assertPasswordRun(t, c, testPassword)
	if c.timeout != sftpTestTime {
		t.Fatalf("timeout = %v", c.timeout)
	}
	if c.batch != "cd /home/backup/zenex\nls\n" {
		t.Fatalf("batch = %q", c.batch)
	}

	kh := filepath.Join(keyDir, knownHostsName)
	want := []string{
		"-e", binSFTP,
		"-P", "22",
		"-o", "PubkeyAuthentication=no",
		"-o", "PreferredAuthentications=password",
		"-o", "NumberOfPasswordPrompts=1",
		"-o", "BatchMode=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + kh,
		"-o", "ConnectTimeout=10",
		"-b", "", // batch path, checked separately
		"backup@files.example.com",
	}
	if len(c.args) != len(want) {
		t.Fatalf("argv length = %d, want %d: %v", len(c.args), len(want), c.args)
	}
	for i, w := range want {
		if w == "" {
			if !strings.HasPrefix(c.args[i], keyDir) {
				t.Fatalf("batch file outside key folder: %s", c.args[i])
			}
			continue
		}
		if c.args[i] != w {
			t.Fatalf("argv[%d] = %q, want %q", i, c.args[i], w)
		}
	}
	entries, err := os.ReadDir(keyDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != backupKeyName {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestBackupPasswordModeDoesNotNeedKeyFile(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"password": testPassword})); err != nil {
		t.Fatalf("password run without key pair failed: %v", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("expected one run, got %d", len(ex.calls))
	}
}

func TestEmptyPasswordUsesKeyMode(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"password": ""})); err != nil {
		t.Fatal(err)
	}
	if len(ex.calls) != 1 || ex.calls[0].bin != binSFTP || ex.calls[0].env != nil {
		t.Fatalf("empty password did not use key mode: %+v", ex.calls)
	}
	if indexOfArg(ex.calls[0].args, "-i") < 0 || indexOfArg(ex.calls[0].args, "BatchMode=yes") < 0 {
		t.Fatalf("key options missing: %v", ex.calls[0].args)
	}
}

func TestPasswordIsNotInheritedByLaterRuns(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)

	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"password": testPassword})); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(nil)); err != nil {
		t.Fatal(err)
	}
	if len(ex.calls) != 2 {
		t.Fatalf("expected two runs, got %d", len(ex.calls))
	}
	if ex.calls[1].bin != binSFTP || ex.calls[1].env != nil {
		t.Fatalf("key run carried environment: %+v", ex.calls[1])
	}
}

func TestPasswordModeAcrossOperations(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	local := writeArchive(t, o)

	if _, err := o.Do(context.Background(), "backup.upload", uploadArgs(o, map[string]string{"password": testPassword})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("local archive removed by upload: %v", err)
	}
	if _, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(map[string]string{"password": testPassword})); err != nil {
		t.Fatal(err)
	}
	// upload, manifest delete, archive delete
	if len(ex.calls) != 3 {
		t.Fatalf("expected three runs, got %d", len(ex.calls))
	}
	upload, del := ex.calls[0], ex.calls[2]
	assertPasswordRun(t, upload, testPassword)
	if upload.timeout != sftpUploadTime {
		t.Fatalf("upload timeout = %v", upload.timeout)
	}
	if !strings.Contains(upload.batch, "put "+local+" shop-1.tar.gz.part\n") {
		t.Fatalf("upload batch = %q", upload.batch)
	}
	assertPasswordRun(t, del, testPassword)
	if del.batch != "rm /home/backup/zenex/zx_shop/shop-1.tar.gz\n" {
		t.Fatalf("delete batch = %q", del.batch)
	}
	if del.timeout != sftpDeleteTime {
		t.Fatalf("delete timeout = %v", del.timeout)
	}
}

func TestInvalidPasswordRejectedBeforeAnyRun(t *testing.T) {
	bad := map[string]string{
		"too long":         strings.Repeat("a", maxPassword+1),
		"embedded LF":      "pass\nword",
		"embedded CR":      "pass\rword",
		"embedded NUL":     "pass\x00word",
		"trailing newline": "password\n",
	}
	ops := map[string]func(o *Ops, pw string) map[string]string{
		"backup.test": func(o *Ops, pw string) map[string]string {
			return sftpTargetArgs(map[string]string{"password": pw})
		},
		"backup.upload": func(o *Ops, pw string) map[string]string {
			return uploadArgs(o, map[string]string{"password": pw})
		},
		"backup.delete": func(o *Ops, pw string) map[string]string {
			return remoteDeleteArgs(map[string]string{"password": pw})
		},
	}
	for opName, build := range ops {
		for name, pw := range bad {
			t.Run(opName+"/"+name, func(t *testing.T) {
				ex := &recExec{}
				o, keyDir := remoteOps(t, ex)
				withKey(t, keyDir)
				writeArchive(t, o)
				_, err := o.Do(context.Background(), opName, build(o, pw))
				if err == nil {
					t.Fatal("invalid password accepted")
				}
				if err.Error() != "invalid password" {
					t.Fatalf("error = %q", err.Error())
				}
				if len(ex.calls) != 0 {
					t.Fatalf("command ran despite invalid password: %+v", ex.calls)
				}
			})
		}
	}
}

func TestPasswordLengthBoundary(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	pw := strings.Repeat("a", maxPassword)
	if _, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"password": pw})); err != nil {
		t.Fatalf("512-byte password rejected: %v", err)
	}
	if len(ex.calls) != 1 || len(ex.calls[0].env) != 1 || ex.calls[0].env[0] != "SSHPASS="+pw {
		t.Fatalf("512-byte password not passed through env")
	}
}

func TestPasswordModeErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		exit   int
		stderr string
		want   string
	}{
		{"sshpass wrong password", sshpassWrongPassword, "", msgAuth},
		{"ssh permission denied retry", 255, "Permission denied, please try again.", msgAuth},
		{"ssh permission denied list", 255, "Permission denied (password).", msgAuth},
		{"host key unknown", sshpassHostKeyUnknown, "", msgConnect},
		{"host key changed", sshpassHostKeyChanged, "", msgConnect},
		{"connection refused", 255, "connect to host files.example.com port 22: Connection refused", msgConnect},
		{"unknown failure", 3, "internal /secret/path " + testPassword, msgTestFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
				return executor.Result{ExitCode: tc.exit, Stderr: tc.stderr}, nil
			}}
			o, keyDir := remoteOps(t, ex)
			withKey(t, keyDir)
			_, err := o.Do(context.Background(), "backup.test", sftpTargetArgs(map[string]string{"password": testPassword}))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), testPassword) || strings.Contains(err.Error(), "/secret") {
				t.Fatal("secret or raw stderr leaked into the error")
			}
		})
	}
}

func TestPasswordModeAuthFailureOnDelete(t *testing.T) {
	ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{ExitCode: sshpassWrongPassword}, nil
	}}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	_, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(map[string]string{"password": testPassword}))
	if err == nil || err.Error() != msgAuth {
		t.Fatalf("error = %v, want %q", err, msgAuth)
	}
}

func TestPasswordModeMissingRemoteFileIsNotAnError(t *testing.T) {
	ex := &recExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{ExitCode: 1, Stderr: "Couldn't stat remote file: No such file or directory"}, nil
	}}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	if _, err := o.Do(context.Background(), "backup.delete", remoteDeleteArgs(map[string]string{"password": testPassword})); err != nil {
		t.Fatalf("missing remote file reported as error: %v", err)
	}
}
