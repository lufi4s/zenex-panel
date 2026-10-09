package httpapi

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// brandingResponse is the branding plus whether a logo and a favicon are uploaded.
type brandingResponse struct {
	store.Branding
	HasLogo    bool `json:"has_logo"`
	HasFavicon bool `json:"has_favicon"`
}

// brandingView loads the branding and which images exist.
func (d Deps) brandingView(r *http.Request) (brandingResponse, error) {
	resp := brandingResponse{Branding: store.DefaultBranding()}
	if d.Sites == nil {
		return resp, nil
	}
	b, err := d.Sites.GetBranding(r.Context())
	if err != nil {
		return resp, err
	}
	assets, err := d.Sites.GetBrandingAssets(r.Context())
	if err != nil {
		return resp, err
	}
	resp.Branding = b
	resp.HasLogo = assets.Logo != nil
	resp.HasFavicon = assets.Favicon != nil
	return resp, nil
}

// handleGetBranding is public: the sign-in page needs the name before anyone signs in.
func (d Deps) handleGetBranding(w http.ResponseWriter, r *http.Request) {
	resp, err := d.brandingView(r)
	if err != nil {
		d.internal(w, r, "branding.get", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
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
	resp, err := d.brandingView(r)
	if err != nil {
		d.internal(w, r, "branding.get", err)
		return
	}
	resp.Branding = b
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Logo and favicon
// ---------------------------------------------------------------------------

// maxBrandingAssetBytes is the largest decoded logo or favicon.
const maxBrandingAssetBytes = 256 << 10

// allowedBrandingMimes are the only image types accepted. SVG is refused on purpose:
// it can carry scripts.
var allowedBrandingMimes = map[string]bool{
	"image/png":    true,
	"image/jpeg":   true,
	"image/webp":   true,
	"image/x-icon": true,
}

// brandingAssetInput is the body of PUT /api/v1/branding/logo and /favicon.
type brandingAssetInput struct {
	Mime string `json:"mime"`
	Data string `json:"data"`
}

// detectImageMime returns the type that the file's leading bytes identify, or "".
func detectImageMime(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0x89, 0x50, 0x4E, 0x47}):
		return "image/png"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp"
	case bytes.HasPrefix(b, []byte{0x00, 0x00, 0x01, 0x00}):
		return "image/x-icon"
	}
	return ""
}

// parseBrandingAsset checks the claimed type, the size and the magic bytes of an upload.
func parseBrandingAsset(in brandingAssetInput) (store.BrandingAsset, *APIError) {
	mime := strings.ToLower(strings.TrimSpace(in.Mime))
	invalid := &APIError{Status: http.StatusBadRequest, Code: "invalid_image", Message: "Upload a PNG, JPEG, WebP or ICO image."}
	if !allowedBrandingMimes[mime] {
		return store.BrandingAsset{}, invalid
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.Data))
	if err != nil || len(raw) == 0 {
		return store.BrandingAsset{}, invalid
	}
	if len(raw) > maxBrandingAssetBytes {
		return store.BrandingAsset{}, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "image_too_large", Message: "Images can be at most 256 KB."}
	}
	if detectImageMime(raw) != mime {
		return store.BrandingAsset{}, invalid
	}
	return store.BrandingAsset{Mime: mime, Data: base64.StdEncoding.EncodeToString(raw)}, nil
}

// brandingAssetSlot returns the field of the assets document for kind ("logo" or "favicon").
func brandingAssetSlot(a *store.BrandingAssets, kind string) **store.BrandingAsset {
	if kind == "logo" {
		return &a.Logo
	}
	return &a.Favicon
}

// handlePutBrandingAsset stores a logo or favicon after the admin check.
func (d Deps) handlePutBrandingAsset(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := d.requireAdminUser(w, r)
		if !ok {
			return
		}
		var in brandingAssetInput
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, requestIDFrom(r), ErrInvalidRequest)
			return
		}
		asset, apiErr := parseBrandingAsset(in)
		if apiErr != nil {
			writeError(w, requestIDFrom(r), *apiErr)
			return
		}
		err := d.Sites.UpdateBrandingAssets(r.Context(), user.ID, func(a *store.BrandingAssets) {
			*brandingAssetSlot(a, kind) = &asset
		})
		if err != nil {
			d.internal(w, r, "branding."+kind+".set", err)
			return
		}
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "branding." + kind + ".update", TargetType: "settings", TargetID: "branding_" + kind, Result: "success"})
		d.writeBrandingView(w, r)
	}
}

// handleDeleteBrandingAsset removes a logo or favicon. Deleting a missing one succeeds.
func (d Deps) handleDeleteBrandingAsset(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := d.requireAdminUser(w, r)
		if !ok {
			return
		}
		err := d.Sites.UpdateBrandingAssets(r.Context(), user.ID, func(a *store.BrandingAssets) {
			*brandingAssetSlot(a, kind) = nil
		})
		if err != nil {
			d.internal(w, r, "branding."+kind+".delete", err)
			return
		}
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "branding." + kind + ".delete", TargetType: "settings", TargetID: "branding_" + kind, Result: "success"})
		d.writeBrandingView(w, r)
	}
}

func (d Deps) writeBrandingView(w http.ResponseWriter, r *http.Request) {
	resp, err := d.brandingView(r)
	if err != nil {
		d.internal(w, r, "branding.get", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetBrandingAsset is public: browsers request the logo and favicon without a session.
// It serves the stored bytes with the stored type and a CSP that forbids scripts.
func (d Deps) handleGetBrandingAsset(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Sites == nil {
			writeError(w, requestIDFrom(r), ErrNotFound)
			return
		}
		assets, err := d.Sites.GetBrandingAssets(r.Context())
		if err != nil {
			d.internal(w, r, "branding."+kind+".get", err)
			return
		}
		asset := *brandingAssetSlot(&assets, kind)
		if asset == nil {
			writeError(w, requestIDFrom(r), ErrNotFound)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(asset.Data)
		if err != nil {
			d.internal(w, r, "branding."+kind+".decode", err)
			return
		}
		h := w.Header()
		h.Set("Content-Type", asset.Mime)
		h.Set("Cache-Control", "public, max-age=300")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}
}
