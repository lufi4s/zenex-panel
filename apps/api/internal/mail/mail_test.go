package mail

import (
	"strings"
	"testing"
)

func TestValidLocalPart(t *testing.T) {
	for _, ok := range []string{"info", "sales.team", "a-b_c", "x1"} {
		if !ValidLocalPart(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "Info", "-start", "has space", "a..b", "a@b", "é", strings.Repeat("a", 65)} {
		if ValidLocalPart(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidPassword(t *testing.T) {
	if err := ValidPassword("correct horse 1"); err != nil {
		t.Fatal(err)
	}
	if ValidPassword("short") == nil {
		t.Error("short password accepted")
	}
	if ValidPassword("twelve chars\n") == nil {
		t.Error("control character accepted")
	}
	if ValidPassword(strings.Repeat("a", MaxPasswordLen+1)) == nil {
		t.Error("too long password accepted")
	}
}

func TestValidQuota(t *testing.T) {
	if !ValidQuota(DefaultQuotaMB) || ValidQuota(MinQuotaMB-1) || ValidQuota(MaxQuotaMB+1) {
		t.Fatal("quota bounds wrong")
	}
}

func TestDNSRecordsForAMailDomain(t *testing.T) {
	recs := DNSRecords("ozima.cloud", "162.4.35.76")
	want := map[string]string{
		"MX":       "10 mail.ozima.cloud.",
		"A":        "162.4.35.76",
		"_dmarc":   "v=DMARC1; p=none; rua=mailto:postmaster@ozima.cloud",
		"@ v=spf1": "v=spf1 mx ~all",
	}
	got := map[string]string{}
	for _, r := range recs {
		if r.Type == "MX" || r.Type == "A" {
			got[r.Type] = r.Value
		}
		if r.Name == "_dmarc" {
			got["_dmarc"] = r.Value
		}
		if r.Type == "TXT" && r.Name == "@" {
			got["@ v=spf1"] = r.Value
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if !strings.Contains(DNSRecords("ozima.cloud", "")[0].Value, "this server") {
		t.Error("unknown server IP not explained")
	}
}
