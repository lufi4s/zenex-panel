package manage

import (
	"context"
	"encoding/json"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/mail"
)

// MailServerSettings come from the environment (ZENEX_MAIL_HOST, ZENEX_MAIL_DB_PASSWORD).
// Host empty means the mail server is not set up on this server.
type MailServerSettings struct {
	Host   string
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

// ApplyMailServer renders the mail configuration from the current settings and has the helper
// write, check and reload it. The helper restores the old files if any check fails.
func (m *Manager) ApplyMailServer(ctx context.Context) error {
	s := m.MailServer
	if s.Host == "" {
		return refuse("the mail server is not set up on this server (ZENEX_MAIL_HOST is empty)")
	}
	files, err := mail.Render(mail.Settings{
		Host: s.Host, DBHost: s.DBHost, DBName: s.DBName, DBUser: s.DBUser, DBPass: s.DBPass, MaxMB: s.MaxMB,
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
	_, err = m.Helper.Do(ctx, "mail.apply", map[string]string{"files": string(raw), "dbpass": s.DBPass})
	return err
}

// MailDKIMValue returns the public DKIM value for a domain, creating its key the first time.
// The key does not need the mail server to be installed, so mail can be turned on before it is.
func (m *Manager) MailDKIMValue(ctx context.Context, apex string) (string, error) {
	out, err := m.Helper.Output(ctx, "mail.dkim-key", map[string]string{"domain": apex})
	if err != nil {
		return "", err
	}
	return out, nil
}
