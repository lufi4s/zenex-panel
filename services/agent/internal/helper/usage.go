package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SiteUsage is the disk space one website account uses, in bytes.
type SiteUsage struct {
	User          string `json:"user"`
	FilesBytes    int64  `json:"files_bytes"`
	DatabaseBytes int64  `json:"database_bytes"`
	BackupBytes   int64  `json:"backup_bytes"`
}

// maxUsageAccounts bounds the answer; the panel hosts far fewer websites than this.
const maxUsageAccounts = 2000

// usageSQL lists the size of every website database (each is named after its account).
const usageSQL = "SELECT table_schema, COALESCE(SUM(data_length + index_length), 0) " +
	"FROM information_schema.tables WHERE table_schema REGEXP '^zx_' GROUP BY table_schema;"

// sitesUsage measures every website's files, database and backups. It only reads: the
// file walk does not follow symbolic links, and the database query changes nothing.
func (o *Ops) sitesUsage(ctx context.Context) (Result, error) {
	dirs, err := filepath.Glob(filepath.Join(o.Paths.WebRoot, "zx_*"))
	if err != nil {
		return Result{}, fmt.Errorf("list websites: %w", err)
	}
	byUser := map[string]*SiteUsage{}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if !linuxUserRe.MatchString(name) || len(byUser) >= maxUsageAccounts {
			continue
		}
		byUser[name] = &SiteUsage{User: name, FilesBytes: treeBytes(dir)}
	}
	for name, u := range byUser {
		u.BackupBytes = treeBytes(filepath.Join(o.Paths.BackupDir, name))
	}

	res, err := o.Exec.RunInput(ctx, binMariadb, []string{"--protocol=socket", "-uroot", "--batch", "--skip-column-names"}, usageSQL, 60*time.Second)
	if err != nil {
		return Result{}, fmt.Errorf("database sizes: %w", err)
	}
	if res.ExitCode != 0 {
		return Result{}, fmt.Errorf("database sizes failed: %s", trim(res.Stderr))
	}
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		schema, size, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
		if err != nil {
			continue
		}
		if u, found := byUser[schema]; found {
			u.DatabaseBytes = n
		}
	}

	out := make([]SiteUsage, 0, len(byUser))
	for _, u := range byUser {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User < out[j].User })
	b, err := json.Marshal(out)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(b)}, nil
}

// treeBytes adds the size of every regular file under dir. Missing folders count as 0,
// and unreadable entries are skipped rather than failing the whole measurement.
func treeBytes(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
