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
	"github.com/zenexcloud/zenex-panel/apps/api/internal/dnscheck"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/httpapi"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/monitor"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/provision"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/update"
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
			return createAdmin(ctx, s, cfg.NodeName, cfg.PublicIP, os.Stdin, os.Stdout)
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

	nodeID, err := s.EnsureLocalNode(ctx, cfg.NodeName, cfg.PublicIP)
	if err != nil {
		// Not fatal: the panel still serves sign-in until an administrator exists.
		log.Warn("this server is not registered yet; website creation is unavailable", "reason", err.Error())
	}

	helpers := helperclient.New(cfg.HelperSocket)
	prov := provision.New(s, helpers, []byte(cfg.SecretKey), log)
	manager := manage.New(s, helpers, log)
	startJob := func(jobID string) {
		go func() {
			if err := prov.Run(context.Background(), jobID); err != nil {
				log.Error("provisioning job failed", "job_id", jobID, "error", err)
			}
		}()
	}
	// A restart can leave jobs half done. Put them back in the queue and resume.
	if err := s.ResetRunningJobs(ctx); err != nil {
		return fmt.Errorf("reset jobs: %w", err)
	}
	// Host metrics and website uptime are recorded in the background.
	go monitor.New(s, log).Run(ctx)

	pending, err := s.UnfinishedProvisionJobs(ctx)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	for _, id := range pending {
		startJob(id)
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Log:     log,
			Users:   s,
			Sites:   s,
			Manage:  manager,
			Monitor: s,
			Updates: update.New(helpers),
			Site: httpapi.SiteSettings{
				NodeID:     nodeID,
				PHPVersion: cfg.PHPVersion,
				SecretKey:  []byte(cfg.SecretKey),
				StartJob:   startJob,
				DNS:        dnscheck.New(cfg.PublicIP),
			},
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
func createAdmin(ctx context.Context, s *store.Store, nodeName, publicIP string, in io.Reader, out io.Writer) error {
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
	id, err := s.UpsertAdmin(ctx, email, hash)
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	if _, err := s.EnsureLocalNode(ctx, nodeName, publicIP); err != nil {
		return fmt.Errorf("register this server: %w", err)
	}
	fmt.Fprintf(out, "administrator ready: %s (id %s)\n", email, id)
	return nil
}
