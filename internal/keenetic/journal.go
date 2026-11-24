// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type rollbackJournal struct {
	ID       string
	Commands []Command
}

// Keep this inode across transactions: removing a lock file permits two owners
// to lock different inodes under the same name. The kernel releases it on exit.
func lockJournal(ctx context.Context, path string) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("invalid rollback lock file")
	}
	for {
		if err := ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN && err != unix.EINTR {
			f.Close()
			return nil, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			f.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// SaveUndo must finish before the first router mutation. The private journal
// contains WLAN keys and is removed only after confirmation or full recovery.
func SaveUndo(path string, changes []Change) error {
	if len(changes) == 0 {
		return fmt.Errorf("empty rollback journal")
	}
	for _, change := range changes {
		if strings.TrimSpace(change.Undo.Text) == "" {
			return fmt.Errorf("empty rollback command")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lock, err := lockJournal(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("pending rollback journal exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	var undo []Command
	for i := len(changes) - 1; i >= 0; i-- {
		undo = append(undo, changes[i].Undo)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	b, err := json.Marshal(rollbackJournal{ID: hex.EncodeToString(nonce[:]), Commands: undo})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return syncDir(path)
}

func syncDir(path string) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func ClearUndo(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lock, err := lockJournal(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	return clearUndo(path)
}

func clearUndo(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDir(path)
}

// RecoverUndo is idempotent. A failed recovery retains the complete journal
// so the next start retries every inverse in the same order.
func RecoverUndo(ctx context.Context, path string, run Runner) error {
	return recoverUndo(ctx, path, "", run)
}

// RecoverUndoMatching is for detached watchers. Identity comparison, replay and
// journal removal share one lock, so a stale watcher cannot recover a newer
// transaction after waiting behind startup recovery or another watcher.
func RecoverUndoMatching(ctx context.Context, path, digest string, run Runner) error {
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("invalid rollback identity")
	}
	return recoverUndo(ctx, path, digest, run)
}

func recoverUndo(ctx context.Context, path, digest string, run Runner) error {
	lock, err := lockJournal(ctx, path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if digest != "" {
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != digest {
			return nil
		}
	}
	var journal rollbackJournal
	if err = json.Unmarshal(b, &journal); err != nil || journal.ID == "" || len(journal.Commands) == 0 {
		return fmt.Errorf("invalid rollback journal")
	}
	// Validate the entire sequence before executing any inverse. A malformed
	// later entry must not leave the router partially recovered.
	for _, c := range journal.Commands {
		if strings.TrimSpace(c.Text) == "" {
			return fmt.Errorf("empty rollback command")
		}
	}
	guardCache := map[string]bool{}
	for i, c := range journal.Commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := runUndo(ctx, run, c, guardCache); err != nil {
			return fmt.Errorf("rollback command %d failed", i+1)
		}
	}
	return clearUndo(path)
}
