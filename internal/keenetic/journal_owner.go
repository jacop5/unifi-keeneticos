// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// A live transaction may execute only while its exact durable journal remains
// pending. Recovery holds the same lock for the complete inverse sequence.
type journalOwner struct {
	path   string
	digest [sha256.Size]byte
}

func (o journalOwner) check() error {
	b, err := os.ReadFile(o.path)
	if err != nil {
		return fmt.Errorf("rollback journal is no longer owned: %w", err)
	}
	if sha256.Sum256(b) != o.digest {
		return fmt.Errorf("rollback journal belongs to another transaction")
	}
	return nil
}

func (o journalOwner) runner(run Runner) Runner {
	return func(ctx context.Context, command string) ([]byte, error) {
		lock, err := lockJournal(ctx, o.path)
		if err != nil {
			return nil, err
		}
		defer lock.Close()
		if err := o.check(); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return run(ctx, command)
	}
}

func (o journalOwner) clearBefore(deadline time.Time) error {
	return o.clear(context.Background(), deadline)
}

func (o journalOwner) clear(parent context.Context, deadline time.Time) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if !deadline.IsZero() {
		var deadlineCancel context.CancelFunc
		ctx, deadlineCancel = context.WithDeadline(ctx, deadline)
		defer deadlineCancel()
	}
	lock, err := lockJournal(ctx, o.path)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := o.check(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return clearUndo(o.path)
}

func (o journalOwner) recover(ctx context.Context, run Runner) error {
	return RecoverUndoMatching(ctx, o.path, hex.EncodeToString(o.digest[:]), run)
}
