package helper

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Limits for one download. They keep one request from filling the disk or running for hours.
const (
	maxExportBytes = 1 << 30
	maxExportFiles = 50000
	maxExportPaths = 200
)

// exportResult is what filesExport reports: the name the browser should save the download as.
type exportResult struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "file" for one file as it is, "zip" for folders and several items
}

// filesExport prepares a download. One file is copied as it is; a folder, or several items, are
// packed into a zip archive, folders included with everything inside. The result is written to
// staged, an empty file the API made in the upload folder; the API sends it and removes it.
// Links inside the site are never followed, so a link cannot pull in a file from elsewhere.
func (o *Ops) filesExport(ctx context.Context, linuxUser, pathList, staged string) (Result, error) {
	out, err := o.stagedUpload(staged)
	if err != nil {
		return Result{}, err
	}
	rels := splitExportPaths(pathList)
	if len(rels) == 0 || len(rels) > maxExportPaths {
		return Result{}, errors.New("choose between 1 and 200 items to download")
	}
	realRoot, err := filepath.EvalSymlinks(filepath.Clean(o.docRoot(linuxUser)))
	if err != nil {
		return Result{}, errors.New("website folder is missing")
	}
	type item struct{ full, name string }
	items := make([]item, 0, len(rels))
	for _, rel := range rels {
		full, err := o.resolveInSite(linuxUser, rel)
		if err != nil {
			return Result{}, err
		}
		info, err := os.Lstat(full)
		if err != nil {
			return Result{}, errors.New("an item to download does not exist")
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return Result{}, errors.New("only files and folders can be downloaded")
		}
		name := filepath.Base(full)
		if full == filepath.Clean(o.docRoot(linuxUser)) {
			name = "site"
		}
		items = append(items, item{full, name})
	}

	dst, err := os.OpenFile(out, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return Result{}, errors.New("the download could not be prepared")
	}
	defer dst.Close()

	if len(items) == 1 {
		if info, err := os.Lstat(items[0].full); err == nil && info.Mode().IsRegular() {
			if info.Size() > maxExportBytes {
				return Result{}, fmt.Errorf("the file is larger than %d MB", maxExportBytes>>20)
			}
			if err := copyInSite(realRoot, items[0].full, dst); err != nil {
				return Result{}, err
			}
			return exportJSON(exportResult{Name: items[0].name, Kind: "file"})
		}
	}

	zw := zip.NewWriter(dst)
	var total int64
	files := 0
	add := func(full, archiveName string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		files++
		if files > maxExportFiles {
			return fmt.Errorf("too many files for one download (limit %d). Download smaller folders", maxExportFiles)
		}
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			return nil // vanished or not a plain file: skipped
		}
		total += info.Size()
		if total > maxExportBytes {
			return fmt.Errorf("the download is larger than %d MB. Download smaller folders", maxExportBytes>>20)
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = archiveName
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		return copyInSite(realRoot, full, w)
	}
	for _, it := range items {
		info, err := os.Lstat(it.full)
		if err != nil {
			return Result{}, errors.New("an item to download does not exist")
		}
		if info.Mode().IsRegular() {
			if err := add(it.full, it.name); err != nil {
				return Result{}, err
			}
			continue
		}
		walkErr := filepath.WalkDir(it.full, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(it.full, p)
			if err != nil {
				return nil
			}
			return add(p, it.name+"/"+filepath.ToSlash(rel))
		})
		if walkErr != nil {
			return Result{}, walkErr
		}
	}
	if err := zw.Close(); err != nil {
		return Result{}, errors.New("the archive could not be finished")
	}
	name := items[0].name + ".zip"
	if len(items) > 1 {
		name = "download.zip"
	}
	return exportJSON(exportResult{Name: name, Kind: "zip"})
}

func exportJSON(r exportResult) (Result, error) {
	data, _ := json.Marshal(r)
	return Result{Output: string(data)}, nil
}

// splitExportPaths reads the newline-separated list of paths, dropping empty lines except a
// single empty path, which means the whole website.
func splitExportPaths(list string) []string {
	if list == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(list, "\n") {
		out = append(out, strings.TrimSuffix(p, "\r"))
	}
	return out
}

// copyInSite copies one regular file. The location is checked again after opening, so a file
// swapped for a link a moment earlier is not followed out of the website.
func copyInSite(realRoot, full string, dst io.Writer) error {
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("a file could not be read")
	}
	f, err := os.Open(full)
	if err != nil {
		return errors.New("a file could not be read")
	}
	defer f.Close()
	real, err := filepath.EvalSymlinks(full)
	if err != nil || (real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator))) {
		return errors.New("a file points outside the website folder")
	}
	if _, err := io.Copy(dst, io.LimitReader(f, maxExportBytes+1)); err != nil {
		return errors.New("a file could not be read")
	}
	return nil
}
