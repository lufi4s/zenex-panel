package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/packages/validation"
	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// Mail server operations. The API decides what the configuration says (apps/api/internal/mail);
// the helper only accepts that text for a fixed list of files, checks it with the mail
// programs, and rolls back if any check fails. It never accepts a path from the caller.

const (
	binPostfix   = "/usr/sbin/postfix"
	binDoveconf  = "/usr/bin/doveconf"
	binRspamadm  = "/usr/bin/rspamadm"
	binOpenSSL   = "/usr/bin/openssl"
	binUfw       = "/usr/sbin/ufw"
	mailTLSDir   = "/etc/zenex/mail/tls"
	mailTimeout  = 2 * time.Minute
	maxMailFile  = 1 << 20 // 1 MiB per configuration file
	dkimKeyBits  = "2048"
	dkimSelector = "default"
)

// mailHostRe is a full host name such as mail.example.com (lower case).
var mailHostRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// mailBinaries are the programs the mail operations run; AllowedBinaries lists them.
var mailBinaries = []string{binPostfix, binDoveconf, binRspamadm, binOpenSSL, binUfw}

// mailPorts are the ports the mail server listens on: SMTP, submission and IMAP over TLS.
var mailPorts = []string{"25", "587", "993"}

// mailFileAllowlist is the only files mail.apply may write, with the modes it may use.
// Keep it in step with apps/api/internal/mail/server.go.
var mailFileAllowlist = map[string][]uint32{
	"/etc/postfix/main.cf":                   {0o644},
	"/etc/postfix/master.cf":                 {0o644},
	"/etc/postfix/zenex-domains.cf":          {0o640},
	"/etc/postfix/zenex-mailboxes.cf":        {0o640},
	"/etc/dovecot/conf.d/zenex-sql.conf.ext": {0o640},
	"/etc/dovecot/conf.d/99-zenex.conf":      {0o644},
	"/etc/rspamd/local.d/dkim_signing.conf":  {0o644},
}

// mailFile is one configuration file as the API sends it.
type mailFile struct {
	Path    string `json:"path"`
	Mode    uint32 `json:"mode"`
	Content string `json:"content"`
}

// validateMailFiles checks every file against the allowlist before anything is written.
func validateMailFiles(files []mailFile) error {
	if len(files) == 0 {
		return errors.New("no configuration files were sent")
	}
	seen := map[string]bool{}
	for _, f := range files {
		modes, ok := mailFileAllowlist[f.Path]
		if !ok {
			return fmt.Errorf("%q is not a mail configuration file", f.Path)
		}
		if seen[f.Path] {
			return fmt.Errorf("%q was sent twice", f.Path)
		}
		seen[f.Path] = true
		allowed := false
		for _, m := range modes {
			allowed = allowed || m == f.Mode
		}
		if !allowed {
			return fmt.Errorf("mode %o is not allowed for %s", f.Mode, f.Path)
		}
		if len(f.Content) == 0 || len(f.Content) > maxMailFile {
			return fmt.Errorf("%s has an invalid size", f.Path)
		}
	}
	return nil
}

// mailPath maps an allowlisted path to where it is written. Tests set MailRoot so nothing
// outside the temporary folder is touched.
func (o *Ops) mailPath(path string) string {
	if o.MailRoot == "" {
		return path
	}
	return filepath.Join(o.MailRoot, path)
}

// mailApply writes the mail configuration, checks it, and reloads the services. If a check
// fails, every file goes back to what it was before.
func (o *Ops) mailApply(ctx context.Context, args map[string]string) (Result, error) {
	var files []mailFile
	if err := json.Unmarshal([]byte(args["files"]), &files); err != nil {
		return Result{}, errors.New("the configuration could not be read")
	}
	if err := validateMailFiles(files); err != nil {
		return Result{}, err
	}
	redact := func(s string) string {
		if pass := args["dbpass"]; pass != "" {
			s = strings.ReplaceAll(s, pass, "[hidden]")
		}
		return trim(s)
	}

	type backup struct {
		path    string
		existed bool
		content []byte
		mode    os.FileMode
	}
	var saved []backup
	restore := func() {
		for i := len(saved) - 1; i >= 0; i-- {
			b := saved[i]
			if b.existed {
				_ = writeFileAtomic(b.path, b.content, b.mode)
			} else {
				_ = os.Remove(b.path)
			}
		}
	}

	for _, f := range files {
		target := o.mailPath(f.Path)
		b := backup{path: target}
		if info, err := os.Stat(target); err == nil {
			b.existed, b.mode = true, info.Mode().Perm()
			if b.content, err = os.ReadFile(target); err != nil {
				return Result{}, fmt.Errorf("read %s: %w", f.Path, err)
			}
		}
		saved = append(saved, b)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			restore()
			return Result{}, fmt.Errorf("prepare %s: %w", filepath.Dir(f.Path), err)
		}
		if err := writeFileAtomic(target, []byte(f.Content), os.FileMode(f.Mode)); err != nil {
			restore()
			return Result{}, fmt.Errorf("write %s: %w", f.Path, err)
		}
	}

	checks := []struct {
		name string
		bin  string
		args []string
	}{
		{"postfix check", binPostfix, []string{"check"}},
		{"dovecot configuration", binDoveconf, []string{"-n"}},
		{"rspamd configuration", binRspamadm, []string{"configtest"}},
	}
	for _, c := range checks {
		res, err := o.Exec.Run(ctx, c.bin, c.args, mailTimeout)
		if err != nil || res.ExitCode != 0 {
			restore()
			detail := ""
			if err != nil {
				detail = err.Error()
			} else {
				detail = res.Stderr + res.Stdout
			}
			return Result{}, fmt.Errorf("the %s check failed; nothing was changed: %s", c.name, redact(detail))
		}
	}

	// The installer leaves the mail services stopped until the first apply, so start them (a
	// no-op when they already run) and then reload to pick up the new configuration.
	for _, svc := range []string{"postfix", "dovecot", "rspamd"} {
		for _, action := range [][]string{{"enable", "--now", svc}, {"reload", svc}} {
			res, err := o.Exec.Run(ctx, binSystemctl, action, mailTimeout)
			if err != nil || res.ExitCode != 0 {
				detail := ""
				if err != nil {
					detail = err.Error()
				} else {
					detail = res.Stderr
				}
				return Result{}, fmt.Errorf("%s %s failed: %s", action[0], svc, redact(detail))
			}
		}
	}
	return Result{Output: "applied"}, nil
}

// mailPrepare makes the server ready to serve mail under host: a TLS certificate for the name
// (self-signed, until a real one is put in place) and the firewall ports. A certificate that is
// already there for the same host is left alone, so a real certificate is never replaced.
func (o *Ops) mailPrepare(ctx context.Context, args map[string]string) (Result, error) {
	host := args["host"]
	if !mailHostRe.MatchString(host) {
		return Result{}, errors.New("the mail host must be a full host name such as mail.example.com")
	}
	tlsDir := o.mailPath(mailTLSDir)
	if err := os.MkdirAll(tlsDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("prepare the certificate folder: %w", err)
	}
	cert := filepath.Join(tlsDir, "fullchain.pem")
	key := filepath.Join(tlsDir, "privkey.pem")
	marker := filepath.Join(tlsDir, "host")

	current, _ := os.ReadFile(marker)
	_, certErr := os.Stat(cert)
	if strings.TrimSpace(string(current)) != host || certErr != nil {
		res, err := o.Exec.Run(ctx, binOpenSSL, []string{
			"req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "825",
			"-keyout", key, "-out", cert, "-subj", "/CN=" + host, "-addext", "subjectAltName=DNS:" + host,
		}, mailTimeout)
		if err != nil || res.ExitCode != 0 {
			return Result{}, fmt.Errorf("create the certificate: %s", runDetail(res, err))
		}
		_ = os.Chmod(key, 0o640)
		_ = os.Chmod(cert, 0o644)
		// Postfix reads the key as the postfix group. Best effort: skipped where there is none.
		if g, err := user.LookupGroup("postfix"); err == nil {
			if gid, err := strconv.Atoi(g.Gid); err == nil {
				_ = os.Chown(key, -1, gid)
			}
		}
		if err := writeFileAtomic(marker, []byte(host+"\n"), 0o644); err != nil {
			return Result{}, fmt.Errorf("remember the mail host: %w", err)
		}
	}

	for _, port := range mailPorts {
		res, err := o.Exec.Run(ctx, binUfw, []string{"allow", port + "/tcp", "comment", "mail"}, mailTimeout)
		if err != nil || res.ExitCode != 0 {
			return Result{}, fmt.Errorf("open port %s: %s", port, runDetail(res, err))
		}
	}
	return Result{Output: "prepared"}, nil
}

// dkimKey returns the public DKIM value (the p= part of the DNS record) for a domain, creating
// the key pair the first time. The private key stays in the root-only key folder.
func (o *Ops) dkimKey(ctx context.Context, args map[string]string) (Result, error) {
	domain := args["domain"]
	if err := validation.DomainName(domain); err != nil {
		return Result{}, errors.New("invalid domain")
	}
	dir := filepath.Join(o.dkimRoot(), domain)
	keyPath := filepath.Join(dir, dkimSelector+".key")
	pubPath := filepath.Join(dir, dkimSelector+".pub")

	if _, err := os.Stat(pubPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Result{}, fmt.Errorf("create key folder: %w", err)
		}
		if res, err := o.Exec.Run(ctx, binOpenSSL, []string{"genpkey", "-algorithm", "RSA", "-pkeyopt", "rsa_keygen_bits:" + dkimKeyBits, "-out", keyPath}, mailTimeout); err != nil || res.ExitCode != 0 {
			return Result{}, fmt.Errorf("create DKIM key: %s", runDetail(res, err))
		}
		if err := os.Chmod(keyPath, 0o600); err != nil {
			return Result{}, fmt.Errorf("protect DKIM key: %w", err)
		}
		if res, err := o.Exec.Run(ctx, binOpenSSL, []string{"pkey", "-in", keyPath, "-pubout", "-out", pubPath}, mailTimeout); err != nil || res.ExitCode != 0 {
			return Result{}, fmt.Errorf("read DKIM public key: %s", runDetail(res, err))
		}
	}
	pem, err := os.ReadFile(pubPath)
	if err != nil {
		return Result{}, fmt.Errorf("read DKIM public key: %w", err)
	}
	return Result{Output: dkimTXTValue(string(pem))}, nil
}

// dkimTXTValue turns a PEM public key into the p= value a DKIM record needs (base64 of the DER).
func dkimTXTValue(pem string) string {
	var b strings.Builder
	for _, line := range strings.Split(pem, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		b.WriteString(line)
	}
	return "p=" + b.String()
}

func (o *Ops) dkimRoot() string {
	if o.Paths.DKIMDir != "" {
		return o.Paths.DKIMDir
	}
	return "/var/lib/zenex/dkim"
}

func runDetail(res executor.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	return trim(res.Stderr)
}
