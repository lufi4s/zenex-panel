// Package mail holds the rules for mailboxes that do not need the database or the mail
// server: which addresses and passwords are allowed, and which DNS records a domain needs.
package mail

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// localPartRe is the part before the @. Lower case only, so two spellings never clash.
var localPartRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
	MinQuotaMB     = 10
	MaxQuotaMB     = 102400
	DefaultQuotaMB = 1024
)

// ValidLocalPart reports whether s is an allowed mailbox name (the part before the @).
func ValidLocalPart(s string) bool {
	return localPartRe.MatchString(s) && !strings.Contains(s, "..")
}

// ValidPassword checks the length and the characters of a new mailbox password.
func ValidPassword(s string) error {
	if len(s) < MinPasswordLen || len(s) > MaxPasswordLen {
		return errors.New("the password must be between 12 and 128 characters")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return errors.New("the password cannot contain control characters")
		}
	}
	return nil
}

// ValidQuota checks a mailbox quota in megabytes.
func ValidQuota(mb int) bool { return mb >= MinQuotaMB && mb <= MaxQuotaMB }

// Address joins a mailbox name and its domain.
func Address(localPart, apex string) string { return localPart + "@" + apex }

// Record is one DNS record the customer adds at their DNS provider.
type Record struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Note  string `json:"note"`
}

// MailHost is the name of the mail server for a domain.
func MailHost(apex string) string { return "mail." + apex }

// DNSRecords returns the records a domain needs before its mail works. serverIP may be empty
// when the server does not know its address yet; the A record then says so.
// The DKIM key is created when the mail server is installed (phase 2), so its value is empty.
func DNSRecords(apex, serverIP string) []Record {
	host := MailHost(apex)
	aValue, aNote := serverIP, "Points the mail host name at this server."
	if serverIP == "" {
		aValue, aNote = "(this server's IP)", "The panel does not know this server's IP yet. Use the address shown on the Domains page."
	}
	return []Record{
		{Type: "A", Name: "mail", Value: aValue, Note: aNote},
		{Type: "MX", Name: "@", Value: "10 " + host + ".", Note: "Sends incoming mail for the domain to the mail host."},
		{Type: "TXT", Name: "@", Value: "v=spf1 mx ~all", Note: "Lists this server as allowed to send mail for the domain."},
		{Type: "TXT", Name: "_dmarc", Value: "v=DMARC1; p=none; rua=mailto:postmaster@" + apex, Note: "Tells receivers what to do with unsigned mail. Start with p=none."},
		{Type: "TXT", Name: "default._domainkey", Value: "", Note: "Added once the mail server is installed. Signs outgoing mail."},
	}
}
