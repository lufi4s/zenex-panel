package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

// Store is the persistence surface the alert service needs.
type Store interface {
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	PutSetting(ctx context.Context, key string, value any, userID string) error
	AdministratorIDs(ctx context.Context) ([]string, error)
	Notify(ctx context.Context, userID, level, title, body string) error
}

// Service stores the alert configuration and delivers alerts. The monitor calls
// Check after every host sample.
type Service struct {
	Store    Store
	Mail     Mailer
	Telegram TelegramSender
	Log      *slog.Logger
	// Node names this server in alert messages.
	Node string

	key     []byte
	hasKey  bool
	now     func() time.Time
	mu      sync.Mutex // serialises Check, which owns the tracker state
	tracker *Tracker
}

// New returns a service that sends real email and Telegram messages. secret is
// the server secret (ZENEX_SECRET_KEY); it is never stored or logged.
func New(s Store, secret, node string, log *slog.Logger) *Service {
	return &Service{
		Store:    s,
		Mail:     SMTPMailer{Timeout: 20 * time.Second},
		Telegram: NewHTTPTelegram(),
		Log:      log,
		Node:     node,
		key:      DeriveKey(secret),
		hasKey:   secret != "",
		now:      time.Now,
		tracker:  NewTracker(DefaultCooldown),
	}
}

// Load returns the saved non-secret settings, or the defaults.
func (s *Service) Load(ctx context.Context) (Settings, error) {
	cfg := DefaultSettings()
	raw, err := s.Store.GetSetting(ctx, SettingsKey)
	if errors.Is(err, store.ErrNotFound) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(raw, &cfg)
}

// View returns the settings for the administrator, with secrets reduced to flags.
func (s *Service) View(ctx context.Context) (View, error) {
	cfg, err := s.Load(ctx)
	if err != nil {
		return View{}, err
	}
	pwSet, err := s.isSet(ctx, SMTPPasswordKey)
	if err != nil {
		return View{}, err
	}
	tokenSet, err := s.isSet(ctx, TelegramTokenKey)
	if err != nil {
		return View{}, err
	}
	return View{
		Email:      EmailView{EmailSettings: cfg.Email, PasswordSet: pwSet},
		Telegram:   TelegramView{TelegramSettings: cfg.Telegram, TokenSet: tokenSet},
		Thresholds: cfg.Thresholds,
	}, nil
}

// Update validates and saves the configuration. An empty password or token keeps
// the one already stored.
func (s *Service) Update(ctx context.Context, userID string, in Input) error {
	clean, err := Validate(Settings{Email: in.Email.EmailSettings, Telegram: in.Telegram.TelegramSettings, Thresholds: in.Thresholds})
	if err != nil {
		return err
	}
	if err := ValidateSecret(in.Email.Password); err != nil {
		return err
	}
	if err := ValidateSecret(in.Telegram.Token); err != nil {
		return err
	}
	if in.Email.Password != "" {
		if err := s.saveSecret(ctx, userID, SMTPPasswordKey, in.Email.Password); err != nil {
			return err
		}
	}
	if in.Telegram.Token != "" {
		if err := s.saveSecret(ctx, userID, TelegramTokenKey, in.Telegram.Token); err != nil {
			return err
		}
	}
	return s.Store.PutSetting(ctx, SettingsKey, clean, userID)
}

func (s *Service) isSet(ctx context.Context, key string) (bool, error) {
	_, err := s.Store.GetSetting(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) saveSecret(ctx context.Context, userID, key, value string) error {
	if !s.hasKey {
		return ErrNoSecretKey
	}
	sealed, err := Seal(s.key, value)
	if err != nil {
		return err
	}
	return s.Store.PutSetting(ctx, key, sealed, userID)
}

// secret returns the decrypted value, or "" when none is stored.
func (s *Service) secret(ctx context.Context, key string) (string, error) {
	raw, err := s.Store.GetSetting(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var sealed string
	if err := json.Unmarshal(raw, &sealed); err != nil {
		return "", ErrCiphertext
	}
	if !s.hasKey {
		return "", ErrNoSecretKey
	}
	return Open(s.key, sealed)
}

// Check compares one host sample with the thresholds and delivers any alerts.
// Delivery problems are logged and never stop monitoring.
func (s *Service) Check(ctx context.Context, m sysinfo.Metrics) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.Load(ctx)
	if err != nil {
		s.Log.Warn("loading alert settings failed", "error", err)
		return
	}
	for _, ev := range s.tracker.Evaluate(s.now(), ReadingsFrom(m), cfg.Thresholds) {
		s.deliver(ctx, cfg, ev)
	}
}

func (s *Service) deliver(ctx context.Context, cfg Settings, ev Event) {
	title, body := Describe(ev, s.Node)
	if cfg.Email.Enabled {
		s.sendEmail(ctx, cfg.Email, ev, title, body)
	}
	if cfg.Telegram.Enabled {
		s.sendTelegram(ctx, cfg.Telegram, ev, title+"\n"+body)
	}
	s.notifyAdmins(ctx, ev, title, body)
}

func (s *Service) sendEmail(ctx context.Context, cfg EmailSettings, ev Event, title, body string) {
	password, err := s.secret(ctx, SMTPPasswordKey)
	if err != nil {
		s.Log.Warn("email alert skipped: password unavailable", "metric", ev.Metric, "error", err)
		return
	}
	if err := s.Mail.Send(ctx, cfg, password, title, body); err != nil {
		s.Log.Warn("sending email alert failed", "metric", ev.Metric, "error", err)
	}
}

func (s *Service) sendTelegram(ctx context.Context, cfg TelegramSettings, ev Event, text string) {
	token, err := s.secret(ctx, TelegramTokenKey)
	if err != nil || token == "" {
		s.Log.Warn("telegram alert skipped: bot token unavailable", "metric", ev.Metric)
		return
	}
	if err := s.Telegram.Send(ctx, token, cfg.ChatID, text); err != nil {
		s.Log.Warn("sending telegram alert failed", "metric", ev.Metric, "error", err)
	}
}

func (s *Service) notifyAdmins(ctx context.Context, ev Event, title, body string) {
	ids, err := s.Store.AdministratorIDs(ctx)
	if err != nil {
		s.Log.Warn("listing administrators for alert failed", "error", err)
		return
	}
	level := "warning"
	if ev.Recovered {
		level = "success"
	}
	for _, id := range ids {
		if err := s.Store.Notify(ctx, id, level, title, body); err != nil {
			s.Log.Warn("saving alert notification failed", "error", err)
		}
	}
}

// Describe returns the subject and body of an alert. The node name is optional.
func Describe(ev Event, node string) (string, string) {
	label := metricLabel(ev.Metric)
	where := ""
	if node != "" {
		where = " on " + node
	}
	if ev.Recovered {
		return fmt.Sprintf("[Zenex] %s is back to normal%s", label, where),
			fmt.Sprintf("%s is at %.1f%%, below the %d%% threshold.", label, ev.Value, ev.Limit)
	}
	return fmt.Sprintf("[Zenex] High %s%s", label, where),
		fmt.Sprintf("%s is at %.1f%%, above the %d%% threshold.", label, ev.Value, ev.Limit)
}

func metricLabel(m Metric) string {
	switch m {
	case MetricCPU:
		return "CPU load"
	case MetricMemory:
		return "Memory use"
	case MetricDisk:
		return "Disk use"
	}
	return string(m)
}
