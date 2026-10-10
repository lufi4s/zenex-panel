package httpapi

import (
	"net/http"
	"regexp"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

var (
	quarantineItemRe = regexp.MustCompile(`^[a-f0-9]{24}$`)
	scanRuleRe       = regexp.MustCompile(`^[a-z0-9-]{0,40}$`)
)

// handleSecurityStatus returns the scanner state of a website: whether a scan runs, the last
// report and the quarantined files.
func (d Deps) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	status, err := d.Manage.SecurityStatus(r.Context(), site)
	if err != nil {
		d.manageError(w, r, "security.status", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleSecurityScan starts a malware scan in the background.
func (d Deps) handleSecurityScan(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	if err := d.Manage.StartScan(r.Context(), site); err != nil {
		d.manageError(w, r, "security.scan", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.malware_scan", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "running"})
}

type quarantineRequest struct {
	Path string `json:"path"`
	Rule string `json:"rule"`
}

// handleSecurityQuarantine moves a file out of the website into quarantine.
func (d Deps) handleSecurityQuarantine(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	var in quarantineRequest
	if err := decodeJSON(r, &in); err != nil || in.Path == "" || len(in.Path) > maxPathLength || !scanRuleRe.MatchString(in.Rule) {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if err := d.Manage.Quarantine(r.Context(), site, in.Path, in.Rule); err != nil {
		d.manageError(w, r, "security.quarantine", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.malware_quarantine", TargetType: "site", TargetID: site.ID, Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}

// handleSecurityRestore puts a quarantined file back.
func (d Deps) handleSecurityRestore(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	item := r.PathValue("item")
	if !quarantineItemRe.MatchString(item) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	if err := d.Manage.RestoreQuarantined(r.Context(), site, item); err != nil {
		d.manageError(w, r, "security.restore", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.malware_restore", TargetType: "site", TargetID: site.ID, Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}

// handleSecurityDelete removes a quarantined file for good.
func (d Deps) handleSecurityDelete(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	item := r.PathValue("item")
	if !quarantineItemRe.MatchString(item) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	if err := d.Manage.DeleteQuarantined(r.Context(), site, item); err != nil {
		d.manageError(w, r, "security.delete", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.malware_delete", TargetType: "site", TargetID: site.ID, Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}
