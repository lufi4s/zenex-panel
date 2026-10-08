// Package httpapi wires the REST surface under /api/v1/.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Version is the API version reported by /api/v1/health. Set at build time.
var Version = "0.1.0-dev"

// NewRouter returns the fully wrapped API handler.
func NewRouter(log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", handleHealth)

	// Any path under /api/ that does not match a route gets a structured 404
	// instead of the default plain-text response.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, requestIDFrom(r), ErrNotFound)
	})

	var h http.Handler = mux
	h = limitBody(h)
	h = securityHeaders(h)
	h = accessLog(log)(h)
	h = recoverPanic(log)(h)
	h = requestID(h)
	return h
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: Version})
}
