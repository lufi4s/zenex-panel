package manage

import (
	"context"
	"encoding/json"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/mail"
)

// MailServerSettings are the fixed mail server settings. The host name is chosen in the panel
// and passed to ApplyMailServer. DBPass is the password of the read-only mail database role.
type MailServerSettings struct {
	DBHost string
	DBName string
	DBUser string
	DBPass string
	MaxMB  int
}

// mailFilePayload is one configuration file as the helper expects it.
type mailFilePayload struct {
	Path    string `json:"path"`
	Mode    uint32 `json:"mode"`
	Content string `json:"content"`
}

// ApplyMailServer sets the mail server up for host: the helper first prepares the certificate
// and the firewall ports, makes sure every mail domain's DKIM key is where rspamd can read it,
// writes, checks and loads the configuration, and finally publishes the webmail under the host
// name. If a check fails the helper restores the old files.
func (m *Manager) ApplyMailServer(ctx context.Context, host string, domains []string) error {
	if !mail.ValidHostname(host) {
		return refuse("the mail host must be a full host name such as mail.example.com (lower case)")
	}
	s := m.MailServer
	files, err := mail.Render(mail.Settings{
		Host: host, DBHost: s.DBHost, DBName: s.DBName, DBUser: s.DBUser, DBPass: s.DBPass, MaxMB: s.MaxMB,
	})
	if err != nil {
		return refuse("%s", err.Error())
	}
	payload := make([]mailFilePayload, 0, len(files))
	for _, f := range files {
		payload = append(payload, mailFilePayload{Path: f.Path, Mode: f.Mode, Content: f.Content})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := m.Helper.Do(ctx, "mail.prepare", map[string]string{"host": host}); err != nil {
		return err
	}
	for _, domain := range domains {
		if _, err := m.MailDKIMValue(ctx, domain); err != nil {
			return err
		}
	}
	if _, err := m.Helper.Do(ctx, "mail.apply", map[string]string{"files": string(raw), "dbpass": s.DBPass}); err != nil {
		return err
	}
	// Publish the webmail under the mail host name: Caddy fetches its HTTPS certificate.
	_, err = m.Helper.Do(ctx, "webmail.setup", map[string]string{"host": host})
	return err
}

// MailDKIMValue returns the public DKIM value for a domain, creating its key the first time.
// The key does not need the mail server to be set up, so mail can be turned on before it is.
func (m *Manager) MailDKIMValue(ctx context.Context, apex string) (string, error) {
	out, err := m.Helper.Output(ctx, "mail.dkim-key", map[string]string{"domain": apex})
	if err != nil {
		return "", err
	}
	return out, nil
}
