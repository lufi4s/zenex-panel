// Package validation holds input validators shared by the API and the VPS Agent.
// Every value that ends up in a filesystem path, a Linux username, a DNS record
// or a command argument must pass through here first.
package validation

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidSiteSlug       = errors.New("invalid site slug")
	ErrReservedSiteSlug      = errors.New("site slug is reserved")
	ErrInvalidDomain         = errors.New("invalid domain name")
	ErrInvalidLinuxUser      = errors.New("invalid linux user name")
	ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")
)

var (
	siteSlugRe       = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)
	dnsLabelRe       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	linuxUserRe      = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	idempotencyKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)
)

// reservedSlugs cannot be used as a site label because they collide with
// infrastructure hostnames under the customer's apex domain.
var reservedSlugs = map[string]struct{}{
	"www": {}, "mail": {}, "smtp": {}, "imap": {}, "pop": {}, "ftp": {},
	"ns1": {}, "ns2": {}, "api": {}, "panel": {}, "admin": {}, "agent": {},
	"zenex": {}, "root": {}, "autoconfig": {}, "autodiscover": {},
}

// SiteSlug validates the single-label subdomain a customer types (e.g. "shop").
// 3 to 32 characters, lowercase letters, digits and hyphens, starting with a letter.
func SiteSlug(s string) error {
	if !siteSlugRe.MatchString(s) || strings.Contains(s, "--") {
		return ErrInvalidSiteSlug
	}
	if _, reserved := reservedSlugs[s]; reserved {
		return ErrReservedSiteSlug
	}
	return nil
}

// DomainName validates a fully qualified, lowercase DNS name without a trailing dot.
func DomainName(s string) error {
	if len(s) == 0 || len(s) > 253 {
		return ErrInvalidDomain
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return ErrInvalidDomain
	}
	for _, label := range labels {
		if !dnsLabelRe.MatchString(label) {
			return ErrInvalidDomain
		}
	}
	// A TLD must not be purely numeric, otherwise "1.2.3.4" would pass.
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return ErrInvalidDomain
	}
	return nil
}

// ComposeSiteDomain turns a customer-entered label and the connected apex
// domain into the site's FQDN, e.g. ("shop", "example.com") -> "shop.example.com".
func ComposeSiteDomain(slug, apex string) (string, error) {
	if err := SiteSlug(slug); err != nil {
		return "", err
	}
	if err := DomainName(apex); err != nil {
		return "", err
	}
	return slug + "." + apex, nil
}

// LinuxUser validates a per-site system account name.
func LinuxUser(s string) error {
	if !linuxUserRe.MatchString(s) {
		return ErrInvalidLinuxUser
	}
	return nil
}

// IdempotencyKey validates a client-supplied idempotency key.
func IdempotencyKey(s string) error {
	if !idempotencyKeyRe.MatchString(s) {
		return ErrInvalidIdempotencyKey
	}
	return nil
}
