// Command zenex-helper is the root-level helper for the Zenex panel. It listens
// on a Unix socket that only the panel's service group can open, and performs
// the fixed set of site-building operations defined in internal/helper.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
	"github.com/zenexcloud/zenex-panel/services/agent/internal/helper"
)

const (
	defaultSocket = "/run/zenex/helper.sock"
	defaultGroup  = "zenex"
	maxRequest    = 2 << 20 // large enough for a 1 MB file save
	opTimeout     = 15 * time.Minute
)

type request struct {
	Op   string            `json:"op"`
	Args map[string]string `json:"args"`
}

type response struct {
	OK     bool   `json:"ok"`
	UID    string `json:"uid,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		println("zenex-helper 0.1.0")
		return
	}
	if os.Geteuid() != 0 {
		log.Error("zenex-helper must run as root")
		os.Exit(1)
	}
	if err := run(log); err != nil {
		log.Error("helper stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	socket := envOr("ZENEX_HELPER_SOCKET", defaultSocket)
	group := envOr("ZENEX_HELPER_GROUP", defaultGroup)

	phpBins, _ := filepath.Glob("/usr/sbin/php-fpm[0-9]*.[0-9]*")
	runner, err := executor.NewRunner(helper.AllowedBinaries(phpBins)...)
	if err != nil {
		return err
	}
	ops := &helper.Ops{Exec: runner, Paths: helper.DefaultPaths()}

	ln, err := listen(socket, group)
	if err != nil {
		return err
	}
	defer ln.Close()

	srv := &http.Server{
		Handler:           handler(log, ops),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("helper listening", "socket", socket, "php_versions", len(phpBins))

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func handler(log *slog.Logger, ops *helper.Ops) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/op", func(w http.ResponseWriter, r *http.Request) {
		var req request
		dec := json.NewDecoder(io.LimitReader(r.Body, maxRequest))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "invalid request"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), opTimeout)
		defer cancel()

		// Arguments are never logged: they can contain passwords.
		res, err := ops.Do(ctx, req.Op, req.Args)
		if err != nil {
			log.Warn("operation failed", "op", req.Op, "error", err.Error())
			writeJSON(w, http.StatusUnprocessableEntity, response{Error: err.Error()})
			return
		}
		log.Info("operation ok", "op", req.Op)
		writeJSON(w, http.StatusOK, response{OK: true, UID: res.UID, Output: res.Output})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// listen creates the socket. Only root and members of group can connect.
func listen(path, group string) (net.Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	gid, err := groupID(group)
	if err != nil {
		ln.Close()
		return nil, err
	}
	if err := os.Chown(dir, 0, gid); err != nil {
		ln.Close()
		return nil, err
	}
	if err := os.Chown(path, 0, gid); err != nil {
		ln.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func groupID(name string) (int, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(g.Gid)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
