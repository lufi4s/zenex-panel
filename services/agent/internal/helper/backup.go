package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// backupSuffix is the only archive format the helper creates or deletes.
const backupSuffix = ".tar.gz"

// backupPath checks that p is a clean absolute path to an archive under the backup
// root. Requiring p to be clean rejects anything that could hide a traversal.
func (o *Ops) backupPath(p string) (string, error) {
	root := filepath.Clean(o.Paths.BackupDir)
	clean := filepath.Clean(p)
	if p == "" || clean != p || strings.Contains(p, "..") || strings.ContainsRune(p, 0) ||
		!filepath.IsAbs(clean) || !strings.HasPrefix(clean, root+string(filepath.Separator)) ||
		!strings.HasSuffix(clean, backupSuffix) || len(filepath.Base(clean)) <= len(backupSuffix) {
		return "", errors.New("invalid backup path")
	}
	return clean, nil
}

// manifestSuffix names the manifest that sits beside each archive. The manifest says which
// website the archive belongs to, so another panel can offer it for restore.
const (
	manifestSuffix   = ".json"
	manifestFormat   = 1
	maxManifestBytes = 16 << 10
)

var (
	manifestDomainRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,252}$`)
	manifestSiteIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// backupManifest is the JSON written beside an archive.
type backupManifest struct {
	Format    int    `json:"format"`
	Domain    string `json:"domain"`
	SiteID    string `json:"site_id"`
	LinuxUser string `json:"linux_user"`
	Database  string `json:"database"`
	CreatedAt string `json:"created_at"`
}

// manifestPath is the manifest of an archive: the same path with the .json suffix.
func manifestPath(archive string) string {
	return strings.TrimSuffix(archive, backupSuffix) + manifestSuffix
}

// checkManifestIdentity accepts an empty domain and site ID (archives made before manifests
// existed). Otherwise both must be valid.
func checkManifestIdentity(domain, siteID string) error {
	if domain == "" && siteID == "" {
		return nil
	}
	if !manifestDomainRe.MatchString(domain) || !manifestSiteIDRe.MatchString(siteID) {
		return errors.New("invalid website identity for the backup")
	}
	return nil
}

// writeManifest writes the manifest with owner-only permissions.
func writeManifest(path string, m backupManifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// backupCreate writes a tar.gz of the website folder and a dump of its database.
// The database name is the site account name, as db.create sets it up.
func (o *Ops) backupCreate(ctx context.Context, args map[string]string) (Result, error) {
	name, err := requireLinuxUser(args)
	if err != nil {
		return Result{}, err
	}
	db := args["database"]
	if db != name {
		return Result{}, errors.New("database identifiers must match the site account")
	}
	docroot := o.docRoot(name)
	if args["docroot"] != docroot {
		return Result{}, errors.New("invalid website folder")
	}
	output, err := o.backupPath(args["output"])
	if err != nil {
		return Result{}, err
	}
	domain, siteID := args["domain"], args["site_id"]
	if err := checkManifestIdentity(domain, siteID); err != nil {
		return Result{}, err
	}
	siteDir := filepath.Join(filepath.Clean(o.Paths.BackupDir), name)
	if !strings.HasPrefix(output, siteDir+string(filepath.Separator)) {
		return Result{}, errors.New("backup must be stored in the site's backup folder")
	}
	if _, err := os.Lstat(output); err == nil {
		return Result{}, errors.New("a backup with this name already exists")
	}

	// The folder is root-only, so the archive and the dump are never readable by a site account.
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create backup folder: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("secure backup folder: %w", err)
	}

	sqlPath := filepath.Join(dir, "database.sql")
	sqlFile, err := os.OpenFile(sqlPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Result{}, fmt.Errorf("create database dump file: %w", err)
	}
	if err := sqlFile.Close(); err != nil {
		_ = os.Remove(sqlPath)
		return Result{}, fmt.Errorf("create database dump file: %w", err)
	}

	done := false
	defer func() {
		_ = os.Remove(sqlPath)
		if !done {
			_ = os.Remove(output)
			_ = os.Remove(manifestPath(output))
		}
	}()

	// Root connects through the MariaDB socket, as db.create does. The dump goes
	// straight to the file, so it never passes through the output buffer.
	res, err := o.Exec.Run(ctx, binMariadbDump, []string{
		"--protocol=socket", "-uroot",
		"--single-transaction", "--quick", "--routines",
		"--result-file=" + sqlPath,
		db,
	}, backupDumpTime)
	if err := execErr(res, err); err != nil {
		return Result{}, fmt.Errorf("database dump failed: %s", trim(err.Error()))
	}
	if err := os.Chmod(sqlPath, 0o600); err != nil {
		return Result{}, err
	}

	// Relative names inside the archive: the website folder and the dump.
	res, err = o.Exec.Run(ctx, binTar, []string{
		"-czf", output,
		"-C", filepath.Dir(docroot), filepath.Base(docroot),
		"-C", dir, filepath.Base(sqlPath),
	}, backupTarTime)
	if err := execErr(res, err); err != nil {
		return Result{}, fmt.Errorf("archive failed: %s", trim(err.Error()))
	}
	if err := os.Chmod(output, 0o600); err != nil {
		return Result{}, err
	}
	if domain != "" {
		m := backupManifest{
			Format: manifestFormat, Domain: domain, SiteID: siteID, LinuxUser: name, Database: db,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeManifest(manifestPath(output), m); err != nil {
			return Result{}, fmt.Errorf("write backup manifest: %w", err)
		}
	}
	info, err := os.Stat(output)
	if err != nil {
		return Result{}, fmt.Errorf("backup missing after archive: %w", err)
	}
	done = true
	return Result{Output: output + " " + strconv.FormatInt(info.Size(), 10)}, nil
}

// backupDelete removes one archive. A file that is already gone is not an error.
// With remote="1" the archive is removed from the off-server backup target instead.
func (o *Ops) backupDelete(ctx context.Context, args map[string]string) error {
	switch args["remote"] {
	case "1":
		return o.backupDeleteRemote(ctx, args)
	case "", "0":
	default:
		return errors.New("invalid remote flag")
	}
	p, err := o.backupPath(args["path"])
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return errors.New("the backup could not be deleted")
	}
	// The manifest goes with its archive. A leftover one is harmless, so its error is ignored.
	_ = os.Remove(manifestPath(p))
	return nil
}
