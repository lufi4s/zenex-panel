package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// NotificationStore is the persistence surface for notifications and domain removal.
// *store.Store satisfies it.
type NotificationStore interface {
	Notify(ctx context.Context, userID, level, title, body string) error
	ListNotifications(ctx context.Context, userID string, limit int) ([]store.Notification, error)
	CountUnread(ctx context.Context, userID string) (int, error)
	MarkNotificationsRead(ctx context.Context, userID string, ids []int64, all bool) error
	DeleteDomain(ctx context.Context, ownerID, id string) error
	DomainLiveSites(ctx context.Context, id string) (int, error)
}

type notificationsResponse struct {
	Items  []store.Notification `json:"items"`
	Unread int                  `json:"unread"`
}

func (d Deps) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = min(v, 200)
	}
	items, err := d.Sites.ListNotifications(r.Context(), user.ID, limit)
	if err != nil {
		d.internal(w, r, "notifications.list", err)
		return
	}
	unread, err := d.Sites.CountUnread(r.Context(), user.ID)
	if err != nil {
		d.internal(w, r, "notifications.count", err)
		return
	}
	writeJSON(w, http.StatusOK, notificationsResponse{Items: items, Unread: unread})
}

type markReadRequest struct {
	IDs []int64 `json:"ids"`
	All bool    `json:"all"`
}

func (d Deps) handleMarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	var in markReadRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if len(in.IDs) > 500 {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if err := d.Sites.MarkNotificationsRead(r.Context(), user.ID, in.IDs, in.All); err != nil {
		d.internal(w, r, "notifications.read", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteDomain removes a connected domain. A domain that still serves live
// websites cannot be removed; the customer is told how many to delete first.
func (d Deps) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	dom, err := d.Sites.GetDomainOwned(r.Context(), user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	if err != nil {
		d.internal(w, r, "domains.delete", err)
		return
	}

	err = d.Sites.DeleteDomain(r.Context(), user.ID, id)
	if errors.Is(err, store.ErrDomainInUse) {
		n, _ := d.Sites.DomainLiveSites(r.Context(), id)
		writeError(w, requestIDFrom(r), APIError{
			Status:  http.StatusConflict,
			Code:    "domain_in_use",
			Message: fmt.Sprintf("%s still has %d live website%s. Delete or move %s first.", dom.Apex, n, plural(n), theyOrIt(n)),
		})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	if err != nil {
		d.internal(w, r, "domains.delete", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "domain.delete", TargetType: "domain", TargetID: dom.Apex, Result: "success"})
	d.notify(r, user.ID, "info", "Domain removed", dom.Apex+" was removed from your account.")
	w.WriteHeader(http.StatusNoContent)
}

// notify records a notification and never fails the request that caused it.
func (d Deps) notify(r *http.Request, userID, level, title, body string) {
	if err := d.Sites.Notify(r.Context(), userID, level, title, body); err != nil {
		d.Log.Warn("saving notification failed", "request_id", requestIDFrom(r), "operation", "notify", "error", err)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func theyOrIt(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
