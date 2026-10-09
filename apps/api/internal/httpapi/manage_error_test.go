package httpapi

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
)

func TestManageErrorSaysWhenTheHelperIsNotRunning(t *testing.T) {
	d := Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	d.manageError(rec, r, "test", fmt.Errorf("%w: dial unix /run/zenex/helper.sock: no such file", helperclient.ErrUnreachable))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := errorCode(t, rec); got != "helper_unavailable" {
		t.Fatalf("code = %q", got)
	}
}

func TestManageErrorStillHidesUnknownFaults(t *testing.T) {
	d := Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	d.manageError(rec, httptest.NewRequest(http.MethodGet, "/", nil), "test", errors.New("pq: password authentication failed for user x"))
	if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal_error" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}
