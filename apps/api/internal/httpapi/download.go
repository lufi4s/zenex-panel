package httpapi

import (
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// maxDownloadPaths is how many files and folders one download may name.
const maxDownloadPaths = 200

// downloadDir is where downloads are built. A variable so tests can use a temporary folder.
var downloadDir = manage.UploadDir

var errDownloadBusy = APIError{Status: http.StatusServiceUnavailable, Code: "downloads_busy", Message: "Other downloads are being prepared. Wait for them to finish, then try again."}

// downloadSlots caps downloads prepared at the same time. Each one is built on disk first.
var downloadSlots = make(chan struct{}, 2)

// handleDownloadFiles sends files and folders of a website to the browser. Repeat the path
// parameter to choose several items. One file is sent as it is; a folder, or several items, are
// packed into a zip archive first. The archive is built in the upload folder and removed after.
func (d Deps) handleDownloadFiles(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	paths := r.URL.Query()["path"]
	if len(paths) == 0 || len(paths) > maxDownloadPaths {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	for _, p := range paths {
		if len(p) > maxPathLength || strings.ContainsAny(p, "\x00\n\r") {
			writeError(w, requestIDFrom(r), ErrInvalidRequest)
			return
		}
	}
	select {
	case downloadSlots <- struct{}{}:
		defer func() { <-downloadSlots }()
	default:
		writeError(w, requestIDFrom(r), errDownloadBusy)
		return
	}
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		d.internal(w, r, "files.download.dir", err)
		return
	}
	free, err := diskFree(downloadDir)
	if err != nil {
		d.internal(w, r, "files.download.statfs", err)
		return
	}
	if free < minFreeForUpload {
		writeError(w, requestIDFrom(r), errDiskFull)
		return
	}
	tmp, err := os.CreateTemp(downloadDir, "download-*")
	if err != nil {
		d.internal(w, r, "files.download.stage", err)
		return
	}
	staged := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(staged) }()

	allowSlowRequest(w, 0, 25*time.Minute)
	export, err := d.Manage.ExportFiles(r.Context(), site, paths, staged)
	if err != nil {
		d.manageError(w, r, "files.download", err)
		return
	}
	f, err := os.Open(staged)
	if err != nil {
		d.internal(w, r, "files.download.open", err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		d.internal(w, r, "files.download.stat", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.file.download", TargetType: "site", TargetID: site.ID, Result: "success"})

	contentType := "application/octet-stream"
	if export.Kind == "zip" {
		contentType = "application/zip"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": export.Name}))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}
