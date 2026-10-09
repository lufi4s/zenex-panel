package helper

import (
	"context"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// blockingExec holds every ssh command until release is closed.
type blockingExec struct {
	scriptExec
	started chan struct{}
	release chan struct{}
}

func (b *blockingExec) RunEnv(ctx context.Context, bin string, args []string, env []string, timeout time.Duration) (executor.Result, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-b.release
	return b.scriptExec.RunEnv(ctx, bin, args, env, timeout)
}

func TestSlowOperationsDoNotBlockFolderListings(t *testing.T) {
	ex := &blockingExec{started: make(chan struct{}, 1), release: make(chan struct{})}
	o := siteFixture(t)
	o.Exec = ex
	o.Paths.BackupKeyDir = t.TempDir()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = o.Do(context.Background(), "cpanel.scan", map[string]string{
			"host": "cpanel.example.com", "port": "22", "username": "acct", "password": "pw",
		})
	}()
	select {
	case <-ex.started:
	case <-time.After(3 * time.Second):
		t.Fatal("the slow operation never started")
	}

	listed := make(chan error, 1)
	go func() {
		_, err := o.Do(context.Background(), "files.list", map[string]string{"user": "zx_shop", "path": ""})
		listed <- err
	}()
	select {
	case err := <-listed:
		if err != nil {
			t.Fatalf("files.list: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a folder listing waited for a slow operation")
	}
	close(ex.release)
	<-done
}

func TestOperationsThatChangeSharedConfigurationStayLocked(t *testing.T) {
	for _, op := range []string{"vhost.write", "vhost.disable", "vhost.enable", "pool.write", "php.switch", "php.restart",
		"site.purge", "user.create", "db.create", "fs.prepare", "wp.config-create", "files.write", "files.delete", "files.import"} {
		if runsWithoutLock(op) {
			t.Errorf("%s runs without the lock", op)
		}
	}
	for _, op := range []string{"files.list", "logs.tail", "backup.create", "backup.restore", "cpanel.pull"} {
		if !runsWithoutLock(op) {
			t.Errorf("%s still takes the lock", op)
		}
	}
}
