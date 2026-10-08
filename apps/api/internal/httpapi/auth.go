package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/auth"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const (
	maxLoginFailures = 5
	accountLockFor   = 15 * time.Minute
)

// AuthStore is the persistence surface the auth handlers need. *store.Store satisfies it.
type AuthStore interface {
	UserByEmail(ctx context.Context, email string) (*store.User, error)
	RecordFailedLogin(ctx context.Context, userID string, maxFailures int, lockFor time.Duration) error
	RecordSuccessfulLogin(ctx context.Context, userID string) error
	CreateSession(ctx context.Context, in store.SessionInput) error
	SessionUser(ctx context.Context, tokenHash []byte) (*store.User, error)
	RevokeSession(ctx context.Context, tokenHash []byte) error
	Audit(ctx context.Context, e store.AuditEntry) error
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

func (d Deps) handleLogin(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFrom(r)
	ip := clientIP(r)

	if !d.LoginLimiter.Allow(ip) {
		writeError(w, reqID, ErrRateLimited)
		return
	}

	var in loginRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, reqID, ErrInvalidRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !validEmail(email) || len(in.Password) == 0 || len(in.Password) > 1024 {
		writeError(w, reqID, ErrInvalidRequest)
		return
	}

	ctx := r.Context()
	user, err := d.Users.UserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		// Spend the same time as a real check so unknown emails are not distinguishable.
		_, _ = auth.VerifyPassword(in.Password, auth.DummyHash())
		d.audit(ctx, r, store.AuditEntry{Action: "login.failure", Result: "failure", ErrorCode: ErrInvalidCreds.Code})
		writeError(w, reqID, ErrInvalidCreds)
		return
	}
	if err != nil {
		d.Log.Error("login lookup failed", "request_id", reqID, "operation", "auth.login", "error_code", ErrUnavailable.Code, "error", err)
		writeError(w, reqID, ErrUnavailable)
		return
	}

	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		d.audit(ctx, r, store.AuditEntry{ActorUserID: user.ID, Action: "login.locked", Result: "denied", ErrorCode: ErrAccountLocked.Code})
		writeError(w, reqID, ErrAccountLocked)
		return
	}

	ok, err := auth.VerifyPassword(in.Password, user.PasswordHash)
	if err != nil || user.Status != "active" || !ok {
		_ = d.Users.RecordFailedLogin(ctx, user.ID, maxLoginFailures, accountLockFor)
		d.audit(ctx, r, store.AuditEntry{ActorUserID: user.ID, Action: "login.failure", Result: "failure", ErrorCode: ErrInvalidCreds.Code})
		writeError(w, reqID, ErrInvalidCreds)
		return
	}

	token, hash, err := auth.NewToken()
	if err != nil {
		d.Log.Error("token generation failed", "request_id", reqID, "operation", "auth.login", "error_code", ErrInternal.Code, "error", err)
		writeError(w, reqID, ErrInternal)
		return
	}
	expires := time.Now().Add(d.SessionTTL)
	if err := d.Users.CreateSession(ctx, store.SessionInput{
		UserID: user.ID, TokenHash: hash, IP: ip, UserAgent: r.UserAgent(), ExpiresAt: expires,
	}); err != nil {
		d.Log.Error("session create failed", "request_id", reqID, "operation", "auth.login", "error_code", ErrUnavailable.Code, "error", err)
		writeError(w, reqID, ErrUnavailable)
		return
	}
	_ = d.Users.RecordSuccessfulLogin(ctx, user.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(d.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   d.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	d.audit(ctx, r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "login.success", Result: "success"})
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (d Deps) handleLogout(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = d.Users.RevokeSession(r.Context(), auth.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "logout", Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (d Deps) audit(ctx context.Context, r *http.Request, e store.AuditEntry) {
	e.IP = clientIP(r)
	if err := d.Users.Audit(ctx, e); err != nil {
		// Audit failures are logged loudly; the request outcome is not changed.
		d.Log.Error("audit write failed", "request_id", requestIDFrom(r), "operation", e.Action, "error_code", "audit_failed", "error", err)
	}
}

func toUserResponse(u *store.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Roles: u.Roles}
}

func primaryRole(u *store.User) string {
	if len(u.Roles) == 0 {
		return ""
	}
	return u.Roles[0]
}

// hashToken is a local alias so handlers do not import the auth package twice.
func hashToken(token string) []byte { return auth.HashToken(token) }

// clientIP returns the TCP peer address. X-Forwarded-For is ignored because
// nothing sits in front of the API that could set it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return host
}

func validEmail(s string) bool {
	if len(s) == 0 || len(s) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}
