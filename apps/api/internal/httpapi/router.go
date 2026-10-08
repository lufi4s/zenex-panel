// Package httpapi wires the REST surface under /api/v1/ and serves the panel UI.
package httpapi

import (
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

	mux.HandleFunc("GET /api/v1/health", handleHealth)
	mux.Handle("POST /api/v1/auth/login", requireCSRF(http.HandlerFunc(d.handleLogin)))
	mux.Handle("POST /api/v1/auth/logout", requireCSRF(d.requireSession(d.handleLogout)))
	mux.Handle("GET /api/v1/auth/me", d.requireSession(d.handleMe))
	mux.Handle("GET /api/v1/system/metrics", d.requireSession(d.handleMetrics))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, requestIDFrom(r), ErrNotFound)
	})

	// UI: index page at "/" and embedded assets under /assets/.
	mux.HandleFunc("GET /{$}", serveIndex)
	assets, _ := fs.Sub(web.FS, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))

	var h http.Handler = mux
	h = limitBody(h)
	h = securityHeaders(h)
	h = accessLog(d.Log)(h)
	h = recoverPanic(d.Log)(h)
	h = requestID(h)
	return h
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(web.FS, "index.html")
	if err != nil {
		writeError(w, requestIDFrom(r), ErrInternal)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: Version})
}
