// Package dnscheck confirms that a customer's DNS points at this server before
// any website is built. A website is only useful if its name resolves here.
package dnscheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"
)

// Lookup resolves a host name to its IP addresses.
type Lookup func(ctx context.Context, host string) ([]string, error)

// Checker compares DNS answers with this server's public IP.
type Checker struct {
	ServerIP string
	Lookup   Lookup
	Timeout  time.Duration
}

// New returns a checker that uses the system resolver.
func New(serverIP string) *Checker {
	return &Checker{
		ServerIP: serverIP,
		Timeout:  6 * time.Second,
		Lookup: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
	}
}

// Result is the outcome of a domain check.
type Result struct {
	// Verified is true only when a wildcard record points at this server, so every
	// website name under the domain will resolve here.
	Verified bool
	Message  string
}

// CheckDomain verifies a connected apex domain. It queries a random name under the
// domain: if that resolves to this server, the wildcard record is in place.
func (c *Checker) CheckDomain(ctx context.Context, apex string) Result {
	if c.ServerIP == "" {
		return Result{Message: "this server's public IP is not configured on the panel; ask the administrator"}
	}
	probe, err := randomLabel()
	if err != nil {
		return Result{Message: "could not run the DNS check; try again"}
	}
	wildcardIPs := c.resolve(ctx, probe+"."+apex)
	if slices.Contains(wildcardIPs, c.ServerIP) {
		return Result{Verified: true, Message: fmt.Sprintf("DNS verified: *.%s points to this server (%s)", apex, c.ServerIP)}
	}

	apexIPs := c.resolve(ctx, apex)
	if len(apexIPs) > 0 && !slices.Contains(apexIPs, c.ServerIP) {
		return Result{Message: fmt.Sprintf(
			"%s points to %s, not to this server (%s). %s",
			apex, strings.Join(apexIPs, ", "), c.ServerIP, recordHelp(apex, c.ServerIP))}
	}
	return Result{Message: fmt.Sprintf("no wildcard DNS record found for *.%s. %s", apex, recordHelp(apex, c.ServerIP))}
}

// CheckHost confirms that one website name resolves to this server.
func (c *Checker) CheckHost(ctx context.Context, fqdn string) (bool, string) {
	if c.ServerIP == "" {
		return false, "this server's public IP is not configured on the panel"
	}
	ips := c.resolve(ctx, fqdn)
	if slices.Contains(ips, c.ServerIP) {
		return true, ""
	}
	if len(ips) == 0 {
		return false, fmt.Sprintf("%s does not resolve yet. Add a DNS record for *.%s pointing to %s, then try again. DNS changes can take a few minutes.",
			fqdn, parentOf(fqdn), c.ServerIP)
	}
	return false, fmt.Sprintf("%s points to %s, not to this server (%s). Update the DNS record, then try again.",
		fqdn, strings.Join(ips, ", "), c.ServerIP)
}

func (c *Checker) resolve(ctx context.Context, host string) []string {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	ips, err := c.Lookup(ctx, host)
	if err != nil {
		return nil
	}
	return ips
}

func (c *Checker) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 6 * time.Second
}

// recordHelp tells the customer exactly what to add. Cloudflare's proxy hides the
// real IP, so it is called out explicitly.
func recordHelp(apex, ip string) string {
	return fmt.Sprintf("Add a DNS record: type A, name *, value %s. Also add an A record for @ (or %s) pointing to %s. If you use Cloudflare, set the record to DNS only (grey cloud).",
		ip, apex, ip)
}

func parentOf(fqdn string) string {
	if i := strings.Index(fqdn, "."); i >= 0 {
		return fqdn[i+1:]
	}
	return fqdn
}

func randomLabel() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "zenex-check-" + hex.EncodeToString(b), nil
}
