package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const maxPathLength = 1024

// filePath reads and checks the ?path= query value.
func filePath(r *http.Request) (string, bool) {
	p := r.URL.Query().Get("path")
	return p, len(p) <= maxPathLength && !strings.ContainsRune(p, 0)
}

type filePathRequest struct {
	Path string `json:"path"`
}

type fileContentRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// handleListFiles returns the entries of a folder inside a website.
func (d Deps) handleListFiles(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	p, valid := filePath(r)
	if !valid {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	out, err := d.Manage.Files(r.Context(), site, "files.list", p, "")
	if err != nil {
		d.manageError(w, r, "files.list", err)
		return
	}
	// The helper returns a JSON array as text; pass it through unchanged.
	if out == "" || !json.Valid([]byte(out)) {
		out = "[]"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(out))
}

// handleReadFile returns the text of one file.
func (d Deps) handleReadFile(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	p, valid := filePath(r)
	if !valid || p == "" {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	content, err := d.Manage.Files(r.Context(), site, "files.read", p, "")
	if err != nil {
		d.manageError(w, r, "files.read", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": p, "content": content})
}

// handleWriteFile saves text to a file.
func (d Deps) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	var in fileContentRequest
	if err := decodeJSON(r, &in); err != nil || in.Path == "" || len(in.Path) > maxPathLength {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if _, err := d.Manage.Files(r.Context(), site, "files.write", in.Path, in.Content); err != nil {
		d.manageError(w, r, "files.write", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.file.write", TargetType: "site", TargetID: site.ID, Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateFolder creates a folder inside a website.
func (d Deps) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	var in filePathRequest
	if err := decodeJSON(r, &in); err != nil || in.Path == "" || len(in.Path) > maxPathLength {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if _, err := d.Manage.Files(r.Context(), site, "files.mkdir", in.Path, ""); err != nil {
		d.manageError(w, r, "files.mkdir", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.folder.create", TargetType: "site", TargetID: site.ID, Result: "success"})
	w.WriteHeader(http.StatusCreated)
}

// handleDeleteFile removes a file or folder inside a website.
func (d Deps) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	p, valid := filePath(r)
	if !valid || p == "" {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if _, err := d.Manage.Files(r.Context(), site, "files.delete", p, ""); err != nil {
		d.manageError(w, r, "files.delete", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.file.delete", TargetType: "site", TargetID: site.ID, Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}
