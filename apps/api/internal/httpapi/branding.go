package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// handleGetBranding is public: the sign-in page needs the name before anyone signs in.
func (d Deps) handleGetBranding(w http.ResponseWriter, r *http.Request) {
	b := store.DefaultBranding()
	if d.Sites != nil {
		loaded, err := d.Sites.GetBranding(r.Context())
		if err != nil {
			d.internal(w, r, "branding.get", err)
			return
		}
		b = loaded
	}
	writeJSON(w, http.StatusOK, b)
}

// validBranding checks the fields and returns a trimmed copy.
func validBranding(in store.Branding) (store.Branding, string) {
	in.Name = strings.TrimSpace(in.Name)
	in.Tagline = strings.TrimSpace(in.Tagline)
	in.PrimaryColor = strings.ToLower(strings.TrimSpace(in.PrimaryColor))
	if n := len([]rune(in.Name)); n < 2 || n > 40 {
		return in, "The name must be between 2 and 40 characters."
	}
	if len([]rune(in.Tagline)) > 120 {
		return in, "The tagline can be at most 120 characters."
	}
	for _, r := range in.Name + in.Tagline {
		if unicode.IsControl(r) {
			return in, "The name and tagline cannot contain control characters."
		}
	}
	if !colorRe.MatchString(in.PrimaryColor) {
		return in, "Choose a colour in the form #rrggbb."
	}
	return in, ""
}

// handleSetBranding changes the panel's branding. Administrators only.
func (d Deps) handleSetBranding(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if !isAdmin(user) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Only administrators can change the branding."})
		return
	}
	var in store.Branding
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	b, problem := validBranding(in)
	if problem != "" {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_branding", Message: problem})
		return
	}
	if err := d.Sites.SetBranding(r.Context(), user.ID, b); err != nil {
		d.internal(w, r, "branding.set", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "branding.update", TargetType: "settings", TargetID: "branding", Result: "success"})
	writeJSON(w, http.StatusOK, b)
}
