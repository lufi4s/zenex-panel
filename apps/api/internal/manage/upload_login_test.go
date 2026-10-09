package manage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func TestWPLoginLinkIsSignedShortLivedAndPreparesPlugin(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	key := strings.Repeat("ab", 16)
	m.AutoLoginKey = func(string) string { return key }
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "ready"}
	now := time.Unix(1_800_000_000, 0)

	link, expires, err := m.WPLoginLink(context.Background(), site, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.args["wp.autologin"]; len(got) != 1 || got[0]["key"] != key || got[0]["login"] != WPAdminLogin {
		t.Fatalf("wp.autologin args = %v", got)
	}
	if !expires.Equal(now.Add(loginLinkTTL)) {
		t.Fatalf("expires = %v", expires)
	}

	prefix := "https://shop.example.com/?zenex_autologin="
	if !strings.HasPrefix(link, prefix) {
		t.Fatalf("link = %q", link)
	}
	parts := strings.Split(strings.TrimPrefix(link, prefix), ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d", len(parts))
	}
	if exp, err := strconv.ParseInt(parts[0], 10, 64); err != nil || exp != now.Add(loginLinkTTL).Unix() {
		t.Fatalf("expiry = %q", parts[0])
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if want := hex.EncodeToString(mac.Sum(nil)); !hmac.Equal([]byte(want), []byte(parts[2])) {
		t.Fatal("signature does not verify with the site key")
	}
}

func TestWPLoginLinkRefusesUnreadySite(t *testing.T) {
	m, _, _ := newFeatureTest("provisioning")
	m.AutoLoginKey = func(string) string { return strings.Repeat("ab", 16) }
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "provisioning"}
	if _, _, err := m.WPLoginLink(context.Background(), site, time.Now()); !asRefusal(err) {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestImportFileSendsStagedPathAndName(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "ready"}
	if err := m.ImportFile(context.Background(), site, "wp-content", "photo.jpg", UploadDir+"/upload-1"); err != nil {
		t.Fatal(err)
	}
	got := h.args["files.import"]
	if len(got) != 1 || got[0]["name"] != "photo.jpg" || got[0]["path"] != "wp-content" || got[0]["staged"] != UploadDir+"/upload-1" {
		t.Fatalf("files.import args = %v", got)
	}
}
