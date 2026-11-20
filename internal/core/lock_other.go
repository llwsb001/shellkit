//go:build !unix

package core

import (
	"os"
	"path/filepath"
)

// lockStore on platforms without flock: keep the lock file for symmetry, but
// the in-process mutex is all we get, as documented.
func lockStore() (func(), error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(Dir(), "notes.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
