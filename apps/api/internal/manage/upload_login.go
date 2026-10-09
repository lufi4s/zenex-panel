package manage

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const (
	// UploadDir holds uploads while they wait to be moved into a website. Only the panel
	// account and the helper can read it. It must match the helper's upload folder.
	UploadDir = "/var/lib/zenex/uploads"
	// WPAdminLogin is the WordPress administrator every website is installed with.
	WPAdminLogin = "zenexadmin"
	// loginLinkTTL is how long an "Open admin" link works. It is also single-use.
	loginLinkTTL = 90 * time.Second
)

// ImportFile moves a file that was uploaded to a folder of the website. The file must already
// be staged in UploadDir. An existing file with the same name is not replaced.
func (m *Manager) ImportFile(ctx context.Context, site store.Site, dir, name, staged string) error {
	if site.State != "ready" && site.State != "suspended" {
		return refuse("files can be managed once the website is ready (it is %s)", site.State)
	}
	_, err := m.Helper.Do(ctx, "files.import", map[string]string{
		"user": site.LinuxUser, "path": dir, "name": name, "staged": staged,
	})
	return err
}

// WPLoginLink prepares the sign-in plugin of a website and returns a one-time link that signs
// the administrator in. The link is "<expiry>.<nonce>.<hmac>", checked by the plugin with the
// same key. It is valid for loginLinkTTL and works once.
func (m *Manager) WPLoginLink(ctx context.Context, site store.Site, now time.Time) (string, time.Time, error) {
	if site.State != "ready" {
		return "", time.Time{}, refuse("the admin dashboard can be opened once the website is ready (it is %s)", site.State)
	}
	if m.AutoLoginKey == nil {
		return "", time.Time{}, errors.New("sign-in links cannot be made on this server")
	}
	key := m.AutoLoginKey(site.ID)
	if _, err := m.Helper.Do(ctx, "wp.autologin", map[string]string{
		"user": site.LinuxUser, "key": key, "login": WPAdminLogin,
	}); err != nil {
		return "", time.Time{}, err
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", time.Time{}, errors.New("a sign-in link could not be made")
	}
	expires := now.Add(loginLinkTTL)
	payload := strconv.FormatInt(expires.Unix(), 10) + "." + hex.EncodeToString(nonce)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	link := "https://" + site.Domain + "/?zenex_autologin=" + payload + "." + hex.EncodeToString(mac.Sum(nil))
	return link, expires, nil
}
