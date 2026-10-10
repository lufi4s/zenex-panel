package helper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// Per-website SFTP access. A website's account is put in the zenex-sftp group and
// sshd locks that group to its own home folder (ChrootDirectory). The account keeps
// its nologin shell, so SSH sessions are impossible; only the SFTP subsystem works.
// The password is set with chpasswd (stdin), never argv. Disabling locks the password.

const (
	sftpGroup   = "zenex-sftp"
	binGroupadd = "/usr/sbin/groupadd"
	binUsermod  = "/usr/sbin/usermod"
	binChpasswd = "/usr/sbin/chpasswd"
	binPasswd   = "/usr/bin/passwd"
	binGpasswd  = "/usr/bin/gpasswd"
	binSSHD     = "/usr/sbin/sshd"
	sshTime     = 60 * time.Second
)

// sftpBinaries are the programs SFTP access needs; AllowedBinaries lists them.
var sftpBinaries = []string{binGroupadd, binUsermod, binChpasswd, binPasswd, binGpasswd, binSSHD}

// sftpDropInContent is the only sshd configuration Zenex writes. It applies to the
// zenex-sftp group and nothing else, so administrator logins are unchanged.
const sftpDropInContent = `# Managed by Zenex. Website accounts get SFTP into their own folder only.
Match Group zenex-sftp
    ChrootDirectory %h
    ForceCommand internal-sftp
    PasswordAuthentication yes
    KbdInteractiveAuthentication no
    AllowTcpForwarding no
    X11Forwarding no
    PermitTTY no
`

var sftpPasswordRe = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

// sftpEnable makes the website's SFTP login work with the given password.
// Arguments: user, password (hex). The sshd configuration is tested before it is reloaded.
func (o *Ops) sftpEnable(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	pass := args["password"]
	if !sftpPasswordRe.MatchString(pass) {
		return errors.New("invalid SFTP password")
	}
	if _, err := os.Stat(filepath.Join(o.homeDir(name))); err != nil {
		return errors.New("website folder not found")
	}

	if err := runOK(ctx, o, binGroupadd, []string{"-f", sftpGroup}, nil); err != nil {
		return fmt.Errorf("create SFTP group: %w", err)
	}
	if err := runOK(ctx, o, binChpasswd, nil, []byte(name+":"+pass+"\n")); err != nil {
		return fmt.Errorf("set SFTP password: %w", err)
	}
	if err := runOK(ctx, o, binUsermod, []string{"-a", "-G", sftpGroup, name}, nil); err != nil {
		return fmt.Errorf("add account to SFTP group: %w", err)
	}

	previous, _ := os.ReadFile(o.Paths.SSHDropIn)
	if err := writeFileAtomic(o.Paths.SSHDropIn, []byte(sftpDropInContent), 0o644); err != nil {
		return fmt.Errorf("write SFTP settings: %w", err)
	}
	if err := runOK(ctx, o, binSSHD, []string{"-t"}, nil); err != nil {
		// Put back what was there before, so a bad file never breaks SSH for the server.
		if len(previous) > 0 {
			_ = writeFileAtomic(o.Paths.SSHDropIn, previous, 0o644)
		} else {
			_ = os.Remove(o.Paths.SSHDropIn)
		}
		return fmt.Errorf("SSH configuration test failed; SFTP not enabled: %w", err)
	}
	if err := runOK(ctx, o, binSystemctl, []string{"reload", "ssh"}, nil); err != nil {
		return fmt.Errorf("reload SSH: %w", err)
	}
	return nil
}

// sftpDisable stops SFTP for the website: the password is locked and the account
// leaves the SFTP group. The site's files are not touched.
func (o *Ops) sftpDisable(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if err := runOK(ctx, o, binPasswd, []string{"-l", name}, nil); err != nil {
		return fmt.Errorf("lock SFTP password: %w", err)
	}
	// gpasswd exits non-zero when the account is not a member; that is already the goal.
	if _, err := o.Exec.Run(ctx, binGpasswd, []string{"-d", name, sftpGroup}, sshTime); err != nil {
		return fmt.Errorf("remove from SFTP group: %w", err)
	}
	return nil
}

// runOK runs a program with optional stdin and reports a non-zero exit as an error.
func runOK(ctx context.Context, o *Ops, bin string, args []string, stdin []byte) error {
	var res executor.Result
	var err error
	if stdin == nil {
		res, err = o.Exec.Run(ctx, bin, args, sshTime)
	} else {
		res, err = o.Exec.RunInput(ctx, bin, args, string(stdin), sshTime)
	}
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s exited %d: %s", filepath.Base(bin), res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}
