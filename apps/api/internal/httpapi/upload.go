package httpapi

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// maxUploadFileBytes is the largest file that can be uploaded. The helper enforces the same limit.
const maxUploadFileBytes = 64 << 20

// maxUploadBodyBytes allows the file plus multipart overhead.
const maxUploadBodyBytes = maxUploadFileBytes + 1<<20

var errUploadTooLarge = APIError{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: "The file is larger than 64 MB."}

// isFileUpload reports whether a request is a file upload, which may be larger than other requests.
func isFileUpload(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/files/upload")
}

// handleUploadFile saves an uploaded file into a folder of the website. The file is streamed
// to the upload folder first; the helper then moves it into the website and never replaces
// an existing file.
func (d Deps) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	dir, valid := filePath(r)
	if !valid {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	part, err := nextFormFile(reader)
	if err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	defer func() { _ = part.Close() }()

	name := baseFileName(part.FileName())
	if name == "" {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	staged, err := stageUpload(part)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, requestIDFrom(r), errUploadTooLarge)
			return
		}
		d.internal(w, r, "files.upload.stage", err)
		return
	}
	defer func() { _ = os.Remove(staged) }()

	if err := d.Manage.ImportFile(r.Context(), site, dir, name, staged); err != nil {
		d.manageError(w, r, "files.upload", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.file.upload", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}

// nextFormFile returns the part named "file" and skips the other form fields.
func nextFormFile(reader *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" {
			return part, nil
		}
		_ = part.Close()
	}
}

// baseFileName returns the last element of a name a browser sent, for both / and \ separators.
func baseFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// stageUpload copies the upload to a new file in the upload folder and returns its path.
// A partial file is removed on failure.
func stageUpload(src io.Reader) (string, error) {
	if err := os.MkdirAll(manage.UploadDir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(manage.UploadDir, "upload-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(name)
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	return name, nil
}

// handleWPLoginLink prepares a one-time link that opens the website's admin dashboard already
// signed in. The link is opened on the website itself, so the browser keeps the login.
func (d Deps) handleWPLoginLink(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	link, expires, err := d.Manage.WPLoginLink(r.Context(), site, time.Now())
	if err != nil {
		d.manageError(w, r, "sites.wp_login_link", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.wp_login_link", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusOK, map[string]string{"url": link, "expires_at": expires.UTC().Format(time.RFC3339)})
}
