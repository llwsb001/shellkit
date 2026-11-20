//go:build unix

package core

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockStore takes an exclusive advisory lock on the store directory, so a CLI
// add and a running server cannot both read the old list and write their own
// version (the in-process mutex cannot see another process).
func lockStore() (func(), error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(Dir(), "notes.lock"), os.O_CREATE|os.O_RDWR, 0o644)
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
