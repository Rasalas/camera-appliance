//go:build unix

package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LockAccessPolicy coordinates restore with password/policy edits across CLI
// and server processes. The sidecar survives SQLite and credential replacement.
func LockAccessPolicy(ctx context.Context, dbPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(dbPath+".access.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
