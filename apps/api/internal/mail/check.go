package mail

import (
	"context"
	"errors"
	"net"
	"strings"
)

// Resolver is the part of DNS lookups the check needs. *net.Resolver satisfies it.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Check statuses.
const (
	StatusOK        = "ok"        // the record is published and correct
	StatusMissing   = "missing"   // nothing is published under that name
	StatusDifferent = "different" // something is published, but not what mail needs
	StatusError     = "error"     // the lookup failed (network or DNS server)
	StatusPending   = "pending"   // cannot be checked yet (for example the DKIM key does not exist)
)

// CheckResult is the outcome of checking one required record.
type CheckResult struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Found   []string `json:"found"`
	Message string   `json:"message"`
}

// CheckRecords looks up every record in DNSRecords and reports what is published. DNS changes
// can take hours to spread, so a missing record is a hint, not a verdict.
func CheckRecords(ctx context.Context, r Resolver, apex, serverIP, dkimValue string) []CheckResult {
	host := MailHost(apex)
	var out []CheckResult

	out = append(out, checkMX(ctx, r, apex, host))
	out = append(out, checkA(ctx, r, host, serverIP))
	out = append(out, checkSPF(ctx, r, apex))
	out = append(out, checkDMARC(ctx, r, apex))
	out = append(out, checkDKIM(ctx, r, apex, dkimValue))
	return out
}

func notFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func trimDot(s string) string { return strings.ToLower(strings.TrimSuffix(s, ".")) }

func checkMX(ctx context.Context, r Resolver, apex, host string) CheckResult {
	res := CheckResult{Type: "MX", Name: "@"}
	mxs, err := r.LookupMX(ctx, apex)
	if err != nil && !notFound(err) {
		res.Status, res.Message = StatusError, "The MX lookup failed. Try again in a few minutes."
		return res
	}
	for _, mx := range mxs {
		res.Found = append(res.Found, trimDot(mx.Host))
	}
	switch {
	case len(res.Found) == 0:
		res.Status, res.Message = StatusMissing, "No MX record. Add it at your DNS provider."
	case containsName(res.Found, host):
		res.Status, res.Message = StatusOK, "Mail for the domain is delivered to the mail host."
	default:
		res.Status, res.Message = StatusDifferent, "The MX record points somewhere else. It must point to "+host+"."
	}
	return res
}

func checkA(ctx context.Context, r Resolver, host, serverIP string) CheckResult {
	res := CheckResult{Type: "A", Name: "mail"}
	addrs, err := r.LookupHost(ctx, host)
	if err != nil && !notFound(err) {
		res.Status, res.Message = StatusError, "The lookup of the mail host failed. Try again in a few minutes."
		return res
	}
	res.Found = addrs
	switch {
	case len(addrs) == 0:
		res.Status, res.Message = StatusMissing, "The mail host name does not resolve yet."
	case serverIP == "":
		res.Status, res.Message = StatusPending, "The panel does not know this server's IP, so the address cannot be compared."
	case containsName(addrs, serverIP):
		res.Status, res.Message = StatusOK, "The mail host points at this server."
	default:
		res.Status, res.Message = StatusDifferent, "The mail host points at another address. It must point to "+serverIP+"."
	}
	return res
}

func checkSPF(ctx context.Context, r Resolver, apex string) CheckResult {
	res := CheckResult{Type: "TXT", Name: "@"}
	txts, err := r.LookupTXT(ctx, apex)
	if err != nil && !notFound(err) {
		res.Status, res.Message = StatusError, "The SPF lookup failed. Try again in a few minutes."
		return res
	}
	var spf []string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(t), "v=spf1") {
			spf = append(spf, t)
		}
	}
	res.Found = spf
	switch {
	case len(spf) == 0:
		res.Status, res.Message = StatusMissing, "No SPF record. Add a TXT record starting with v=spf1."
	case len(spf) > 1:
		res.Status, res.Message = StatusDifferent, "More than one SPF record. Keep only one."
	case !hasMechanism(spf[0], "mx"):
		res.Status, res.Message = StatusDifferent, "The SPF record does not allow the mail host. It must include mx."
	default:
		res.Status, res.Message = StatusOK, "SPF allows this server to send mail for the domain."
	}
	return res
}

func checkDMARC(ctx context.Context, r Resolver, apex string) CheckResult {
	res := CheckResult{Type: "TXT", Name: "_dmarc"}
	txts, err := r.LookupTXT(ctx, "_dmarc."+apex)
	if err != nil && !notFound(err) {
		res.Status, res.Message = StatusError, "The DMARC lookup failed. Try again in a few minutes."
		return res
	}
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			res.Found = append(res.Found, t)
		}
	}
	if len(res.Found) == 0 {
		res.Status, res.Message = StatusMissing, "No DMARC record. Add a TXT record starting with v=DMARC1."
		return res
	}
	res.Status, res.Message = StatusOK, "DMARC is published."
	return res
}

func checkDKIM(ctx context.Context, r Resolver, apex, dkimValue string) CheckResult {
	res := CheckResult{Type: "TXT", Name: dkimSelector + "._domainkey"}
	if dkimValue == "" {
		res.Status, res.Message = StatusPending, "The DKIM key is created when mail is turned on. Turn mail on, then check again."
		return res
	}
	txts, err := r.LookupTXT(ctx, dkimSelector+"._domainkey."+apex)
	if err != nil && !notFound(err) {
		res.Status, res.Message = StatusError, "The DKIM lookup failed. Try again in a few minutes."
		return res
	}
	res.Found = txts
	want := compact("v=DKIM1; k=rsa; " + dkimValue)
	for _, t := range txts {
		// A long key is one TXT record made of several chunks; Go joins the chunks, so only spaces differ.
		if compact(t) == want {
			res.Status, res.Message = StatusOK, "The DKIM key matches."
			return res
		}
	}
	if len(txts) == 0 {
		res.Status, res.Message = StatusMissing, "No DKIM record. Add the value shown above."
		return res
	}
	res.Status, res.Message = StatusDifferent, "The DKIM record does not match the key on this server. Copy the value again."
	return res
}

// compact removes spaces, so two spellings of the same record compare equal.
func compact(s string) string { return strings.Join(strings.Fields(s), "") }

func hasMechanism(spf, mech string) bool {
	for _, f := range strings.Fields(strings.ToLower(spf)) {
		if strings.TrimLeft(f, "+-~?") == mech {
			return true
		}
	}
	return false
}

func containsName(list []string, want string) bool {
	for _, v := range list {
		if trimDot(v) == trimDot(want) {
			return true
		}
	}
	return false
}
