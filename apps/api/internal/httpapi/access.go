package httpapi

import (
	"context"
	"net/http"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/provision"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// siteAccess is what a website's owner or an administrator needs to manage its
// database and files: the database login for phpMyAdmin, and the SFTP login.
type siteAccess struct {
	Database   databaseAccess   `json:"database"`
	PhpMyAdmin phpMyAdminAccess `json:"phpmyadmin"`
	SFTP       sftpAccess       `json:"sftp"`
}

type databaseAccess struct {
	Host     string `json:"host"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type phpMyAdminAccess struct {
	// URL opens phpMyAdmin under the website's address. Sign in with the database login above.
	URL string `json:"url"`
}

type sftpAccess struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	// Password is only set while SFTP is enabled.
	Password string `json:"password"`
}

const sftpPort = 22

func (d Deps) accessView(site store.Site) siteAccess {
	key := d.Site.SecretKey
	view := siteAccess{
		Database: databaseAccess{
			Host:     "localhost",
			Name:     site.DBName,
			User:     site.DBUser,
			Password: provision.DBPassword(key, site.ID),
		},
		PhpMyAdmin: phpMyAdminAccess{URL: "https://" + site.Domain + "/zenex-dbadmin/"},
		SFTP: sftpAccess{
			Enabled: site.SFTPEnabled,
			Host:    site.Domain,
			Port:    sftpPort,
			User:    site.LinuxUser,
		},
	}
	if site.SFTPEnabled {
		view.SFTP.Password = provision.SFTPPassword(key, site.ID, site.SFTPGeneration)
	}
	return view
}

// handleSiteAccess returns the database and SFTP logins. Only the website's owner and
// administrators reach it; every read is audited.
func (d Deps) handleSiteAccess(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.access.view", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusOK, d.accessView(site))
}

// handleEnableSFTP turns SFTP on for the website. ?reset=1 issues a new password.
func (d Deps) handleEnableSFTP(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	reset := r.URL.Query().Get("reset") == "1"
	if err := d.Manage.SetSFTP(r.Context(), site, true, reset); err != nil {
		d.manageError(w, r, "sites.sftp", err)
		return
	}
	action := "site.sftp.enable"
	if reset {
		action = "site.sftp.reset"
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: action, TargetType: "site", TargetID: site.ID, Result: "success"})
	d.writeAccessAfterChange(w, r, site.ID)
}

// handleDisableSFTP turns SFTP off. The password is locked; the files stay as they are.
func (d Deps) handleDisableSFTP(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	if err := d.Manage.SetSFTP(r.Context(), site, false, false); err != nil {
		d.manageError(w, r, "sites.sftp", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.sftp.disable", TargetType: "site", TargetID: site.ID, Result: "success"})
	d.writeAccessAfterChange(w, r, site.ID)
}

// handleOpenPhpMyAdmin prepares the website so phpMyAdmin answers under its address,
// and returns that address.
func (d Deps) handleOpenPhpMyAdmin(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	if err := d.Manage.PrepareDBAdmin(r.Context(), site); err != nil {
		d.manageError(w, r, "sites.phpmyadmin", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.phpmyadmin.open", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusOK, d.accessView(site).PhpMyAdmin)
}

// writeAccessAfterChange answers with the updated access view, read back from the store.
func (d Deps) writeAccessAfterChange(w http.ResponseWriter, r *http.Request, siteID string) {
	updated, err := d.Sites.GetSite(context.WithoutCancel(r.Context()), siteID)
	if err != nil {
		d.internal(w, r, "sites.access", err)
		return
	}
	writeJSON(w, http.StatusOK, d.accessView(updated))
}
