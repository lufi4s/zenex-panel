package mail

import (
	"context"
	"errors"
	"net"
	"testing"
)

// fakeDNS answers lookups from maps. Names not in a map are "not found", as real DNS says.
type fakeDNS struct {
	mx   map[string][]*net.MX
	txt  map[string][]string
	host map[string][]string
	fail bool
}

func (f *fakeDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if f.fail {
		return nil, errors.New("server failure")
	}
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Name: name, IsNotFound: true}
}

func (f *fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if f.fail {
		return nil, errors.New("server failure")
	}
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Name: name, IsNotFound: true}
}

func (f *fakeDNS) LookupHost(_ context.Context, name string) ([]string, error) {
	if f.fail {
		return nil, errors.New("server failure")
	}
	if v, ok := f.host[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Name: name, IsNotFound: true}
}

func byType(results []CheckResult, typ, name string) CheckResult {
	for _, r := range results {
		if r.Type == typ && r.Name == name {
			return r
		}
	}
	return CheckResult{}
}

func goodDNS(dkim string) *fakeDNS {
	return &fakeDNS{
		mx:   map[string][]*net.MX{"ozima.cloud": {{Host: "mail.ozima.cloud.", Pref: 10}}},
		host: map[string][]string{"mail.ozima.cloud": {"162.4.35.76"}},
		txt: map[string][]string{
			"ozima.cloud":                    {"google-site-verification=abc", "v=spf1 mx ~all"},
			"_dmarc.ozima.cloud":             {"v=DMARC1; p=none; rua=mailto:postmaster@ozima.cloud"},
			"default._domainkey.ozima.cloud": {"v=DKIM1; k=rsa; " + dkim},
		},
	}
}

func TestCheckPassesWhenEveryRecordMatches(t *testing.T) {
	res := CheckRecords(context.Background(), goodDNS("p=QUJD"), "ozima.cloud", "162.4.35.76", "p=QUJD")
	for _, r := range res {
		if r.Status != StatusOK {
			t.Errorf("%s %s = %s (%s), want ok", r.Type, r.Name, r.Status, r.Message)
		}
	}
}

func TestCheckReportsMissingRecords(t *testing.T) {
	res := CheckRecords(context.Background(), &fakeDNS{}, "ozima.cloud", "162.4.35.76", "p=QUJD")
	for _, want := range []struct{ typ, name string }{{"MX", "@"}, {"A", "mail"}, {"TXT", "@"}, {"TXT", "_dmarc"}, {"TXT", "default._domainkey"}} {
		if got := byType(res, want.typ, want.name); got.Status != StatusMissing {
			t.Errorf("%s %s = %s, want missing", want.typ, want.name, got.Status)
		}
	}
}

func TestCheckCatchesWrongTargets(t *testing.T) {
	d := goodDNS("p=QUJD")
	d.mx["ozima.cloud"] = []*net.MX{{Host: "mx.someone-else.example.", Pref: 10}}
	d.host["mail.ozima.cloud"] = []string{"203.0.113.9"}
	d.txt["ozima.cloud"] = []string{"v=spf1 include:_spf.google.com ~all"}
	res := CheckRecords(context.Background(), d, "ozima.cloud", "162.4.35.76", "p=QUJD")
	for _, want := range []struct{ typ, name string }{{"MX", "@"}, {"A", "mail"}, {"TXT", "@"}} {
		if got := byType(res, want.typ, want.name); got.Status != StatusDifferent {
			t.Errorf("%s %s = %s, want different", want.typ, want.name, got.Status)
		}
	}
}

func TestCheckDKIMNeedsTheWholeRecord(t *testing.T) {
	d := goodDNS("")
	d.txt["default._domainkey.ozima.cloud"] = []string{"v=DKIM1; k=rsa; p=QUJD", "REVG"}
	res := CheckRecords(context.Background(), d, "ozima.cloud", "162.4.35.76", "p=QUJDREVG")
	if got := byType(res, "TXT", "default._domainkey"); got.Status != StatusDifferent {
		t.Errorf("first split part alone = %s, want different", got.Status)
	}
	d.txt["default._domainkey.ozima.cloud"] = []string{"v=DKIM1; k=rsa; p=QUJDREVG"}
	res = CheckRecords(context.Background(), d, "ozima.cloud", "162.4.35.76", "p=QUJDREVG")
	if got := byType(res, "TXT", "default._domainkey"); got.Status != StatusOK {
		t.Errorf("DKIM = %s, want ok", got.Status)
	}
}

func TestCheckDKIMPendingBeforeTheKeyExists(t *testing.T) {
	res := CheckRecords(context.Background(), goodDNS(""), "ozima.cloud", "162.4.35.76", "")
	if got := byType(res, "TXT", "default._domainkey"); got.Status != StatusPending {
		t.Errorf("DKIM without a key = %s, want pending", got.Status)
	}
}

func TestCheckSPFNeedsMXAndOneRecord(t *testing.T) {
	d := goodDNS("p=QUJD")
	d.txt["ozima.cloud"] = []string{"v=spf1 mx ~all", "v=spf1 -all"}
	if got := byType(CheckRecords(context.Background(), d, "ozima.cloud", "162.4.35.76", "p=QUJD"), "TXT", "@"); got.Status != StatusDifferent {
		t.Errorf("two SPF records = %s, want different", got.Status)
	}
}

func TestCheckReportsLookupFailuresAsErrors(t *testing.T) {
	res := CheckRecords(context.Background(), &fakeDNS{fail: true}, "ozima.cloud", "162.4.35.76", "p=QUJD")
	for _, r := range res {
		if r.Status == StatusOK {
			t.Errorf("%s %s passed although the lookup failed", r.Type, r.Name)
		}
	}
	if got := byType(res, "MX", "@"); got.Status != StatusError {
		t.Errorf("MX on failure = %s, want error", got.Status)
	}
}

func TestCheckServerIPUnknownIsPending(t *testing.T) {
	res := CheckRecords(context.Background(), goodDNS("p=QUJD"), "ozima.cloud", "", "p=QUJD")
	if got := byType(res, "A", "mail"); got.Status != StatusPending {
		t.Errorf("A with unknown server IP = %s, want pending", got.Status)
	}
}
