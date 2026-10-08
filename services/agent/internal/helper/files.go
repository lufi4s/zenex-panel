package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxEditableBytes = 1 << 20 // text files up to 1 MB can be opened and saved
	maxListEntries   = 2000
)

// coreFiles are WordPress files the file manager will not delete, so a mistake
// cannot take the whole site down.
var coreFiles = map[string]bool{
	"wp-config.php":      true,
	"wp-load.php":        true,
	"wp-settings.php":    true,
	"wp-blog-header.php": true,
	"index.php":          true,
	"wp-includes":        true,
	"wp-admin":           true,
}

// FileEntry is one item in a folder listing.
type FileEntry struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"` // "dir" or "file"
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Editable bool      `json:"editable"`
}

// resolveInSite turns a path relative to the site's web root into an absolute
// path. It refuses anything that would leave the root, including through a
// symbolic link.
func (o *Ops) resolveInSite(linuxUser, rel string) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", errors.New("invalid path")
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	root := filepath.Clean(o.docRoot(linuxUser))
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." {
		clean = ""
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path is outside the website folder")
	}
	full := filepath.Join(root, clean)

	// Check the real location: a symbolic link inside the site must not lead out.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("website folder is missing")
	}
	existing := full
	for {
		real, err := filepath.EvalSymlinks(existing)
		if err == nil {
			if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
				return "", errors.New("path points outside the website folder")
			}
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing || len(existing) <= len(root) {
			break
		}
		existing = parent
	}
	return full, nil
}

// filesList returns the entries of a folder, folders first, then by name.
func (o *Ops) filesList(linuxUser, rel string) (Result, error) {
	full, err := o.resolveInSite(linuxUser, rel)
	if err != nil {
		return Result{}, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return Result{}, errors.New("this folder could not be read")
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		if len(out) >= maxListEntries {
			break
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fe := FileEntry{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()}
		if e.IsDir() {
			fe.Type = "dir"
		} else if info.Mode().IsRegular() {
			fe.Type = "file"
			fe.Editable = info.Size() <= maxEditableBytes && isTextName(e.Name())
		} else {
			continue // symbolic links and devices are not listed
		}
		out = append(out, fe)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "dir"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	data, _ := json.Marshal(out)
	return Result{Output: string(data)}, nil
}

func isTextName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".php", ".html", ".htm", ".css", ".js", ".json", ".txt", ".md", ".xml", ".svg",
		".htaccess", ".env", ".ini", ".yml", ".yaml", ".conf", ".log", ".csv", ".ts", ".tsx":
		return true
	}
	return filepath.Base(name) == ".htaccess" || filepath.Ext(name) == ""
}

// filesRead returns the text of one file.
func (o *Ops) filesRead(linuxUser, rel string) (Result, error) {
	full, err := o.resolveInSite(linuxUser, rel)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() {
		return Result{}, errors.New("that file does not exist")
	}
	if info.Size() > maxEditableBytes {
		return Result{}, fmt.Errorf("this file is larger than %d KB and cannot be edited here", maxEditableBytes/1024)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return Result{}, errors.New("this file could not be read")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return Result{}, errors.New("this file is binary and cannot be shown as text")
	}
	return Result{Output: string(data)}, nil
}

// filesWrite saves text to a file. The file is written to a temporary name and
// renamed, so a crash never leaves a half-written file behind.
func (o *Ops) filesWrite(linuxUser, rel, content string) error {
	if len(content) > maxEditableBytes {
		return fmt.Errorf("the text is larger than %d KB", maxEditableBytes/1024)
	}
	full, err := o.resolveInSite(linuxUser, rel)
	if err != nil {
		return err
	}
	if info, err := os.Stat(full); err == nil && info.IsDir() {
		return errors.New("a folder with this name already exists")
	}
	dir := filepath.Dir(full)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return errors.New("the folder for this file does not exist")
	}
	if err := writeFileAtomic(full, []byte(content), 0o640); err != nil {
		return errors.New("the file could not be saved")
	}
	return o.ownBySite(linuxUser, full)
}

// filesMkdir creates a folder. Its parent must already exist.
func (o *Ops) filesMkdir(linuxUser, rel string) error {
	full, err := o.resolveInSite(linuxUser, rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); err == nil {
		return errors.New("something with this name already exists")
	}
	if err := os.Mkdir(full, 0o750); err != nil {
		return errors.New("the folder could not be created")
	}
	return o.ownBySite(linuxUser, full)
}

// filesDelete removes a file or folder inside the site. WordPress core files
// and the web root itself are protected.
func (o *Ops) filesDelete(linuxUser, rel string) error {
	cleanRel := strings.Trim(filepath.ToSlash(rel), "/")
	if cleanRel == "" {
		return errors.New("the website folder itself cannot be deleted here")
	}
	if !strings.Contains(cleanRel, "/") && coreFiles[cleanRel] {
		return fmt.Errorf("%s is a WordPress core file and is protected", cleanRel)
	}
	full, err := o.resolveInSite(linuxUser, rel)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(full); err != nil {
		return errors.New("that file or folder does not exist")
	}
	if err := os.RemoveAll(full); err != nil {
		return errors.New("the file or folder could not be deleted")
	}
	return nil
}

// ownBySite gives a file to the site's account, so PHP can read and update it.
func (o *Ops) ownBySite(linuxUser, full string) error {
	siteUser, err := o.lookup(linuxUser)
	if err != nil {
		return err
	}
	web, err := user.LookupGroup("www-data")
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(siteUser.Uid)
	gid, _ := strconv.Atoi(web.Gid)
	return os.Chown(full, uid, gid)
}

// filesOp dispatches the file operations used by the panel.
func (o *Ops) filesOp(ctx context.Context, op string, args map[string]string) (Result, error) {
	name, err := requireLinuxUser(args)
	if err != nil {
		return Result{}, err
	}
	rel := args["path"]
	switch op {
	case "files.list":
		return o.filesList(name, rel)
	case "files.read":
		return o.filesRead(name, rel)
	case "files.write":
		return Result{}, o.filesWrite(name, rel, args["content"])
	case "files.mkdir":
		return Result{}, o.filesMkdir(name, rel)
	case "files.delete":
		return Result{}, o.filesDelete(name, rel)
	}
	return Result{}, fmt.Errorf("unknown file operation %q", op)
}
