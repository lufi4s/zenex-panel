package validation

import (
	"errors"
	"testing"
)

func TestSiteSlug(t *testing.T) {
	cases := []struct {
		in      string
		wantErr error
	}{
		{"shop", nil},
		{"my-shop-2", nil},
		{"abc", nil},
		{"sh", ErrInvalidSiteSlug},
		{"2shop", ErrInvalidSiteSlug},
		{"shop-", ErrInvalidSiteSlug},
		{"sh--op", ErrInvalidSiteSlug},
		{"Shop", ErrInvalidSiteSlug},
		{"shop.evil", ErrInvalidSiteSlug},
		{"../etc", ErrInvalidSiteSlug},
		{"shop;rm", ErrInvalidSiteSlug},
		{"", ErrInvalidSiteSlug},
		{"www", ErrReservedSiteSlug},
		{"panel", ErrReservedSiteSlug},
		{"mail", ErrReservedSiteSlug},
	}
	for _, c := range cases {
		if err := SiteSlug(c.in); !errors.Is(err, c.wantErr) {
			t.Errorf("SiteSlug(%q) = %v, want %v", c.in, err, c.wantErr)
		}
	}
}

func TestDomainName(t *testing.T) {
	valid := []string{"example.com", "shop.example.co.uk", "a-b.example.org"}
	invalid := []string{"", "example", "Example.com", "example..com", "-bad.com",
		"bad-.com", "1.2.3.4", "exa mple.com", "example.com.", "a.b/c.com"}

	for _, d := range valid {
		if err := DomainName(d); err != nil {
			t.Errorf("DomainName(%q) unexpected error: %v", d, err)
		}
	}
	for _, d := range invalid {
		if err := DomainName(d); !errors.Is(err, ErrInvalidDomain) {
			t.Errorf("DomainName(%q) = %v, want ErrInvalidDomain", d, err)
		}
	}
}

func TestComposeSiteDomain(t *testing.T) {
	got, err := ComposeSiteDomain("shop", "example.com")
	if err != nil || got != "shop.example.com" {
		t.Fatalf("ComposeSiteDomain = %q, %v; want shop.example.com, nil", got, err)
	}
	if _, err := ComposeSiteDomain("www", "example.com"); !errors.Is(err, ErrReservedSiteSlug) {
		t.Errorf("reserved slug not rejected: %v", err)
	}
	if _, err := ComposeSiteDomain("shop", "bad_domain"); !errors.Is(err, ErrInvalidDomain) {
		t.Errorf("invalid apex not rejected: %v", err)
	}
}

func TestLinuxUser(t *testing.T) {
	if err := LinuxUser("zx_shop-1"); err != nil {
		t.Errorf("valid user rejected: %v", err)
	}
	for _, u := range []string{"", "1user", "Root", "a b", "x;y", "toolongusernamethatexceedsthirtytwochars"} {
		if err := LinuxUser(u); !errors.Is(err, ErrInvalidLinuxUser) {
			t.Errorf("LinuxUser(%q) = %v, want ErrInvalidLinuxUser", u, err)
		}
	}
}

func TestIdempotencyKey(t *testing.T) {
	if err := IdempotencyKey("create-shop-2026-10-08"); err != nil {
		t.Errorf("valid key rejected: %v", err)
	}
	for _, k := range []string{"short", "has space in it", "semi;colon-key"} {
		if err := IdempotencyKey(k); !errors.Is(err, ErrInvalidIdempotencyKey) {
			t.Errorf("IdempotencyKey(%q) = %v, want ErrInvalidIdempotencyKey", k, err)
		}
	}
}
