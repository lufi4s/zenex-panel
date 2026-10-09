package alerts

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Mailer sends one plain-text message. Implemented by SMTPMailer; tests use a fake.
type Mailer interface {
	Send(ctx context.Context, cfg EmailSettings, password, subject, body string) error
}

// TelegramSender posts one message to a chat. Implemented by HTTPTelegram; tests use a fake.
type TelegramSender interface {
	Send(ctx context.Context, token, chatID, text string) error
}

// SMTPMailer delivers mail with net/smtp. Port 465 uses implicit TLS; any other
// port must offer STARTTLS, and the password is only sent after TLS is up.
type SMTPMailer struct {
	Timeout time.Duration
}

// Send delivers one message. Errors never contain the password.
func (m SMTPMailer) Send(ctx context.Context, cfg EmailSettings, password, subject, body string) error {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return errors.New("sender address is invalid")
	}
	recipients, err := parseRecipients(cfg.To)
	if err != nil {
		return err
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	conn, err := m.connect(ctx, cfg, timeout)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return errors.New("SMTP server did not greet correctly")
	}
	defer c.Close()

	if err := m.secure(c, cfg); err != nil {
		return err
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, password, cfg.Host)); err != nil {
			return errors.New("SMTP sign-in was refused; check the username and password")
		}
	}
	msg, err := buildMessage(from, recipients, subject, body, time.Now())
	if err != nil {
		return errors.New("could not build the message")
	}
	return sendMessage(c, from.Address, recipients, msg)
}

func (m SMTPMailer) connect(ctx context.Context, cfg EmailSettings, timeout time.Duration) (net.Conn, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: timeout}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if cfg.Port == 465 {
		raw, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, errors.New("could not reach the SMTP server")
		}
		conn := tls.Client(raw, tlsCfg)
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, errors.New("TLS handshake with the SMTP server failed")
		}
		return conn, nil
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, errors.New("could not reach the SMTP server")
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	return conn, nil
}

// secure upgrades a plain connection with STARTTLS unless it is already TLS (port 465).
func (m SMTPMailer) secure(c *smtp.Client, cfg EmailSettings) error {
	if cfg.Port == 465 {
		return nil
	}
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return errors.New("the SMTP server does not offer STARTTLS; use port 465 or a server with TLS")
	}
	if err := c.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return errors.New("STARTTLS with the SMTP server failed")
	}
	return nil
}

func sendMessage(c *smtp.Client, sender string, recipients []string, msg []byte) error {
	if err := c.Mail(sender); err != nil {
		return errors.New("SMTP server refused the sender address")
	}
	for _, r := range recipients {
		if err := c.Rcpt(r); err != nil {
			return errors.New("SMTP server refused a recipient address")
		}
	}
	w, err := c.Data()
	if err != nil {
		return errors.New("SMTP server refused the message")
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return errors.New("sending the message failed")
	}
	if err := w.Close(); err != nil {
		return errors.New("SMTP server did not accept the message")
	}
	return c.Quit()
}

func parseRecipients(to string) ([]string, error) {
	var out []string
	for _, raw := range strings.Split(to, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		addr, err := mail.ParseAddress(raw)
		if err != nil {
			return nil, errors.New("a recipient address is invalid")
		}
		out = append(out, addr.Address)
	}
	if len(out) == 0 {
		return nil, errors.New("no recipient address")
	}
	return out, nil
}

// buildMessage writes an RFC 5322 message: a plain-text part with a short HTML
// alternative, From and Reply-To set to the configured sender, Date, a Message-ID
// on the sender's domain and MIME headers. CR and LF are removed from the subject
// so it cannot inject headers.
//
// Deliverability: Gmail and most providers only accept a From address equal to
// the SMTP login address (or a verified "send as" alias). Set the alert "from"
// setting to the same address as the SMTP username, or mail may be rejected or
// marked as spam.
func buildMessage(from *mail.Address, to []string, subject, body string, now time.Time) ([]byte, error) {
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	msgID, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	boundary, err := randomToken(12)
	if err != nil {
		return nil, err
	}
	text, err := quotedPrintable(strings.ReplaceAll(body, "\r\n", "\n"))
	if err != nil {
		return nil, err
	}
	html, err := quotedPrintable(htmlBody(body))
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("Reply-To: " + from.String() + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	b.WriteString("Date: " + now.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + msgID + "@" + domain + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	b.WriteString(text + "\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	b.WriteString(html + "\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String()), nil
}

// htmlBody wraps each line of the plain text in a paragraph. Every character is escaped.
func htmlBody(text string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><body style=\"font-family:Arial,Helvetica,sans-serif;font-size:14px;color:#222\">")
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		b.WriteString("<p style=\"margin:0 0 8px\">" + html.EscapeString(line) + "</p>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

// quotedPrintable keeps lines short and 7-bit safe for every relay.
func quotedPrintable(s string) (string, error) {
	var b strings.Builder
	w := quotedprintable.NewWriter(&b)
	if _, err := w.Write([]byte(s)); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// HTTPTelegram posts messages through the Telegram Bot API over HTTPS.
type HTTPTelegram struct {
	Client  *http.Client
	BaseURL string // https://api.telegram.org in production
}

// NewHTTPTelegram returns a sender for the public Bot API.
func NewHTTPTelegram() *HTTPTelegram {
	return &HTTPTelegram{Client: &http.Client{Timeout: 15 * time.Second}, BaseURL: "https://api.telegram.org"}
}

// Send posts one message. The token is part of the request URL, so transport
// errors are reduced to their cause before they leave this function.
func (t *HTTPTelegram) Send(ctx context.Context, token, chatID, text string) error {
	form := url.Values{"chat_id": {chatID}, "text": {text}}
	endpoint := t.BaseURL + "/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("could not build the Telegram request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.Client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return fmt.Errorf("telegram request failed: %v", uerr.Err)
		}
		return errors.New("telegram request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram rejected the message (HTTP %d)", resp.StatusCode)
	}
	return nil
}
