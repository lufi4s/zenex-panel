// Command api runs the Zenex control-plane API and UI.
//
//	api serve            run the HTTP(S) server (applies pending migrations first)
//	api migrate          apply pending migrations and exit
//	api create-admin     create an administrator; reads email and password from stdin
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/auth"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/config"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/httpapi"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	switch cmd {
	case "serve":
		err = serve(cfg, log)
	case "migrate":
		err = withStore(cfg, func(ctx context.Context, s *store.Store) error {
			return s.Migrate(ctx, migrations.FS)
		})
	case "create-admin":
		err = withStore(cfg, func(ctx context.Context, s *store.Store) error {
			if err := s.Migrate(ctx, migrations.FS); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			return createAdmin(ctx, s, os.Stdin, os.Stdout)
		})
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		log.Error("command failed", "command", cmd, "error", err)
		os.Exit(1)
	}
}

func withStore(cfg config.Config, fn func(context.Context, *store.Store) error) error {
	if cfg.DatabaseURL == "" {
		return errors.New("ZENEX_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer s.Close()
	return fn(ctx, s)
}

func serve(cfg config.Config, log *slog.Logger) error {
	if cfg.DatabaseURL == "" {
		return errors.New("ZENEX_DATABASE_URL is required to serve")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Migrate(ctx, migrations.FS); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Log:           log,
			Users:         s,
			SecureCookies: cfg.TLSEnabled(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.ListenAddr, "env", cfg.Env, "tls", cfg.TLSEnabled(), "version", httpapi.Version)
		if cfg.TLSEnabled() {
			errc <- srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// createAdmin reads the email on the first line and the password on the second,
// so secrets never appear in process arguments or shell history.
func createAdmin(ctx context.Context, s *store.Store, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	email, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	password, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	password = strings.TrimRight(password, "\r\n")
	if !strings.Contains(email, "@") || len(password) < 12 {
		return errors.New("input must be: email line, then password (min 12 characters)")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	id, err := s.CreateUser(ctx, email, hash, "administrator")
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	fmt.Fprintf(out, "administrator created: %s (id %s)\n", email, id)
	return nil
}
