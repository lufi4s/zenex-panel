// Package executor runs external programs for the Zenex Agent.
//
// Rules enforced here:
//   - Only absolute binary paths that were explicitly allowlisted at construction.
//   - Arguments are passed as an argv array. No shell is ever involved.
//   - A minimal, fixed environment is used; the parent environment is not inherited.
//   - Every run has a hard timeout and a capped output size.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultMaxOutput = 64 * 1024
	safePath         = "/usr/local/sbin:/usr/sbin:/sbin:/usr/local/bin:/usr/bin:/bin"
)

var (
	ErrBinaryNotAllowed = errors.New("binary is not on the allowlist")
	ErrInvalidArgument  = errors.New("argument contains forbidden characters")
	ErrTimeout          = errors.New("command timed out")
	ErrInvalidEnv       = errors.New("environment contains forbidden entries")

	envNameRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// Result is the outcome of a finished command.
type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Truncated bool
	Duration  time.Duration
}

// Executor runs a program with arguments. Implemented by Runner; tests use fakes.
type Executor interface {
	Run(ctx context.Context, bin string, args []string, timeout time.Duration) (Result, error)
}

// Runner is the production Executor.
type Runner struct {
	allowed map[string]struct{}
}

// NewRunner builds a Runner that may only execute the given absolute paths.
func NewRunner(allowedBinaries ...string) (*Runner, error) {
	allowed := make(map[string]struct{}, len(allowedBinaries))
	for _, b := range allowedBinaries {
		if !filepath.IsAbs(b) || filepath.Clean(b) != b {
			return nil, fmt.Errorf("allowlisted binary must be a clean absolute path: %q", b)
		}
		allowed[b] = struct{}{}
	}
	return &Runner{allowed: allowed}, nil
}

// Run executes bin with args. A non-zero exit code is not an error; it is
// reported in Result.ExitCode. An error is returned only when the command could
// not be run, was not allowlisted, had invalid arguments, or timed out.
func (r *Runner) Run(ctx context.Context, bin string, args []string, timeout time.Duration) (Result, error) {
	return r.RunInput(ctx, bin, args, "", timeout)
}

// RunInput is Run with data written to the program's standard input. Use it for
// secrets and multi-line payloads (for example SQL) so they never appear in argv.
func (r *Runner) RunInput(ctx context.Context, bin string, args []string, stdin string, timeout time.Duration) (Result, error) {
	return r.run(ctx, bin, args, stdin, nil, timeout)
}

// RunEnv is Run with extra environment variables for this one child process.
// Each entry is NAME=VALUE. The variables exist only in that child; the next run
// starts again from the fixed environment. PATH and LC_ALL cannot be overridden.
func (r *Runner) RunEnv(ctx context.Context, bin string, args []string, env []string, timeout time.Duration) (Result, error) {
	return r.run(ctx, bin, args, "", env, timeout)
}

func (r *Runner) run(ctx context.Context, bin string, args []string, stdin string, extraEnv []string, timeout time.Duration) (Result, error) {
	if _, ok := r.allowed[bin]; !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrBinaryNotAllowed, bin)
	}
	for _, a := range args {
		if strings.ContainsAny(a, "\x00\n\r") {
			return Result{}, ErrInvalidArgument
		}
	}
	for _, kv := range extraEnv {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !envNameRe.MatchString(name) || name == "PATH" || name == "LC_ALL" ||
			strings.ContainsAny(value, "\x00\n\r") {
			return Result{}, ErrInvalidEnv
		}
	}
	if timeout <= 0 {
		return Result{}, errors.New("timeout must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = defaultMaxOutput, defaultMaxOutput

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append([]string{"PATH=" + safePath, "LC_ALL=C"}, extraEnv...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	start := time.Now()
	err := cmd.Run()
	res := Result{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Truncated: stdout.truncated || stderr.truncated,
		Duration:  time.Since(start),
	}

	if ctx.Err() == context.DeadlineExceeded {
		return res, ErrTimeout
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		return res, fmt.Errorf("start %s: %w", filepath.Base(bin), err)
	}
	return res, nil
}

// limitedBuffer keeps at most limit bytes and discards the rest.
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	remaining := l.limit - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		l.buf.Write(p[:remaining])
		l.truncated = true
		return len(p), nil
	}
	l.buf.Write(p)
	return len(p), nil
}

func (l *limitedBuffer) String() string { return l.buf.String() }
