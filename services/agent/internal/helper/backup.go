package helper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	info, err := os.Stat(output)
	if err != nil {
		return Result{}, fmt.Errorf("backup missing after archive: %w", err)
	}
	done = true
	return Result{Output: output + " " + strconv.FormatInt(info.Size(), 10)}, nil
}

// backupDelete removes one archive. A file that is already gone is not an error.
func (o *Ops) backupDelete(args map[string]string) error {
	p, err := o.backupPath(args["path"])
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return errors.New("the backup could not be deleted")
	}
	return nil
}
