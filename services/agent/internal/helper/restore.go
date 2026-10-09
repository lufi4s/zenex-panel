package helper

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var tablePrefixRe = regexp.MustCompile(`\$table_prefix\s*=\s*'([A-Za-z0-9_]{1,32})'`)

const (
	sftpDownloadTime  = 30 * time.Minute
	msgDownloadFailed = "download failed"
)

// backupDownload copies one archive from the off-server backup target into the site's
// backup folder, so backup.restore can read it. A partial file is removed on failure.
func (o *Ops) backupDownload(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	local, err := o.validateLocalArchive(args["file"])
	if err != nil {
		return err
	}
	siteDir := filepath.Join(filepath.Clean(o.Paths.BackupDir), name)
	if !strings.HasPrefix(local, siteDir+string(filepath.Separator)) {
		return errors.New("backup must be stored in the site's backup folder")
	}
	t, err := parseRemoteTarget(args)
	if err != nil {
		return err
	}
	remote := args["path"]
	if err := validateRemoteArchive(remote); err != nil {
		return err
	}
	exists, err := pathExists(local)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("a backup with this name already exists")
	}
	if err := os.MkdirAll(siteDir, 0o700); err != nil {
		return fmt.Errorf("create backup folder: %w", err)
	}

	pw := args["password"]
	res, err := o.runSFTP(ctx, t, pw, []string{"get " + remote + " " + local}, sftpDownloadTime, msgDownloadFailed)
	if err != nil {
		_ = os.Remove(local)
		return err
	}
	if res.ExitCode != 0 {
		_ = os.Remove(local)
		return sftpFailure(res, pw != "", msgDownloadFailed)
	}
	return secureOwnership(local, 0o600)
}

// backupRestore puts a website back from an archive made by backup.create. The archive is
// checked before anything changes. The unpacked files are then moved into place, and the
// database is replaced. If the database step fails, the old folder is moved back. The
// database is dropped and loaded from the dump, so tables that the dump does not contain
// are removed as well.
//
// The archive's wp-config.php came from another server, so it is written again with this
// site's database account (dbpass). When from_domain differs from the site's domain, the
// stored addresses are changed to the new domain.
func (o *Ops) backupRestore(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if args["database"] != name {
		return errors.New("database identifiers must match the site account")
	}
	docroot := o.docRoot(name)
	if args["docroot"] != docroot {
		return errors.New("invalid website folder")
	}
	archive, err := o.validateLocalArchive(args["archive"])
	if err != nil {
		return err
	}
	siteDir := filepath.Join(filepath.Clean(o.Paths.BackupDir), name)
	if !strings.HasPrefix(archive, siteDir+string(filepath.Separator)) {
		return errors.New("backup must be restored from the site's backup folder")
	}
	info, err := os.Lstat(archive)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("backup file not found")
	}
	if err := o.checkArchiveEntries(ctx, archive, filepath.Base(docroot)); err != nil {
		return err
	}

	// The work folder sits beside the website folder, so the final move is a rename on
	// the same file system. It is root-only and removed when the restore ends.
	home := o.homeDir(name)
	work, err := os.MkdirTemp(home, ".restore-")
	if err != nil {
		return fmt.Errorf("create restore folder: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	res, err := o.Exec.Run(ctx, binTar, []string{"-xzf", archive, "-C", work}, backupTarTime)
	if err := execErr(res, err); err != nil {
		return fmt.Errorf("unpacking the backup failed: %s", trim(err.Error()))
	}

	newDocroot := filepath.Join(work, filepath.Base(docroot))
	sqlPath := filepath.Join(work, "database.sql")
	if err := requireRestoreTree(newDocroot, sqlPath); err != nil {
		return err
	}
	if strings.ContainsAny(filepath.ToSlash(sqlPath), localDenyChars) {
		return errors.New("restore folder path contains characters that are not allowed")
	}
	dbPass := args["dbpass"]
	if err := requireHex(dbPass, "database password"); err != nil {
		return err
	}
	fromDomain, toDomain := args["from_domain"], args["to_domain"]
	if fromDomain != "" && (!manifestDomainRe.MatchString(fromDomain) || !manifestDomainRe.MatchString(toDomain)) {
		return errors.New("invalid domain for the restore")
	}
	prefix := readTablePrefix(filepath.Join(newDocroot, "wp-config.php"))

	// Files first, so the old folder can be put back if the move fails.
	old := filepath.Join(work, "previous")
	if err := os.Rename(docroot, old); err != nil {
		return fmt.Errorf("move the current website folder aside: %w", err)
	}
	if err := os.Rename(newDocroot, docroot); err != nil {
		_ = os.Rename(old, docroot)
		return fmt.Errorf("put the restored website folder in place: %w", err)
	}

	// The name is a validated site account, so the identifiers in this script are safe.
	script := "DROP DATABASE IF EXISTS `" + name + "`;\n" +
		"CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n" +
		"USE `" + name + "`;\n" +
		"SOURCE " + sqlPath + ";\n"
	res, err = o.Exec.RunInput(ctx, binMariadb, []string{"--protocol=socket", "-uroot"}, script, backupDumpTime)
	if err := execErr(res, err); err != nil {
		_ = os.Rename(docroot, newDocroot)
		_ = os.Rename(old, docroot)
		return fmt.Errorf("database restore failed, the website files were put back: %s", trim(err.Error()))
	}

	// The files may come from another server, where the account had another user ID. WordPress
	// runs as this site's account, so the files must belong to it before it reads them.
	if err := o.normalizeSiteTree(name, docroot); err != nil {
		return err
	}
	if err := o.rewriteWPConfig(ctx, name, docroot, dbPass, prefix); err != nil {
		return err
	}
	if fromDomain != "" && fromDomain != toDomain {
		if err := o.replaceDomain(ctx, name, fromDomain, toDomain); err != nil {
			return err
		}
	}
	return nil
}

// normalizeSiteTree gives every file below docroot to the site's account and web group, with
// owner and group access only: folders 0750, files 0640. Symbolic links are left as they are.
func (o *Ops) normalizeSiteTree(name, docroot string) error {
	siteUser, err := o.lookup(name)
	if err != nil {
		return err
	}
	web, err := user.LookupGroup("www-data")
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(siteUser.Uid)
	gid, _ := strconv.Atoi(web.Gid)
	return filepath.WalkDir(docroot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Lchown(p, uid, gid); err != nil {
			return fmt.Errorf("set the owner of the restored files: %w", err)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		mode := os.FileMode(0o640)
		if d.IsDir() {
			mode = 0o750
		}
		if err := os.Chmod(p, mode); err != nil {
			return fmt.Errorf("set the permissions of the restored files: %w", err)
		}
		return nil
	})
}

// readTablePrefix returns the table prefix of a wp-config.php, or "wp_" when it cannot be read.
func readTablePrefix(p string) string {
	data, err := os.ReadFile(p)
	if err != nil {
		return "wp_"
	}
	if m := tablePrefixRe.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return "wp_"
}

// rewriteWPConfig replaces the restored wp-config.php with one that uses this site's
// database account. The salts are new, so existing WordPress logins end.
func (o *Ops) rewriteWPConfig(ctx context.Context, name, docroot, dbPass, prefix string) error {
	if err := os.Remove(filepath.Join(docroot, "wp-config.php")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace wp-config.php: %w", err)
	}
	return o.wpRun(ctx, name, "config", "create",
		"--dbname="+name, "--dbuser="+name, "--dbpass="+dbPass, "--dbhost=localhost",
		"--dbcharset=utf8mb4", "--dbprefix="+prefix)
}

// replaceDomain changes the addresses stored in the database from one domain to another.
func (o *Ops) replaceDomain(ctx context.Context, name, from, to string) error {
	for _, pair := range [][2]string{{"http://" + from, "https://" + to}, {"https://" + from, "https://" + to}} {
		if err := o.wpRun(ctx, name, "search-replace", pair[0], pair[1],
			"--all-tables-with-prefix", "--skip-columns=guid", "--quiet"); err != nil {
			return fmt.Errorf("point the website to %s: %w", to, err)
		}
	}
	return nil
}

// checkArchiveEntries lists the archive and refuses anything outside the website folder
// and the database dump, including ".." and absolute names.
func (o *Ops) checkArchiveEntries(ctx context.Context, archive, base string) error {
	res, err := o.Exec.Run(ctx, binTar, []string{"-tzf", archive}, backupTarTime)
	if err := execErr(res, err); err != nil {
		return fmt.Errorf("the backup cannot be read: %s", trim(err.Error()))
	}
	for _, entry := range strings.Split(res.Stdout, "\n") {
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "..") || strings.HasPrefix(entry, "/") {
			return errors.New("the backup contains unsafe file names")
		}
		if entry == "database.sql" || entry == base || strings.HasPrefix(entry, base+"/") {
			continue
		}
		return errors.New("the backup contains unexpected files")
	}
	return nil
}

// requireRestoreTree checks the unpacked folder: a real website folder and a real dump,
// with no symbolic links anywhere in the website folder.
func requireRestoreTree(docroot, sqlPath string) error {
	info, err := os.Lstat(docroot)
	if err != nil || !info.IsDir() {
		return errors.New("the backup does not contain the website folder")
	}
	info, err = os.Lstat(sqlPath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("the backup does not contain the database dump")
	}
	return filepath.WalkDir(docroot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.New("the backup website folder cannot be read")
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return errors.New("the backup contains symbolic links")
		}
		return nil
	})
}
