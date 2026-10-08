// Package httpapi wires the REST surface under /api/v1/ and serves the panel UI.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/web"
)

// Version is reported by /api/v1/health. Set at build time.
var Version = "0.2.0-dev"

// Deps carries the collaborators the routes need.
type Deps struct {
	Log           *slog.Logger
	Users         AuthStore
	Sites         SiteStore
	Manage        SiteManager
	Monitor       MonitorStore
	Site          SiteSettings
	SecureCookies bool // true when served over TLS
	LoginLimiter  *fixedWindowLimiter
	SessionTTL    time.Duration
}

// NewRouter returns the fully wrapped handler.
func NewRouter(d Deps) http.Handler {
	if d.LoginLimiter == nil {
		d.LoginLimiter = newFixedWindowLimiter(10, time.Minute)
	}
	if d.SessionTTL == 0 {
		d.SessionTTL = 12 * time.Hour
	}
	mux := http.NewServeMux()

	mux.Handle("GET /api/v1/health", http.HandlerFunc(d.handleHealth))
	mux.Handle("POST /api/v1/auth/login", requireCSRF(http.HandlerFunc(d.handleLogin)))
	mux.Handle("POST /api/v1/auth/logout", requireCSRF(d.requireSession(d.handleLogout)))
	mux.Handle("GET /api/v1/auth/me", d.requireSession(d.handleMe))
	mux.Handle("GET /api/v1/system/metrics", d.requireSession(d.handleMetrics))

	mux.Handle("GET /api/v1/domains", d.requireSession(d.handleListDomains))
	mux.Handle("POST /api/v1/domains", requireCSRF(d.requireSession(d.handleConnectDomain)))
	mux.Handle("GET /api/v1/sites", d.requireSession(d.handleListSites))
	mux.Handle("POST /api/v1/sites", requireCSRF(d.requireSession(d.handleCreateSite)))
	mux.Handle("GET /api/v1/sites/{id}", d.requireSession(d.handleGetSite))
	mux.Handle("GET /api/v1/sites/{id}/credentials", d.requireSession(d.handleSiteCredentials))
	mux.Handle("GET /api/v1/jobs/{id}", d.requireSession(d.handleGetJob))
	mux.Handle("POST /api/v1/jobs/{id}/retry", requireCSRF(d.requireSession(d.handleRetryJob)))
	mux.Handle("POST /api/v1/domains/{id}/verify", requireCSRF(d.requireSession(d.handleVerifyDomain)))
	mux.Handle("POST /api/v1/sites/{id}/suspend", requireCSRF(d.requireSession(d.handleSuspendSite())))
	mux.Handle("POST /api/v1/sites/{id}/resume", requireCSRF(d.requireSession(d.handleResumeSite())))
	mux.Handle("POST /api/v1/sites/{id}/php-restart", requireCSRF(d.requireSession(d.handleRestartPHP())))
	mux.Handle("POST /api/v1/sites/{id}/php", requireCSRF(d.requireSession(d.handleSwitchPHP)))
	mux.Handle("GET /api/v1/sites/{id}/logs", d.requireSession(d.handleSiteLogs))
	mux.Handle("DELETE /api/v1/sites/{id}", requireCSRF(d.requireSession(d.handleDeleteSite)))
	mux.Handle("GET /api/v1/php-versions", d.requireSession(d.handleListPHPVersions))
	mux.Handle("GET /api/v1/notifications", d.requireSession(d.handleListNotifications))
	mux.Handle("POST /api/v1/notifications/read", requireCSRF(d.requireSession(d.handleMarkNotificationsRead)))
	mux.Handle("DELETE /api/v1/domains/{id}", requireCSRF(d.requireSession(d.handleDeleteDomain)))
	mux.Handle("GET /api/v1/monitoring/metrics", d.requireSession(d.handleMetricSeries))
	mux.Handle("GET /api/v1/monitoring/sites", d.requireSession(d.handleSiteHealth))
	mux.Handle("GET /api/v1/monitoring/services", d.requireSession(d.handleServices))
	mux.Handle("GET /api/v1/activity", d.requireSession(d.handleActivity))
	mux.Handle("GET /api/v1/jobs/{id}/logs", d.requireSession(d.handleJobLogs))
	mux.Handle("GET /api/v1/sites/{id}/files", d.requireSession(d.handleListFiles))
	mux.Handle("GET /api/v1/sites/{id}/file", d.requireSession(d.handleReadFile))
	mux.Handle("PUT /api/v1/sites/{id}/file", requireCSRF(d.requireSession(d.handleWriteFile)))
	mux.Handle("POST /api/v1/sites/{id}/folders", requireCSRF(d.requireSession(d.handleCreateFolder)))
	mux.Handle("DELETE /api/v1/sites/{id}/files", requireCSRF(d.requireSession(d.handleDeleteFile)))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, requestIDFrom(r), ErrNotFound)
	})

	// UI: the single-page app at "/" and its hashed assets under /assets/.
	ui := web.FS()
	mux.HandleFunc("GET /{$}", serveIndex(ui))
	mux.Handle("GET /assets/", http.FileServerFS(ui))

	var h http.Handler = mux
	h = limitBody(h)
	h = securityHeaders(h)
	h = accessLog(d.Log)(h)
	h = recoverPanic(d.Log)(h)
	h = requestID(h)
	return h
}

func serveIndex(ui fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(ui, "index.html")
		if err != nil {
			writeError(w, requestIDFrom(r), ErrInternal)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Version  string `json:"version"`
}

// handleHealth reports whether the API and its database are working.
// It returns 503 when the database cannot be reached so monitors and the
// installer can detect the problem.
func (d Deps) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{Status: "ok", Database: "not_configured", Version: Version}
	code := http.StatusOK
	if d.Users != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := d.Users.Ping(ctx); err != nil {
			resp.Status, resp.Database = "degraded", "unreachable"
			code = http.StatusServiceUnavailable
		} else {
			resp.Database = "ok"
		}
	}
	writeJSON(w, code, resp)
}
