package helper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// Off-server backup target. The helper owns one ed25519 key pair; only the
// public key ever leaves the server.
const (
	backupKeyName    = "id_ed25519"
	backupKeyComment = "zenex-backup"
	knownHostsName   = "known_hosts"
	batchFilePrefix  = "sftp-"

	sshKeygenTime  = 30 * time.Second
	sftpTestTime   = 60 * time.Second
	sftpUploadTime = 30 * time.Minute
	sftpDeleteTime = 60 * time.Second

	maxRemotePath = 200
)

// Messages returned to the panel. Raw sftp output is never passed through.
const (
	msgConnect      = "could not connect"
	msgAuth         = "authentication failed"
	msgFolderMissed = "remote folder does not exist"
	msgFolderDenied = "remote folder is not accessible"
	msgTimedOut     = "operation timed out"
	msgTestFailed   = "connection test failed"
	msgUploadFailed = "upload failed"
	msgDeleteFailed = "remote delete failed"
)

var (
	sftpHostRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
	sftpPortRe     = regexp.MustCompile(`^[1-9][0-9]{0,4}$`)
	sftpUserRe     = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	remotePathRe   = regexp.MustCompile(`^/[A-Za-z0-9._@+/-]*$`)
	archiveNameRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	localDenyChars = " \t\r\n\"'\\*?[]{}`;&|<>$()#"
)

// remoteTarget is the validated destination shared by test, upload and delete.
type remoteTarget struct {
	host     string
	port     int
	username string
}

// backupKeyPath returns the private key path under the configured key folder.
func (o *Ops) backupKeyPath() (string, error) {
	if o.Paths.BackupKeyDir == "" {
		return "", errors.New("backup key folder is not configured")
	}
	return filepath.Join(filepath.Clean(o.Paths.BackupKeyDir), backupKeyName), nil
}

// secureOwnership sets the mode and, when running as root, owner root.
func secureOwnership(p string, mode os.FileMode) error {
	if err := os.Chmod(p, mode); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		return os.Chown(p, 0, 0)
	}
	return nil
}

func pathExists(p string) (bool, error) {
	_, err := os.Lstat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ---------------------------------------------------------------------------
// backup.keygen
// ---------------------------------------------------------------------------

// backupKeygen makes sure the backup key pair exists and returns the public key.
// An existing key is never regenerated.
func (o *Ops) backupKeygen(ctx context.Context, args map[string]string) (Result, error) {
	if len(args) != 0 {
		return Result{}, errors.New("backup.keygen takes no arguments")
	}
	priv, err := o.backupKeyPath()
	if err != nil {
		return Result{}, err
	}
	dir := filepath.Dir(priv)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create key folder: %w", err)
	}
	if err := secureOwnership(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("secure key folder: %w", err)
	}

	pub := priv + ".pub"
	privExists, err := pathExists(priv)
	if err != nil {
		return Result{}, err
	}
	pubExists, err := pathExists(pub)
	if err != nil {
		return Result{}, err
	}
	switch {
	case privExists && pubExists:
		// Already created: keep it.
	case privExists || pubExists:
		return Result{}, errors.New("backup key pair is incomplete; remove the stale file and retry")
	default:
		if err := o.generateBackupKey(ctx, priv, pub); err != nil {
			return Result{}, err
		}
	}

	key, err := readPublicKey(pub)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: key}, nil
}

// generateBackupKey creates the pair with ssh-keygen. Neither file may exist yet.
// On any failure the partial files are removed so the next call starts clean.
func (o *Ops) generateBackupKey(ctx context.Context, priv, pub string) error {
	if err := o.runKeygen(ctx, priv); err != nil {
		_ = os.Remove(priv)
		_ = os.Remove(pub)
		return err
	}
	if err := secureOwnership(priv, 0o600); err != nil {
		_ = os.Remove(priv)
		_ = os.Remove(pub)
		return fmt.Errorf("secure backup key: %w", err)
	}
	if err := secureOwnership(pub, 0o644); err != nil {
		_ = os.Remove(priv)
		_ = os.Remove(pub)
		return fmt.Errorf("secure backup public key: %w", err)
	}
	return nil
}

func (o *Ops) runKeygen(ctx context.Context, priv string) error {
	res, err := o.Exec.Run(ctx, binSSHKeygen, []string{
		"-t", "ed25519", "-N", "", "-C", backupKeyComment, "-f", priv,
	}, sshKeygenTime)
	if err != nil {
		return fmt.Errorf("key generation failed: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("key generation failed: %s", trim(res.Stderr))
	}
	return nil
}

// readPublicKey returns the single-line public key. The private key is never read.
func readPublicKey(pub string) (string, error) {
	data, err := os.ReadFile(pub)
	if err != nil {
		return "", errors.New("backup public key unreadable")
	}
	line := strings.TrimSpace(string(data))
	if line == "" || strings.ContainsAny(line, "\r\n") || !strings.HasPrefix(line, "ssh-ed25519 ") {
		return "", errors.New("backup public key is malformed")
	}
	return line, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// parseRemoteTarget validates host, port and username. The path is checked by the caller.
func parseRemoteTarget(args map[string]string) (remoteTarget, error) {
	host := args["host"]
	if !sftpHostRe.MatchString(host) {
		if ip := net.ParseIP(host); ip == nil {
			return remoteTarget{}, errors.New("invalid host")
		}
	}
	if !sftpPortRe.MatchString(args["port"]) {
		return remoteTarget{}, errors.New("invalid port")
	}
	port, _ := strconv.Atoi(args["port"])
	if port < 1 || port > 65535 {
		return remoteTarget{}, errors.New("invalid port")
	}
	if !sftpUserRe.MatchString(args["username"]) {
		return remoteTarget{}, errors.New("invalid username")
	}
	return remoteTarget{host: host, port: port, username: args["username"]}, nil
}

// validateRemotePath accepts a clean absolute POSIX path made only of safe characters.
// Spaces, quotes and glob characters are refused because sftp would split or expand them.
func validateRemotePath(p string) error {
	if p == "" || len(p) > maxRemotePath || !path.IsAbs(p) || path.Clean(p) != p ||
		strings.Contains(p, "..") || !remotePathRe.MatchString(p) {
		return errors.New("invalid remote path")
	}
	return nil
}

// validateRemoteArchive additionally requires the archive suffix on the last element.
func validateRemoteArchive(p string) error {
	if err := validateRemotePath(p); err != nil {
		return err
	}
	base := path.Base(p)
	if !strings.HasSuffix(base, backupSuffix) || len(base) <= len(backupSuffix) {
		return errors.New("invalid remote backup path")
	}
	return nil
}

// validateLocalArchive applies the backup path rules and refuses names that sftp
// would split or glob. Windows separators are normalised first so tests on Windows work.
func (o *Ops) validateLocalArchive(p string) (string, error) {
	clean, err := o.backupPath(p)
	if err != nil {
		return "", err
	}
	slashed := filepath.ToSlash(clean)
	if strings.ContainsAny(slashed, localDenyChars) || strings.ContainsFunc(slashed, isControl) {
		return "", errors.New("invalid backup path")
	}
	if !archiveNameRe.MatchString(filepath.Base(clean)) {
		return "", errors.New("invalid backup file name")
	}
	return clean, nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// ---------------------------------------------------------------------------
// sftp runner
// ---------------------------------------------------------------------------

// runSFTP writes the batch to a root-only temp file under the key folder, runs
// sftp against it and removes it again. A non-zero sftp exit is returned in the
// Result so the caller can classify stderr. Errors are only for prerequisites
// and for transport failures.
func (o *Ops) runSFTP(ctx context.Context, t remoteTarget, batch []string, timeout time.Duration, def string) (executor.Result, error) {
	keyPath, err := o.backupKeyPath()
	if err != nil {
		return executor.Result{}, err
	}
	dir := filepath.Dir(keyPath)
	if ok, err := pathExists(keyPath); err != nil || !ok {
		return executor.Result{}, errors.New("backup key missing: run backup.keygen first")
	}

	tmp, err := os.MkdirTemp(dir, batchFilePrefix)
	if err != nil {
		return executor.Result{}, errors.New("prepare transfer failed")
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	bf, err := os.CreateTemp(tmp, "batch-*")
	if err != nil {
		return executor.Result{}, errors.New("prepare transfer failed")
	}
	if _, err := bf.WriteString(strings.Join(batch, "\n") + "\n"); err != nil {
		bf.Close()
		return executor.Result{}, errors.New("prepare transfer failed")
	}
	if err := bf.Close(); err != nil {
		return executor.Result{}, errors.New("prepare transfer failed")
	}

	argv := []string{
		"-i", keyPath,
		"-P", strconv.Itoa(t.port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + filepath.Join(dir, knownHostsName),
		"-o", "ConnectTimeout=10",
		"-b", bf.Name(),
		t.username + "@" + t.host,
	}
	res, err := o.Exec.Run(ctx, binSFTP, argv, timeout)
	if err != nil {
		if isTimeout(err) {
			return res, errors.New(msgTimedOut)
		}
		return res, errors.New(def)
	}
	return res, nil
}

func isTimeout(err error) bool {
	return errors.Is(err, executor.ErrTimeout)
}

// sftpReason maps sftp stderr keywords to a short message. It returns "" when no keyword matches.
func sftpReason(stderr string) string {
	s := strings.ToLower(stderr)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(s, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("permission denied (", "authentication failed", "too many authentication failures", "no supported authentication"):
		return msgAuth
	case has("could not resolve hostname", "connection refused", "connection timed out", "operation timed out",
		"no route to host", "network is unreachable", "connection closed", "host key verification failed",
		"connect to host", "kex_exchange_identification", "ssh_exchange_identification"):
		return msgConnect
	case has("can't change directory", "couldn't canonicalise", "no such file", "not found", "not a directory"):
		if has("permission denied") {
			return msgFolderDenied
		}
		return msgFolderMissed
	}
	return ""
}

// sftpFailure is the error for a failed sftp run: the mapped keyword or the default.
func sftpFailure(stderr, def string) error {
	if r := sftpReason(stderr); r != "" {
		return errors.New(r)
	}
	return errors.New(def)
}

// ---------------------------------------------------------------------------
// backup.test / backup.upload / remote backup.delete
// ---------------------------------------------------------------------------

// backupTest checks that the key is accepted, the remote folder exists and it can be listed.
func (o *Ops) backupTest(ctx context.Context, args map[string]string) error {
	t, err := parseRemoteTarget(args)
	if err != nil {
		return err
	}
	dir := args["path"]
	if err := validateRemotePath(dir); err != nil {
		return err
	}
	res, err := o.runSFTP(ctx, t, []string{"cd " + dir, "ls"}, sftpTestTime, msgTestFailed)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return sftpFailure(res.Stderr, msgTestFailed)
	}
	return nil
}

// backupUpload sends one local archive to the remote folder. It is written as
// <name>.part and renamed only when complete. The local file is never removed here.
func (o *Ops) backupUpload(ctx context.Context, args map[string]string) error {
	local, err := o.validateLocalArchive(args["file"])
	if err != nil {
		return err
	}
	t, err := parseRemoteTarget(args)
	if err != nil {
		return err
	}
	dir := args["path"]
	if err := validateRemotePath(dir); err != nil {
		return err
	}
	info, err := os.Lstat(local)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("backup file not found")
	}
	base := filepath.Base(local)
	batch := []string{
		"cd " + dir,
		"put " + local + " " + base + ".part",
		"rename " + base + ".part " + base,
	}
	res, err := o.runSFTP(ctx, t, batch, sftpUploadTime, msgUploadFailed)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return sftpFailure(res.Stderr, msgUploadFailed)
	}
	return nil
}

// backupDeleteRemote removes one archive from the remote target. The path is the
// full remote file path. A file that is already gone is not an error.
func (o *Ops) backupDeleteRemote(ctx context.Context, args map[string]string) error {
	t, err := parseRemoteTarget(args)
	if err != nil {
		return err
	}
	p := args["path"]
	if err := validateRemoteArchive(p); err != nil {
		return err
	}
	res, err := o.runSFTP(ctx, t, []string{"rm " + p}, sftpDeleteTime, msgDeleteFailed)
	if err != nil {
		return err
	}
	if res.ExitCode == 0 {
		return nil
	}
	// sftp reports a missing file as "No such file", which sftpReason maps to a
	// missing folder. Either way the file is not there, so the delete has succeeded.
	switch reason := sftpReason(res.Stderr); reason {
	case msgFolderMissed:
		return nil
	case "":
		return errors.New(msgDeleteFailed)
	default:
		return errors.New(reason)
	}
}
