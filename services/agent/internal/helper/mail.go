package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	mailTimeout  = 2 * time.Minute
	maxMailFile  = 1 << 20 // 1 MiB per configuration file
	dkimKeyBits  = "2048"
	dkimSelector = "default"
)

// mailHostRe is a full host name such as mail.example.com (lower case).
var mailHostRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// mailBinaries are the programs the mail operations run; AllowedBinaries lists them.
var mailBinaries = []string{binPostfix, binDoveconf, binRspamadm, binOpenSSL}

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

	for _, svc := range []string{"postfix", "dovecot", "rspamd"} {
		res, err := o.Exec.Run(ctx, binSystemctl, []string{"reload", svc}, mailTimeout)
		if err != nil || res.ExitCode != 0 {
			detail := ""
			if err != nil {
				detail = err.Error()
			} else {
				detail = res.Stderr
			}
			return Result{}, fmt.Errorf("reloading %s failed: %s", svc, redact(detail))
		}
	}
	return Result{Output: "applied"}, nil
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
