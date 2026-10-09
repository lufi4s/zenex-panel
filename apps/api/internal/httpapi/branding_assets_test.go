package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// brandingSites keeps the branding and image state for the asset tests.
type brandingSites struct {
	SiteStore
	branding store.Branding
	assets   store.BrandingAssets
}

func (b *brandingSites) GetBranding(context.Context) (store.Branding, error) {
	return b.branding, nil
}
func (b *brandingSites) SetBranding(_ context.Context, _ string, v store.Branding) error {
	b.branding = v
	return nil
}
func (b *brandingSites) GetBrandingAssets(context.Context) (store.BrandingAssets, error) {
	return b.assets, nil
}
func (b *brandingSites) UpdateBrandingAssets(_ context.Context, _ string, change func(*store.BrandingAssets)) error {
	change(&b.assets)
	return nil
}

// newBrandingEnv is the settings environment with the branding store swapped in.
func newBrandingEnv(t *testing.T) (settingsEnv, *brandingSites) {
	t.Helper()
	e := newSettingsEnv(t)
	bs := &brandingSites{branding: store.DefaultBranding()}
	e.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  e.fs,
		Sites:  bs,
		Manage: e.manage,
		Alerts: e.alert,
		Site:   SiteSettings{NodeID: "node-1", PHPVersion: "8.3", StartJob: func(string) {}},
	})
	return e, bs
}

// Minimal valid headers for each accepted type.
var (
	pngBytes  = append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}, make([]byte, 32)...)
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 32)...)
	webpBytes = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	icoBytes  = append([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}, make([]byte, 32)...)
)

func assetBody(mime string, raw []byte) string {
	return `{"mime":"` + mime + `","data":"` + base64.StdEncoding.EncodeToString(raw) + `"}`
}

func TestDetectImageMimeChecksMagicBytes(t *testing.T) {
	cases := map[string]struct {
		raw  []byte
		want string
	}{
		"png":           {pngBytes, "image/png"},
		"jpeg":          {jpegBytes, "image/jpeg"},
		"webp":          {webpBytes, "image/webp"},
		"ico":           {icoBytes, "image/x-icon"},
		"svg":           {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ""},
		"riff but wav":  {[]byte("RIFF\x24\x00\x00\x00WAVEfmt "), ""},
		"short riff":    {[]byte("RIFF"), ""},
		"empty":         {nil, ""},
		"gif":           {[]byte("GIF89a\x01\x00"), ""},
		"truncated png": {[]byte{0x89, 'P', 'N'}, ""},
	}
	for name, c := range cases {
		if got := detectImageMime(c.raw); got != c.want {
			t.Errorf("%s: detected %q, want %q", name, got, c.want)
		}
	}
}

func TestParseBrandingAssetRejectsWhatItShouldNot(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	bad := map[string]brandingAssetInput{
		"svg claimed as png":  {Mime: "image/png", Data: base64.StdEncoding.EncodeToString(svg)},
		"svg mime":            {Mime: "image/svg+xml", Data: base64.StdEncoding.EncodeToString(svg)},
		"png claim with jpeg": {Mime: "image/png", Data: base64.StdEncoding.EncodeToString(jpegBytes)},
		"jpeg claim with png": {Mime: "image/jpeg", Data: base64.StdEncoding.EncodeToString(pngBytes)},
		"gif mime":            {Mime: "image/gif", Data: base64.StdEncoding.EncodeToString([]byte("GIF89a"))},
		"not base64":          {Mime: "image/png", Data: "%%%not-base64%%%"},
		"empty data":          {Mime: "image/png", Data: ""},
		"empty mime":          {Mime: "", Data: base64.StdEncoding.EncodeToString(pngBytes)},
	}
	for name, in := range bad {
		_, apiErr := parseBrandingAsset(in)
		if apiErr == nil || apiErr.Code != "invalid_image" || apiErr.Status != http.StatusBadRequest {
			t.Errorf("%s: accepted or wrong error: %+v", name, apiErr)
		}
	}
	for _, in := range []brandingAssetInput{
		{Mime: "image/png", Data: base64.StdEncoding.EncodeToString(pngBytes)},
		{Mime: " IMAGE/X-ICON ", Data: base64.StdEncoding.EncodeToString(icoBytes)},
	} {
		if asset, apiErr := parseBrandingAsset(in); apiErr != nil || asset.Mime == "" {
			t.Errorf("valid %q rejected: %+v", in.Mime, apiErr)
		}
	}
}

func TestBrandingAssetSizeLimit(t *testing.T) {
	atLimit := append(append([]byte{}, pngBytes...), make([]byte, maxBrandingAssetBytes-len(pngBytes))...)
	if len(atLimit) != maxBrandingAssetBytes {
		t.Fatalf("test setup: %d bytes", len(atLimit))
	}
	if _, apiErr := parseBrandingAsset(brandingAssetInput{Mime: "image/png", Data: base64.StdEncoding.EncodeToString(atLimit)}); apiErr != nil {
		t.Fatalf("256 KB rejected: %+v", apiErr)
	}
	over := append(append([]byte{}, atLimit...), 0)
	_, apiErr := parseBrandingAsset(brandingAssetInput{Mime: "image/png", Data: base64.StdEncoding.EncodeToString(over)})
	if apiErr == nil || apiErr.Status != http.StatusRequestEntityTooLarge || apiErr.Code != "image_too_large" {
		t.Fatalf("256 KB + 1 byte: %+v", apiErr)
	}
}

func TestBrandingAssetEndpointsAdminOnlyAndCSRF(t *testing.T) {
	e, _ := newBrandingEnv(t)
	admin := e.login(t, testEmail)
	customer := e.login(t, "customer@example.com")
	body := assetBody("image/png", pngBytes)

	if rec := e.call(t, http.MethodPut, "/api/v1/branding/logo", body, admin, false); rec.Code != http.StatusForbidden {
		t.Fatalf("PUT without CSRF = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/branding/logo", body, customer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("PUT as customer = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodDelete, "/api/v1/branding/favicon", "", customer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE as customer = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/branding/logo", `{"mime":"image/png","data":"AA==","extra":1}`, admin, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", rec.Code)
	}
}

func TestBrandingLogoLifecycleAndPublicHeaders(t *testing.T) {
	e, bs := newBrandingEnv(t)
	admin := e.login(t, testEmail)

	// Nothing uploaded yet: public GET is 404 and the flags are false.
	if rec := e.call(t, http.MethodGet, "/api/v1/branding/logo", "", nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("absent logo = %d", rec.Code)
	}

	rec := e.call(t, http.MethodPut, "/api/v1/branding/logo", assetBody("image/png", pngBytes), admin, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"has_logo":true`) || !strings.Contains(rec.Body.String(), `"has_favicon":false`) {
		t.Fatalf("PUT logo = %d %s", rec.Code, rec.Body.String())
	}
	if bs.assets.Logo == nil || bs.assets.Logo.Mime != "image/png" {
		t.Fatalf("stored logo = %+v", bs.assets.Logo)
	}

	// Public GET needs no session and returns the exact bytes with safety headers.
	rec = e.call(t, http.MethodGet, "/api/v1/branding/logo", "", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatal("served bytes differ from the upload")
	}
	want := map[string]string{
		"Content-Type":            "image/png",
		"Cache-Control":           "public, max-age=300",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; img-src 'self'",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}

	// The public branding JSON reports the flag and keeps the old fields.
	rec = e.call(t, http.MethodGet, "/api/v1/branding", "", nil, false)
	var got brandingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.HasLogo || got.HasFavicon || got.Name == "" {
		t.Fatalf("GET branding = %s", rec.Body.String())
	}

	rec = e.call(t, http.MethodDelete, "/api/v1/branding/logo", "", admin, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"has_logo":false`) {
		t.Fatalf("DELETE logo = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodGet, "/api/v1/branding/logo", "", nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("logo after delete = %d", rec.Code)
	}
}

func TestBrandingFaviconIsIndependentAndAudited(t *testing.T) {
	e, bs := newBrandingEnv(t)
	admin := e.login(t, testEmail)
	if rec := e.call(t, http.MethodPut, "/api/v1/branding/logo", assetBody("image/png", pngBytes), admin, true); rec.Code != http.StatusOK {
		t.Fatalf("logo = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/branding/favicon", assetBody("image/x-icon", icoBytes), admin, true); rec.Code != http.StatusOK {
		t.Fatalf("favicon = %d %s", rec.Code, rec.Body.String())
	}
	if bs.assets.Logo == nil || bs.assets.Favicon == nil {
		t.Fatal("saving the favicon dropped the logo")
	}
	rec := e.call(t, http.MethodGet, "/api/v1/branding/favicon", "", nil, false)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/x-icon" || !bytes.Equal(rec.Body.Bytes(), icoBytes) {
		t.Fatalf("GET favicon = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	if rec := e.call(t, http.MethodPut, "/api/v1/branding/favicon", assetBody("image/svg+xml", []byte("<svg/>")), admin, true); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_image" {
		t.Fatalf("SVG favicon = %d %s", rec.Code, rec.Body.String())
	}

	if rec := e.call(t, http.MethodDelete, "/api/v1/branding/favicon", "", admin, true); rec.Code != http.StatusOK {
		t.Fatalf("delete favicon = %d", rec.Code)
	}
	if bs.assets.Favicon != nil || bs.assets.Logo == nil {
		t.Fatalf("delete favicon touched the logo: %+v", bs.assets)
	}

	actions := []string{}
	for _, a := range e.fs.audits {
		actions = append(actions, a.Action)
	}
	joined := strings.Join(actions, ",")
	for _, want := range []string{"branding.logo.update", "branding.favicon.update", "branding.favicon.delete"} {
		if !strings.Contains(joined, want) {
			t.Errorf("audit %s missing from %s", want, joined)
		}
	}
}

func TestBrandingAssetOversizeRequestReturns413(t *testing.T) {
	e, _ := newBrandingEnv(t)
	admin := e.login(t, testEmail)
	big := append(append([]byte{}, pngBytes...), make([]byte, maxBrandingAssetBytes)...)
	rec := e.call(t, http.MethodPut, "/api/v1/branding/logo", assetBody("image/png", big), admin, true)
	if rec.Code != http.StatusRequestEntityTooLarge || errorCode(t, rec) != "image_too_large" {
		t.Fatalf("oversize = %d %s", rec.Code, rec.Body.String())
	}
}
