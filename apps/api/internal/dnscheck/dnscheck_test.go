package dnscheck

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeDNS maps host names to answers. Unknown names return NXDOMAIN-like errors.
func fakeDNS(records map[string][]string) Lookup {
	return func(_ context.Context, host string) ([]string, error) {
		if ips, ok := records[host]; ok {
			return ips, nil
		}
		return nil, errors.New("no such host")
	}
}

func TestWildcardPointingHereIsVerified(t *testing.T) {
	c := &Checker{ServerIP: "162.4.35.76", Lookup: fakeDNS(map[string][]string{})}
	c.Lookup = func(_ context.Context, host string) ([]string, error) {
		if strings.HasPrefix(host, "zenex-check-") && strings.HasSuffix(host, ".example.com") {
			return []string{"162.4.35.76"}, nil
		}
		return nil, errors.New("no such host")
	}
	r := c.CheckDomain(context.Background(), "example.com")
	if !r.Verified {
		t.Fatalf("wildcard to this server not verified: %s", r.Message)
	}
}

func TestApexOnlyIsNotEnoughForWebsites(t *testing.T) {
	c := &Checker{ServerIP: "162.4.35.76", Lookup: fakeDNS(map[string][]string{
		"example.com": {"162.4.35.76"},
	})}
	r := c.CheckDomain(context.Background(), "example.com")
	if r.Verified {
		t.Fatal("apex alone must not verify: websites need the wildcard")
	}
	if !strings.Contains(r.Message, "wildcard") {
		t.Fatalf("message should ask for a wildcard record: %s", r.Message)
	}
}

func TestDomainPointingElsewhereExplainsFix(t *testing.T) {
	c := &Checker{ServerIP: "162.4.35.76", Lookup: fakeDNS(map[string][]string{
		"example.com": {"203.0.113.9"},
	})}
	r := c.CheckDomain(context.Background(), "example.com")
	if r.Verified {
		t.Fatal("foreign IP verified")
	}
	for _, want := range []string{"203.0.113.9", "162.4.35.76", "grey cloud"} {
		if !strings.Contains(r.Message, want) {
			t.Errorf("message missing %q: %s", want, r.Message)
		}
	}
}

func TestMissingServerIPFailsClosed(t *testing.T) {
	c := &Checker{ServerIP: "", Lookup: fakeDNS(nil)}
	if r := c.CheckDomain(context.Background(), "example.com"); r.Verified {
		t.Fatal("verified without knowing the server IP")
	}
	if ok, _ := c.CheckHost(context.Background(), "shop.example.com"); ok {
		t.Fatal("host verified without knowing the server IP")
	}
}

func TestCheckHostMatchesExactName(t *testing.T) {
	c := &Checker{ServerIP: "162.4.35.76", Lookup: fakeDNS(map[string][]string{
		"shop.example.com": {"162.4.35.76"},
		"blog.example.com": {"203.0.113.9"},
	})}
	if ok, msg := c.CheckHost(context.Background(), "shop.example.com"); !ok {
		t.Fatalf("matching host rejected: %s", msg)
	}
	if ok, msg := c.CheckHost(context.Background(), "blog.example.com"); ok || !strings.Contains(msg, "203.0.113.9") {
		t.Fatalf("foreign host accepted or message unclear: ok=%v msg=%s", ok, msg)
	}
	if ok, msg := c.CheckHost(context.Background(), "new.example.com"); ok || !strings.Contains(msg, "does not resolve") {
		t.Fatalf("unresolved host: ok=%v msg=%s", ok, msg)
	}
}
