// Package alerts watches host metrics against thresholds and tells administrators
// by email, Telegram and in-panel notification when a metric goes over and when
// it recovers. Passwords and bot tokens are stored encrypted, never in the
// settings document.
package alerts

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
)

// SettingsKey is the system_settings key for the non-secret alert configuration.
const SettingsKey = "alert_settings"

// Secret keys in system_settings. Values are base64(nonce || ciphertext) JSON strings.
const (
	SMTPPasswordKey  = "smtp_password"
	TelegramTokenKey = "telegram_token"
)

const (
	maxSecretLen = 512
	maxChatIDLen = 64
)

// EmailSettings configures SMTP delivery. Password is kept separately.
type EmailSettings struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// TelegramSettings configures Telegram delivery. The bot token is kept separately.
type TelegramSettings struct {
	Enabled bool   `json:"enabled"`
	ChatID  string `json:"chat_id"`
}

// Thresholds are percentages (1-100) above which an alert fires.
type Thresholds struct {
	CPU    int `json:"cpu"`
	Memory int `json:"memory"`
	Disk   int `json:"disk"`
}

// Settings is the stored, non-secret alert configuration.
type Settings struct {
	Email      EmailSettings    `json:"email"`
	Telegram   TelegramSettings `json:"telegram"`
	Thresholds Thresholds       `json:"thresholds"`
}

// DefaultSettings is used until an administrator saves the alert configuration.
func DefaultSettings() Settings {
	return Settings{
		Email:      EmailSettings{Port: 587},
		Thresholds: Thresholds{CPU: 85, Memory: 90, Disk: 90},
	}
}

// EmailInput is an email configuration as sent by the client. Password is write-only.
// PasswordSet is read-only and ignored, so a GET response can be sent back as is.
type EmailInput struct {
	EmailSettings
	Password    string `json:"password"`
	PasswordSet bool   `json:"password_set,omitempty"`
}

// TelegramInput is a Telegram configuration as sent by the client. Token is write-only.
type TelegramInput struct {
	TelegramSettings
	Token    string `json:"token"`
	TokenSet bool   `json:"token_set,omitempty"`
}

// Input is the body of PUT /api/v1/settings/alerts.
type Input struct {
	Email      EmailInput    `json:"email"`
	Telegram   TelegramInput `json:"telegram"`
	Thresholds Thresholds    `json:"thresholds"`
}

// EmailView is the email configuration shown to administrators. It never includes the password.
type EmailView struct {
	EmailSettings
	PasswordSet bool `json:"password_set"`
}

// TelegramView is the Telegram configuration shown to administrators. It never includes the token.
type TelegramView struct {
	TelegramSettings
	TokenSet bool `json:"token_set"`
}

// View is the response of GET /api/v1/settings/alerts.
type View struct {
	Email      EmailView    `json:"email"`
	Telegram   TelegramView `json:"telegram"`
	Thresholds Thresholds   `json:"thresholds"`
}

// ValidationError explains a rejected configuration. Its message is shown to the administrator.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// Validate checks the non-secret settings and returns a trimmed copy.
func Validate(in Settings) (Settings, error) {
	s := in
	s.Email.Host = strings.TrimSpace(s.Email.Host)
	s.Email.Username = strings.TrimSpace(s.Email.Username)
	s.Email.From = strings.TrimSpace(s.Email.From)
	s.Email.To = strings.TrimSpace(s.Email.To)
	s.Telegram.ChatID = strings.TrimSpace(s.Telegram.ChatID)

	if s.Email.Port < 1 || s.Email.Port > 65535 {
		return s, invalid("The SMTP port must be between 1 and 65535.")
	}
	if err := validateThresholds(s.Thresholds); err != nil {
		return s, err
	}
	if s.Email.Enabled {
		if err := validateEmail(s.Email); err != nil {
			return s, err
		}
	}
	if s.Telegram.Enabled {
		if s.Telegram.ChatID == "" || len(s.Telegram.ChatID) > maxChatIDLen || hasSpace(s.Telegram.ChatID) {
			return s, invalid("Enter the Telegram chat ID to send alerts to.")
		}
	}
	return s, nil
}

func validateThresholds(t Thresholds) error {
	checks := []struct {
		name  string
		value int
	}{{"CPU", t.CPU}, {"memory", t.Memory}, {"disk", t.Disk}}
	for _, c := range checks {
		if c.value < 1 || c.value > 100 {
			return invalid("The %s threshold must be between 1 and 100 percent.", c.name)
		}
	}
	return nil
}

func validateEmail(e EmailSettings) error {
	if e.Host == "" || hasSpace(e.Host) {
		return invalid("Enter the SMTP server host name.")
	}
	if _, err := mail.ParseAddress(e.From); err != nil {
		return invalid("Enter a valid sender address.")
	}
	recipients := 0
	for _, addr := range strings.Split(e.To, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, err := mail.ParseAddress(addr); err != nil {
			return invalid("Enter valid recipient addresses, separated by commas.")
		}
		recipients++
	}
	if recipients == 0 {
		return invalid("Enter at least one recipient address.")
	}
	return nil
}

// ValidateSecret checks a password or token supplied by the client. Empty means keep the current one.
func ValidateSecret(v string) error {
	if len(v) > maxSecretLen {
		return invalid("The password or token is too long.")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return invalid("The password or token contains control characters.")
		}
	}
	return nil
}

func hasSpace(s string) bool {
	return strings.IndexFunc(s, unicode.IsSpace) >= 0
}
