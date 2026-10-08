package httpapi

import (
	"errors"
	"net/http"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

// handleMetrics returns live host metrics read from the kernel.
func (d Deps) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := sysinfo.Snapshot()
	if errors.Is(err, sysinfo.ErrUnsupported) {
		writeError(w, requestIDFrom(r), ErrUnsupportedHost)
		return
	}
	if err != nil {
		d.Log.Error("metrics read failed", "request_id", requestIDFrom(r), "operation", "system.metrics", "error_code", ErrInternal.Code, "error", err)
		writeError(w, requestIDFrom(r), ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, m)
}
