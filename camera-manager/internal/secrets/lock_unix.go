//go:build unix

package secrets

import (
	"os"
	"syscall"
)

// lockDir takes an exclusive flock on dir so CLI commands and the daemon
// serialize local.env updates. local.env itself is replaced, so it cannot hold the lock.
func lockDir(dir string) (func(), error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
