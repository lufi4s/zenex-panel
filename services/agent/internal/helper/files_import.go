package helper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxUploadBytes is the largest file the panel can upload into a website.
const maxUploadBytes = 64 << 20

// filesImport moves an upload that the API staged in the upload folder into a folder of the
// site. The staged file is always removed afterwards. An existing file is never replaced.
func (o *Ops) filesImport(ctx context.Context, linuxUser, rel, name, staged string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validUploadName(name) {
		return errors.New("invalid file name")
	}
	src, err := o.stagedUpload(staged)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(src) }()

	dir, err := o.resolveInSite(linuxUser, rel)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return errors.New("the folder does not exist")
	}
	target := filepath.Join(dir, name)
	if _, err := os.Lstat(target); err == nil {
		return errors.New("a file or folder with this name already exists")
	}

	in, err := os.Open(src)
	if err != nil {
		return errors.New("the upload could not be read")
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return errors.New("the file could not be saved")
	}
	// One byte past the limit tells a too-large file apart from one that is exactly the limit.
	n, copyErr := io.Copy(out, io.LimitReader(in, maxUploadBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > maxUploadBytes {
		_ = os.Remove(target)
		if n > maxUploadBytes {
			return fmt.Errorf("the file is larger than %d MB", maxUploadBytes>>20)
		}
		return errors.New("the file could not be saved")
	}
	if err := o.ownBySite(linuxUser, target); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("give the file to the website: %w", err)
	}
	return nil
}

// stagedUpload checks that p is a regular file inside the upload folder and returns it.
func (o *Ops) stagedUpload(p string) (string, error) {
	if o.Paths.UploadDir == "" {
		return "", errors.New("uploads are not configured on this server")
	}
	root := filepath.Clean(o.Paths.UploadDir)
	clean := filepath.Clean(p)
	if p == "" || clean != p || strings.Contains(p, "..") || strings.ContainsRune(p, 0) ||
		!strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return "", errors.New("invalid upload")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("the upload is missing")
	}
	return clean, nil
}

// validUploadName accepts one plain file name: no folders, no control characters, no dot names.
func validUploadName(name string) bool {
	if name == "" || len(name) > 200 || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
