// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// LockState holds an advisory kernel lock until Close or process exit. The
// persistent file is not removed, avoiding a replacement-inode lock race.
func LockState(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another daemon owns this state directory: %w", err)
	}
	return f, nil
}
