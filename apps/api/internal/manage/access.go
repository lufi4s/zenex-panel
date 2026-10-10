package manage

import (
	"context"
	"errors"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// SetSFTP turns a website's SFTP login on or off. With reset set while enabling, the
// password is rotated: the new password is derived from the next generation, so the old
// one stops working. The website's files are never touched.
func (m *Manager) SetSFTP(ctx context.Context, site store.Site, enable, reset bool) error {
	if site.State != "ready" {
		return refuse("SFTP access can only be changed for a ready website (it is %s)", site.State)
	}
	if !enable {
		if _, err := m.Helper.Do(ctx, "sftp.disable", map[string]string{"user": site.LinuxUser}); err != nil {
			return err
		}
		return m.Store.SetSiteSFTP(ctx, site.ID, false, site.SFTPGeneration)
	}
	if m.SFTPPasswordFor == nil {
		return errors.New("SFTP password derivation is not configured")
	}
	gen := site.SFTPGeneration
	if reset {
		gen++
	}
	if _, err := m.Helper.Do(ctx, "sftp.enable", map[string]string{
		"user": site.LinuxUser, "password": m.SFTPPasswordFor(site.ID, gen),
	}); err != nil {
		return err
	}
	return m.Store.SetSiteSFTP(ctx, site.ID, true, gen)
}

// PrepareDBAdmin makes phpMyAdmin reachable under the website's own address. The PHP pool
// is rewritten first (it must be able to read phpMyAdmin) and the site file second; both
// are validated by the helper before anything reloads.
func (m *Manager) PrepareDBAdmin(ctx context.Context, site store.Site) error {
	if site.State != "ready" {
		return refuse("phpMyAdmin can only be opened for a ready website (it is %s)", site.State)
	}
	if site.Maintenance {
		return refuse("turn off maintenance mode to open phpMyAdmin")
	}
	if _, err := m.Helper.Do(ctx, "pool.write", map[string]string{
		"user": site.LinuxUser, "php": site.PHPVersion,
	}); err != nil {
		return err
	}
	_, err := m.Helper.Do(ctx, "vhost.write", map[string]string{
		"user": site.LinuxUser, "domain": site.Domain, "maintenance": "0",
	})
	return err
}
