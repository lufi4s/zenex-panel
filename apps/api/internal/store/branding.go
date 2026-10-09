package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
)

// Branding is the name, tagline and accent colour shown across the panel.
type Branding struct {
	Name         string `json:"name"`
	Tagline      string `json:"tagline"`
	PrimaryColor string `json:"primary_color"`
}

// DefaultBranding is used until an administrator changes it.
func DefaultBranding() Branding {
	return Branding{
		Name:         "Zenex Panel",
		Tagline:      "Manage your WordPress websites",
		PrimaryColor: "#3b6fd4",
	}
}

// GetBranding returns the saved branding, or the default when none is saved.
func (s *Store) GetBranding(ctx context.Context) (Branding, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value::text FROM system_settings WHERE key = 'branding'`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultBranding(), nil
	}
	if err != nil {
		return Branding{}, err
	}
	b := DefaultBranding()
	if err := json.Unmarshal(raw, &b); err != nil {
		return DefaultBranding(), nil
	}
	return b, nil
}

// SetBranding saves the branding and records who changed it.
func (s *Store) SetBranding(ctx context.Context, userID string, b Branding) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ('branding', $1::jsonb, NULLIF($2, '')::uuid, now())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		string(raw), userID)
	return err
}

// brandingAssetsKey is the system_settings key for the uploaded logo and favicon.
const brandingAssetsKey = "branding_assets"

// BrandingAsset is an uploaded image. Data is standard base64 of the file bytes.
type BrandingAsset struct {
	Mime string `json:"mime"`
	Data string `json:"data"`
}

// BrandingAssets holds the logo and favicon. Either may be absent.
type BrandingAssets struct {
	Logo    *BrandingAsset `json:"logo,omitempty"`
	Favicon *BrandingAsset `json:"favicon,omitempty"`
}

// brandingAssetsMu serialises read-modify-write of the assets document, so saving the
// logo and the favicon at the same time cannot drop one of them.
var brandingAssetsMu sync.Mutex

// GetBrandingAssets returns the stored images. An empty value means none are set.
func (s *Store) GetBrandingAssets(ctx context.Context) (BrandingAssets, error) {
	var a BrandingAssets
	raw, err := s.GetSetting(ctx, brandingAssetsKey)
	if errors.Is(err, ErrNotFound) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	return a, json.Unmarshal(raw, &a)
}

// UpdateBrandingAssets applies change to the stored images and records who changed them.
func (s *Store) UpdateBrandingAssets(ctx context.Context, userID string, change func(*BrandingAssets)) error {
	brandingAssetsMu.Lock()
	defer brandingAssetsMu.Unlock()
	a, err := s.GetBrandingAssets(ctx)
	if err != nil {
		return err
	}
	change(&a)
	return s.PutSetting(ctx, brandingAssetsKey, a, userID)
}
