// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type rollbackJournal struct {
	ID       string
	Commands []Command
}

// SaveUndo must finish before the first router mutation. The private journal
// contains WLAN keys and is removed only after confirmation or full recovery.
func SaveUndo(path string, changes []Change) error {
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
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDir(path)
}

// RecoverUndo is idempotent. A failed recovery retains the complete journal
// so the next start retries every inverse in the same order.
func RecoverUndo(ctx context.Context, path string, run Runner) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal rollbackJournal
	if err = json.Unmarshal(b, &journal); err != nil || journal.ID == "" || len(journal.Commands) == 0 {
		return fmt.Errorf("invalid rollback journal")
	}
	guardCache := map[string]bool{}
	for i, c := range journal.Commands {
		if c.Text == "" {
			return fmt.Errorf("empty rollback command")
		}
		if err := runUndo(ctx, run, c, guardCache); err != nil {
			return fmt.Errorf("rollback command %d failed", i+1)
		}
	}
	return ClearUndo(path)
}
