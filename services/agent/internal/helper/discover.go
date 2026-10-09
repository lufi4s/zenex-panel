package helper

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sftpDiscoverTime = 5 * time.Minute
	msgListFailed    = "could not list the backups"
	// maxDiscovered caps how many backups one listing offers.
	maxDiscovered = 500
)

// DiscoveredBackup is one entry of the backup.discover reply. File is the remote path of the archive.
type DiscoveredBackup struct {
	File      string `json:"file"`
	SizeBytes int64  `json:"size_bytes"`
	Domain    string `json:"domain"`
	SiteID    string `json:"site_id"`
	LinuxUser string `json:"linux_user"`
	Database  string `json:"database"`
	CreatedAt string `json:"created_at"`
}

// backupDiscover lists the Zenex backups on the off-server target. An archive is offered only
// when its manifest is there too. The reply is a JSON array of DiscoveredBackup.
func (o *Ops) backupDiscover(ctx context.Context, args map[string]string) (Result, error) {
	t, err := parseRemoteTarget(args)
	if err != nil {
		return Result{}, err
	}
	dir := args["path"]
	if err := validateRemotePath(dir); err != nil {
		return Result{}, err
	}
	pw := args["password"]
	res, err := o.runSFTP(ctx, t, pw, []string{"cd " + dir, "ls -l"}, sftpDiscoverTime, msgListFailed)
	if err != nil {
		return Result{}, err
	}
	if res.ExitCode != 0 {
		return Result{}, sftpFailure(res, pw != "", msgListFailed)
	}
	sizes := parseSFTPListing(res.Stdout)

	var names []string
	for _, name := range archivesWithManifest(sizes) {
		if validateRemoteArchive(path.Join(dir, name)) == nil {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return Result{Output: "[]"}, nil
	}

	keyPath, err := o.backupKeyPath()
	if err != nil {
		return Result{}, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(keyPath), "discover-")
	if err != nil {
		return Result{}, errors.New("prepare listing failed")
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	batch := make([]string, 0, len(names))
	for _, name := range names {
		m := manifestName(name)
		batch = append(batch, "get "+path.Join(dir, m)+" "+filepath.Join(tmp, m))
	}
	res, err = o.runSFTP(ctx, t, pw, batch, sftpDiscoverTime, msgListFailed)
	if err != nil {
		return Result{}, err
	}
	if res.ExitCode != 0 {
		return Result{}, sftpFailure(res, pw != "", msgListFailed)
	}

	found := []DiscoveredBackup{}
	for _, name := range names {
		m, ok := readManifest(filepath.Join(tmp, manifestName(name)))
		if !ok {
			continue
		}
		found = append(found, DiscoveredBackup{
			File:      path.Join(dir, name),
			SizeBytes: sizes[name],
			Domain:    m.Domain,
			SiteID:    m.SiteID,
			LinuxUser: m.LinuxUser,
			Database:  m.Database,
			CreatedAt: m.CreatedAt,
		})
	}
	out, err := json.Marshal(found)
	if err != nil {
		return Result{}, errors.New("encode listing failed")
	}
	return Result{Output: string(out)}, nil
}

// manifestName is the file name of the manifest for an archive file name.
func manifestName(archive string) string {
	return strings.TrimSuffix(archive, backupSuffix) + manifestSuffix
}

// archivesWithManifest returns the archive names that have a manifest beside them, sorted,
// capped at maxDiscovered.
func archivesWithManifest(sizes map[string]int64) []string {
	var names []string
	for name := range sizes {
		if !strings.HasSuffix(name, backupSuffix) || len(name) <= len(backupSuffix) {
			continue
		}
		if _, ok := sizes[manifestName(name)]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > maxDiscovered {
		names = names[:maxDiscovered]
	}
	return names
}

// parseSFTPListing reads the regular files from the output of sftp "ls -l": name to size
// in bytes. Other lines, such as directories and the command echo, are skipped.
func parseSFTPListing(out string) map[string]int64 {
	files := map[string]int64{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || !strings.HasPrefix(fields[0], "-") {
			continue
		}
		size, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil || size < 0 {
			continue
		}
		files[fields[len(fields)-1]] = size
	}
	return files
}

// readManifest parses one downloaded manifest. A file that is missing, too large or malformed is skipped.
func readManifest(p string) (backupManifest, bool) {
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return backupManifest{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return backupManifest{}, false
	}
	var m backupManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return backupManifest{}, false
	}
	if m.Format != manifestFormat || m.Domain == "" || checkManifestIdentity(m.Domain, m.SiteID) != nil {
		return backupManifest{}, false
	}
	return m, true
}
