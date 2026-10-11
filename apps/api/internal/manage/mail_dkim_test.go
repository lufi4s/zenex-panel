package manage

import (
	"context"
	"testing"
)

// Turning mail on must create the DKIM key even before the mail server is installed.
func TestDKIMKeyDoesNotNeedTheMailServer(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.outputs["mail.dkim-key"] = "p=QUJD"
	value, err := m.MailDKIMValue(context.Background(), "ozima.cloud")
	if err != nil {
		t.Fatalf("DKIM without a mail server = %v", err)
	}
	if value != "p=QUJD" {
		t.Fatalf("value = %q", value)
	}
	if got := h.args["mail.dkim-key"][0]["domain"]; got != "ozima.cloud" {
		t.Fatalf("domain sent = %q", got)
	}
}
