package httpapi

import (
	"context"
	"net/http"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const (
	sessionCookie = "zenex_session"
	csrfHeader    = "X-Requested-With"
	csrfValue     = "zenex"
)

// currentUser returns the authenticated account attached by requireSession.
func currentUser(r *http.Request) (*store.User, bool) {
	u, ok := r.Context().Value(userKey).(*store.User)
	return u, ok
}

// requireCSRF rejects state-changing API calls that lack the custom header.
// A browser cannot send a custom header cross-site without a CORS preflight,
// and the API sends no CORS grants, so this blocks cross-site form posts.
func requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get(csrfHeader) != csrfValue {
				writeError(w, requestIDFrom(r), ErrCSRF)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireSession resolves the session cookie to a user or returns 401.
func (d Deps) requireSession(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Users == nil {
			writeError(w, requestIDFrom(r), ErrUnavailable)
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeError(w, requestIDFrom(r), ErrUnauthorized)
			return
		}
		user, err := d.Users.SessionUser(r.Context(), hashToken(c.Value))
		if err != nil {
			writeError(w, requestIDFrom(r), ErrUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next(w, r.WithContext(ctx))
	})
}
