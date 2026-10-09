package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeStore struct {
	mu       sync.Mutex
	settings map[string]json.RawMessage
	admins   []string
	notes    []string // "level|title|body"
}

func newFakeStore(admins ...string) *fakeStore {
	return &fakeStore{settings: map[string]json.RawMessage{}, admins: admins}
}

func (f *fakeStore) GetSetting(_ context.Context, key string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.settings[key]; ok {
		return v, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) PutSetting(_ context.Context, key string, value any, _ string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings[key] = raw
	return nil
}

func (f *fakeStore) AdministratorIDs(context.Context) ([]string, error) { return f.admins, nil }

func (f *fakeStore) Notify(_ context.Context, _, level, title, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, level+"|"+title+"|"+body)
	return nil
}

type fakeMailer struct {
	msgs      []string
	passwords []string
}

func (f *fakeMailer) Send(_ context.Context, _ EmailSettings, password, subject, body string) error {
	f.msgs = append(f.msgs, subject+"\n"+body)
	f.passwords = append(f.passwords, password)
	return nil
}

type fakeTelegram struct {
	msgs   []string
	tokens []string
}

func (f *fakeTelegram) Send(_ context.Context, token, _, text string) error {
	f.msgs = append(f.msgs, text)
	f.tokens = append(f.tokens, token)
	return nil
}

const testSecret = "test-secret-key-0123456789abcdef0123"

func newService(t *testing.T, st *fakeStore) (*Service, *fakeMailer, *fakeTelegram) {
	t.Helper()
	svc := New(st, testSecret, "node-1", quiet())
	mail, tg := &fakeMailer{}, &fakeTelegram{}
	svc.Mail = mail
	svc.Telegram = tg
	return svc, mail, tg
}

func enabledInput() Input {
	return Input{
		Email: EmailInput{
			EmailSettings: EmailSettings{Enabled: true, Host: "smtp.example.com", Port: 587,
				Username: "alerts", From: "alerts@example.com", To: "ops@example.com"},
			Password: "smtp-pass-123",
		},
		Telegram: TelegramInput{
			TelegramSettings: TelegramSettings{Enabled: true, ChatID: "-100123"},
			Token:            "111:bot-token-xyz",
		},
		Thresholds: Thresholds{CPU: 85, Memory: 90, Disk: 90},
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestValidateAcceptsDefaultsAndTrims(t *testing.T) {
	in := DefaultSettings()
	in.Email.Host = "  smtp.example.com "
	got, err := Validate(in)
	if err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
	if got.Email.Host != "smtp.example.com" {
		t.Fatalf("host not trimmed: %q", got.Email.Host)
	}
}

func TestValidateRejectsBadSettings(t *testing.T) {
	base := func() Settings {
		s := DefaultSettings()
		s.Thresholds = Thresholds{CPU: 85, Memory: 90, Disk: 90}
		return s
	}
	cases := map[string]func(*Settings){
		"port zero":       func(s *Settings) { s.Email.Port = 0 },
		"port too high":   func(s *Settings) { s.Email.Port = 65536 },
		"cpu threshold 0": func(s *Settings) { s.Thresholds.CPU = 0 },
		"memory over 100": func(s *Settings) { s.Thresholds.Memory = 101 },
		"disk negative":   func(s *Settings) { s.Thresholds.Disk = -5 },
		"email no host": func(s *Settings) {
			s.Email.Enabled = true
			s.Email.From = "a@example.com"
			s.Email.To = "b@example.com"
		},
		"email bad from": func(s *Settings) {
			s.Email.Enabled = true
			s.Email.Host = "h"
			s.Email.From = "nope"
			s.Email.To = "b@example.com"
		},
		"email no to": func(s *Settings) {
			s.Email.Enabled = true
			s.Email.Host = "h"
			s.Email.From = "a@example.com"
			s.Email.To = " , "
		},
		"email bad to": func(s *Settings) {
			s.Email.Enabled = true
			s.Email.Host = "h"
			s.Email.From = "a@example.com"
			s.Email.To = "x@y.z, bad"
		},
		"telegram no chat":  func(s *Settings) { s.Telegram.Enabled = true },
		"telegram spaced":   func(s *Settings) { s.Telegram.Enabled = true; s.Telegram.ChatID = "1 2" },
		"telegram too long": func(s *Settings) { s.Telegram.Enabled = true; s.Telegram.ChatID = strings.Repeat("1", 65) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := base()
			mutate(&s)
			_, err := Validate(s)
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a validation error, got %v", err)
			}
		})
	}
}

func TestDisabledChannelsSkipTheirChecks(t *testing.T) {
	s := DefaultSettings()
	s.Email.Enabled = false
	s.Telegram.Enabled = false
	if _, err := Validate(s); err != nil {
		t.Fatalf("disabled channels should not need addresses: %v", err)
	}
}

func TestValidateSecretLimits(t *testing.T) {
	if err := ValidateSecret(strings.Repeat("x", 513)); err == nil {
		t.Error("oversized secret accepted")
	}
	if err := ValidateSecret("line\nbreak"); err == nil {
		t.Error("secret with control character accepted")
	}
	if err := ValidateSecret("ok-secret"); err != nil {
		t.Errorf("normal secret rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Encryption
// ---------------------------------------------------------------------------

func TestSealOpenRoundTrip(t *testing.T) {
	key := DeriveKey(testSecret)
	sealed, err := Seal(key, "smtp-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "smtp-pass") {
		t.Fatal("ciphertext contains the plaintext")
	}
	plain, err := Open(key, sealed)
	if err != nil || plain != "smtp-pass-123" {
		t.Fatalf("round trip = %q, %v", plain, err)
	}
	again, _ := Seal(key, "smtp-pass-123")
	if again == sealed {
		t.Fatal("two seals of the same value are identical (nonce reused)")
	}
}

func TestOpenRejectsWrongKeyAndDamage(t *testing.T) {
	sealed, _ := Seal(DeriveKey(testSecret), "value")
	if _, err := Open(DeriveKey("another-secret"), sealed); !errors.Is(err, ErrCiphertext) {
		t.Errorf("wrong key: err = %v", err)
	}
	if _, err := Open(DeriveKey(testSecret), "AAAA"); !errors.Is(err, ErrCiphertext) {
		t.Errorf("short value: err = %v", err)
	}
	if _, err := Open(DeriveKey(testSecret), "not base64!!"); !errors.Is(err, ErrCiphertext) {
		t.Errorf("invalid base64: err = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Thresholds, cooldown and recovery
// ---------------------------------------------------------------------------

var th = Thresholds{CPU: 85, Memory: 90, Disk: 90}

func readings(cpu float64) Readings { return Readings{CPU: cpu, Memory: 10, Disk: 10} }

func TestReadingsFromSnapshot(t *testing.T) {
	r := ReadingsFrom(sysinfo.Metrics{CPUCount: 4, Load1: 2, MemTotalBytes: 100, MemUsedBytes: 50, DiskTotalBytes: 200, DiskUsedBytes: 20})
	if r.CPU != 50 || r.Memory != 50 || r.Disk != 10 {
		t.Fatalf("readings = %+v", r)
	}
	unknown := ReadingsFrom(sysinfo.Metrics{})
	if unknown.CPU >= 0 || unknown.Memory >= 0 || unknown.Disk >= 0 {
		t.Fatalf("zero totals should be unknown, got %+v", unknown)
	}
}

func TestTrackerFiresOnceThenRecovers(t *testing.T) {
	tr := NewTracker(DefaultCooldown)
	now := time.Unix(1_000_000, 0)

	if ev := tr.Evaluate(now, readings(80), th); len(ev) != 0 {
		t.Fatalf("below threshold produced %v", ev)
	}
	ev := tr.Evaluate(now, readings(90), th)
	if len(ev) != 1 || ev[0].Metric != MetricCPU || ev[0].Recovered || ev[0].Limit != 85 {
		t.Fatalf("crossing produced %+v", ev)
	}
	if ev := tr.Evaluate(now.Add(time.Minute), readings(95), th); len(ev) != 0 {
		t.Fatalf("still over threshold produced a repeat: %+v", ev)
	}
	ev = tr.Evaluate(now.Add(2*time.Minute), readings(40), th)
	if len(ev) != 1 || !ev[0].Recovered {
		t.Fatalf("drop produced %+v, want one recovery", ev)
	}
	if ev := tr.Evaluate(now.Add(3*time.Minute), readings(40), th); len(ev) != 0 {
		t.Fatalf("recovered metric produced events: %+v", ev)
	}
}

func TestTrackerCooldownSuppressesRepeatAndAnnouncesLater(t *testing.T) {
	tr := NewTracker(DefaultCooldown)
	t0 := time.Unix(2_000_000, 0)

	if ev := tr.Evaluate(t0, readings(90), th); len(ev) != 1 {
		t.Fatalf("first crossing: %+v", ev)
	}
	// Recovers after 2 minutes: a recovery is sent.
	if ev := tr.Evaluate(t0.Add(2*time.Minute), readings(40), th); len(ev) != 1 || !ev[0].Recovered {
		t.Fatalf("recovery: %+v", ev)
	}
	// Goes over again 3 minutes after the alert: suppressed by the cooldown.
	if ev := tr.Evaluate(t0.Add(3*time.Minute), readings(90), th); len(ev) != 0 {
		t.Fatalf("alert inside cooldown was sent: %+v", ev)
	}
	// Drops again before the cooldown ends: nothing was sent, so no recovery.
	if ev := tr.Evaluate(t0.Add(4*time.Minute), readings(40), th); len(ev) != 0 {
		t.Fatalf("recovery for a suppressed alert: %+v", ev)
	}
	// Over again after the cooldown: announced.
	if ev := tr.Evaluate(t0.Add(31*time.Minute), readings(90), th); len(ev) != 1 || ev[0].Recovered {
		t.Fatalf("alert after cooldown: %+v", ev)
	}
}

func TestTrackerAnnouncesSustainedBreachAfterCooldown(t *testing.T) {
	tr := NewTracker(DefaultCooldown)
	t0 := time.Unix(3_000_000, 0)
	tr.Evaluate(t0, readings(40), th)
	// Goes over and is announced; it recovers, then goes over again inside the cooldown.
	tr.Evaluate(t0.Add(time.Minute), readings(90), th)
	tr.Evaluate(t0.Add(2*time.Minute), readings(40), th)
	tr.Evaluate(t0.Add(3*time.Minute), readings(90), th)
	// Still over when the cooldown ends.
	ev := tr.Evaluate(t0.Add(40*time.Minute), readings(91), th)
	if len(ev) != 1 || ev[0].Recovered {
		t.Fatalf("sustained breach not announced after cooldown: %+v", ev)
	}
}

func TestTrackerSkipsUnknownReadings(t *testing.T) {
	tr := NewTracker(DefaultCooldown)
	ev := tr.Evaluate(time.Now(), Readings{CPU: -1, Memory: 95, Disk: -1}, th)
	if len(ev) != 1 || ev[0].Metric != MetricMemory {
		t.Fatalf("events = %+v, want only memory", ev)
	}
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

func TestUpdateStoresSecretsEncryptedAndViewHidesThem(t *testing.T) {
	st := newFakeStore("admin-1")
	svc, _, _ := newService(t, st)
	ctx := context.Background()
	if err := svc.Update(ctx, "admin-1", enabledInput()); err != nil {
		t.Fatal(err)
	}
	raw := string(st.settings[SMTPPasswordKey]) + string(st.settings[TelegramTokenKey])
	if strings.Contains(raw, "smtp-pass-123") || strings.Contains(raw, "bot-token-xyz") {
		t.Fatal("secret stored in plain text")
	}
	settingsJSON, _ := json.Marshal(st.settings[SettingsKey])
	if strings.Contains(string(settingsJSON), "smtp-pass") || strings.Contains(string(settingsJSON), "bot-token") {
		t.Fatal("secret leaked into the settings document")
	}

	view, err := svc.View(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Email.PasswordSet || !view.Telegram.TokenSet {
		t.Fatalf("set flags wrong: %+v", view)
	}
	body, _ := json.Marshal(view)
	if strings.Contains(string(body), "smtp-pass") || strings.Contains(string(body), "bot-token") {
		t.Fatal("view leaks a secret")
	}
}

func TestEmptySecretKeepsExistingPassword(t *testing.T) {
	st := newFakeStore("admin-1")
	svc, mail, _ := newService(t, st)
	ctx := context.Background()
	if err := svc.Update(ctx, "admin-1", enabledInput()); err != nil {
		t.Fatal(err)
	}
	in := enabledInput()
	in.Email.Password = ""
	in.Telegram.Token = ""
	if err := svc.Update(ctx, "admin-1", in); err != nil {
		t.Fatal(err)
	}
	svc.Check(ctx, sysinfo.Metrics{CPUCount: 1, Load1: 0.99, MemTotalBytes: 1, DiskTotalBytes: 1})
	if len(mail.passwords) != 1 || mail.passwords[0] != "smtp-pass-123" {
		t.Fatalf("password after empty update = %v", mail.passwords)
	}
}

func TestSecretsNeedTheServerKey(t *testing.T) {
	st := newFakeStore("admin-1")
	svc := New(st, "", "node-1", quiet())
	err := svc.Update(context.Background(), "admin-1", enabledInput())
	if !errors.Is(err, ErrNoSecretKey) {
		t.Fatalf("err = %v, want ErrNoSecretKey", err)
	}
}

func TestCheckDeliversOnceAndRecoversWithoutLeakingSecrets(t *testing.T) {
	st := newFakeStore("admin-1", "admin-2")
	svc, mail, tg := newService(t, st)
	ctx := context.Background()
	if err := svc.Update(ctx, "admin-1", enabledInput()); err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(4_000_000, 0)
	svc.now = func() time.Time { return clock }

	hot := sysinfo.Metrics{CPUCount: 4, Load1: 3.6, MemTotalBytes: 100, MemUsedBytes: 10, DiskTotalBytes: 100, DiskUsedBytes: 10}
	cool := sysinfo.Metrics{CPUCount: 4, Load1: 0.4, MemTotalBytes: 100, MemUsedBytes: 10, DiskTotalBytes: 100, DiskUsedBytes: 10}

	svc.Check(ctx, hot) // fires
	clock = clock.Add(time.Minute)
	svc.Check(ctx, hot) // still high: nothing new
	clock = clock.Add(time.Minute)
	svc.Check(ctx, cool) // recovers
	clock = clock.Add(time.Minute)
	svc.Check(ctx, hot) // inside cooldown: suppressed
	clock = clock.Add(40 * time.Minute)
	svc.Check(ctx, hot) // cooldown over: fires

	if len(mail.msgs) != 3 || len(tg.msgs) != 3 {
		t.Fatalf("email %d, telegram %d; want 3 each", len(mail.msgs), len(tg.msgs))
	}
	if len(st.notes) != 6 {
		t.Fatalf("in-panel notifications = %d, want 6 (2 admins x 3 events)", len(st.notes))
	}
	if !strings.Contains(mail.msgs[1], "back to normal") {
		t.Errorf("second message is not a recovery: %q", mail.msgs[1])
	}
	if tg.tokens[0] != "111:bot-token-xyz" || mail.passwords[0] != "smtp-pass-123" {
		t.Error("decrypted secrets were not passed to the senders")
	}
	all := strings.Join(append(append(mail.msgs, tg.msgs...), st.notes...), "\n")
	if strings.Contains(all, "smtp-pass") || strings.Contains(all, "bot-token") {
		t.Fatal("a secret appeared in an alert message")
	}
}

func TestCheckSkipsDisabledChannels(t *testing.T) {
	st := newFakeStore("admin-1")
	svc, mail, tg := newService(t, st)
	in := enabledInput()
	in.Email.Enabled = false
	in.Telegram.Enabled = false
	if err := svc.Update(context.Background(), "admin-1", in); err != nil {
		t.Fatal(err)
	}
	svc.Check(context.Background(), sysinfo.Metrics{CPUCount: 1, Load1: 2, MemTotalBytes: 1, DiskTotalBytes: 1})
	if len(mail.msgs) != 0 || len(tg.msgs) != 0 {
		t.Fatal("disabled channel was used")
	}
	if len(st.notes) != 1 {
		t.Fatalf("in-panel notification missing: %v", st.notes)
	}
}

func TestDescribe(t *testing.T) {
	subject, body := Describe(Event{Metric: MetricDisk, Value: 93.25, Limit: 90}, "node-1")
	if !strings.Contains(subject, "High Disk use on node-1") || !strings.Contains(body, "93.2%") {
		t.Fatalf("subject=%q body=%q", subject, body)
	}
}

// ---------------------------------------------------------------------------
// Telegram sender
// ---------------------------------------------------------------------------

func TestTelegramSendPostsFormToBotEndpoint(t *testing.T) {
	var gotPath, gotChat, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotChat, gotText = r.PostForm.Get("chat_id"), r.PostForm.Get("text")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tg := &HTTPTelegram{Client: srv.Client(), BaseURL: srv.URL}
	if err := tg.Send(context.Background(), "111:tok", "-100123", "hello & welcome"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot111:tok/sendMessage" || gotChat != "-100123" || gotText != "hello & welcome" {
		t.Fatalf("request path=%q chat=%q text=%q", gotPath, gotChat, gotText)
	}
}

func TestTelegramErrorsDoNotContainTheToken(t *testing.T) {
	const token = "999:super-secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	statusErr := (&HTTPTelegram{Client: srv.Client(), BaseURL: srv.URL}).Send(context.Background(), token, "1", "x")
	srv.Close()
	if statusErr == nil || !strings.Contains(statusErr.Error(), "HTTP 401") || strings.Contains(statusErr.Error(), token) {
		t.Fatalf("status error = %v", statusErr)
	}

	// The server is closed, so the connection fails. The URL with the token must not leak.
	netErr := (&HTTPTelegram{Client: &http.Client{Timeout: time.Second}, BaseURL: "http://127.0.0.1:1"}).Send(context.Background(), token, "1", "x")
	if netErr == nil || strings.Contains(netErr.Error(), token) {
		t.Fatalf("network error = %v", netErr)
	}
}

// ---------------------------------------------------------------------------
// Test email and message format
// ---------------------------------------------------------------------------

type failingMailer struct{ err error }

func (f *failingMailer) Send(context.Context, EmailSettings, string, string, string) error {
	return f.err
}

func TestSendTestEmailUsesSavedSettings(t *testing.T) {
	st := newFakeStore("admin-1")
	svc, mail, _ := newService(t, st)
	if err := svc.Update(context.Background(), "admin-1", enabledInput()); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendTestEmail(context.Background()); err != nil {
		t.Fatalf("SendTestEmail = %v", err)
	}
	if len(mail.msgs) != 1 || !strings.HasPrefix(mail.msgs[0], "Zenex panel: test email\n") {
		t.Fatalf("messages = %q", mail.msgs)
	}
	if !strings.Contains(mail.msgs[0], "test message from the Zenex panel") {
		t.Fatalf("body = %q", mail.msgs[0])
	}
	if mail.passwords[0] != "smtp-pass-123" {
		t.Fatalf("password not taken from the store: %q", mail.passwords[0])
	}
}

func TestSendTestEmailNeedsCompleteEnabledSettings(t *testing.T) {
	st := newFakeStore("admin-1")
	svc, mail, _ := newService(t, st)
	ctx := context.Background()

	if err := svc.SendTestEmail(ctx); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("nothing saved: err = %v", err)
	}
	in := enabledInput()
	in.Email.Enabled = false
	if err := svc.Update(ctx, "admin-1", in); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendTestEmail(ctx); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("disabled: err = %v", err)
	}
	in = enabledInput()
	in.Email.Password = ""
	st.settings = map[string]json.RawMessage{}
	if err := svc.Update(ctx, "admin-1", in); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendTestEmail(ctx); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("username without password: err = %v", err)
	}
	if len(mail.msgs) != 0 {
		t.Fatal("mail sent although email is not configured")
	}
}

func TestSendTestEmailFailureHidesPassword(t *testing.T) {
	st := newFakeStore("admin-1")
	svc, _, _ := newService(t, st)
	ctx := context.Background()
	if err := svc.Update(ctx, "admin-1", enabledInput()); err != nil {
		t.Fatal(err)
	}
	svc.Mail = &failingMailer{err: errors.New("SMTP sign-in with smtp-pass-123 was refused " + strings.Repeat("x", 500))}
	err := svc.SendTestEmail(ctx)
	var derr *DeliveryError
	if !errors.As(err, &derr) {
		t.Fatalf("err = %v, want DeliveryError", err)
	}
	if strings.Contains(derr.Message, "smtp-pass-123") || len(derr.Message) > maxDeliveryErrLen {
		t.Fatalf("unsafe or long message: %q", derr.Message)
	}
}

func TestBuildMessageHeadersAndAlternatives(t *testing.T) {
	from, err := mail.ParseAddress("Zenex <alerts@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	raw, err := buildMessage(from, []string{"ops@example.com"}, "Zenex panel: test email\r\nBcc: evil@example.com", "Line one <b>\nLine two", now)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(raw)
	headers, _, ok := strings.Cut(msg, "\r\n\r\n")
	if !ok {
		t.Fatal("no header/body separator")
	}
	for _, want := range []string{
		"From: \"Zenex\" <alerts@example.com>\r\n",
		"Reply-To: \"Zenex\" <alerts@example.com>\r\n",
		"Date: Fri, 09 Oct 2026 12:00:00 +0000\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: multipart/alternative; boundary=",
	} {
		if !strings.Contains(headers, want) {
			t.Errorf("missing header %q", want)
		}
	}
	if !regexp.MustCompile(`Message-ID: <[0-9a-f]{32}@example\.com>`).MatchString(headers) {
		t.Errorf("Message-ID not on the sender domain: %s", headers)
	}
	if strings.Contains(headers, "\r\nBcc:") {
		t.Error("subject injected a header")
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8\r\n") ||
		!strings.Contains(msg, "Content-Type: text/html; charset=UTF-8\r\n") {
		t.Error("plain-text or HTML part missing")
	}
	if !strings.Contains(msg, "&lt;b&gt;") {
		t.Error("HTML part is not escaped")
	}
	if strings.Count(msg, "\n") != strings.Count(msg, "\r\n") {
		t.Fatal("message contains bare LF line endings")
	}
	for _, line := range strings.Split(msg, "\r\n") {
		if len(line) > 998 {
			t.Fatalf("line longer than 998 bytes")
		}
	}
}
