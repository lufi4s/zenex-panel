package httpapi

import (
	"net/http"
	"sort"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

// storageResponse says how much disk each website uses. Customers see their own websites.
// Only administrators also see the server's disk, because that describes the shared server.
type storageResponse struct {
	Disk  *serverDisk   `json:"disk,omitempty"`
	Sites []siteStorage `json:"sites"`
}

type serverDisk struct {
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

type siteStorage struct {
	SiteID        string `json:"site_id"`
	Domain        string `json:"domain"`
	FilesBytes    int64  `json:"files_bytes"`
	DatabaseBytes int64  `json:"database_bytes"`
	BackupBytes   int64  `json:"backup_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
}

// handleStorage answers GET /api/v1/storage. Sites are sorted with the largest first.
func (d Deps) handleStorage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	owner := user.ID
	if isAdmin(user) {
		owner = ""
	}
	sites, err := d.Sites.ListSites(r.Context(), owner)
	if err != nil {
		d.internal(w, r, "storage.sites", err)
		return
	}
	usages, err := d.Manage.SiteUsages(r.Context())
	if err != nil {
		d.manageError(w, r, "storage.usage", err)
		return
	}

	resp := storageResponse{Sites: make([]siteStorage, 0, len(sites))}
	for _, s := range sites {
		resp.Sites = append(resp.Sites, storageRow(s, usages[s.LinuxUser].FilesBytes, usages[s.LinuxUser].DatabaseBytes, usages[s.LinuxUser].BackupBytes))
	}
	sort.SliceStable(resp.Sites, func(i, j int) bool { return resp.Sites[i].TotalBytes > resp.Sites[j].TotalBytes })

	if isAdmin(user) {
		if m, err := sysinfo.Snapshot(); err == nil {
			resp.Disk = &serverDisk{TotalBytes: m.DiskTotalBytes, UsedBytes: m.DiskUsedBytes}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func storageRow(s store.Site, files, database, backup int64) siteStorage {
	return siteStorage{
		SiteID:        s.ID,
		Domain:        s.Domain,
		FilesBytes:    files,
		DatabaseBytes: database,
		BackupBytes:   backup,
		TotalBytes:    files + database + backup,
	}
}
