// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Change pairs a desired step with recovery from the router's preimage.
// New resources recover as a lifecycle (ending in deletion); their ownership
// marker remains until that deletion succeeds. Prepare before any mutation.
type Change struct{ Forward, Undo Command }

// Transaction serializes confirmation and rollback. Its deadline is independent
// of inform HTTP requests; loss of the controller cannot cancel the watchdog.
type Transaction struct {
	mu          sync.Mutex
	run         Runner
	changes     []Change
	applied     int
	timer       *time.Timer
	done        bool
	rollbackErr error
	deadline    time.Time
}

func Begin(ctx context.Context, run Runner, changes []Change, timeout time.Duration) (*Transaction, error) {
	if len(changes) == 0 || timeout <= 0 {
		return nil, fmt.Errorf("empty transaction or invalid timeout")
	}
	for _, c := range changes {
		if c.Forward.Text == "" || c.Undo.Text == "" {
			return nil, fmt.Errorf("change has no inverse")
		}
	}
	t := &Transaction{run: run, changes: append([]Change(nil), changes...), deadline: time.Now().Add(timeout)}
	t.mu.Lock()
	t.timer = time.AfterFunc(timeout, func() { _ = t.Rollback() })
	for i, c := range t.changes {
		// Include the current command in rollback: a failed execution may still
		// have partially changed router state.
		t.applied = i + 1
		if _, err := run(ctx, c.Forward.Text); err != nil {
			rollbackErr := t.rollbackLocked()
			t.mu.Unlock()
			return t, errors.Join(fmt.Errorf("configuration command %d failed", i+1), rollbackErr)
		}
	}
	t.mu.Unlock()
	return t, nil
}

func (t *Transaction) rollbackLocked() error {
	if t.done {
		return t.rollbackErr
	}
	if t.timer != nil {
		t.timer.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var errs []error
	guardCache := map[string]bool{}
	for i := t.applied - 1; i >= 0; i-- {
		if err := runUndo(ctx, t.run, t.changes[i].Undo, guardCache); err != nil {
			errs = append(errs, fmt.Errorf("rollback command %d failed", i+1))
		}
	}
	t.rollbackErr = errors.Join(errs...)
	t.done = true
	return t.rollbackErr
}
func (t *Transaction) Rollback() error { t.mu.Lock(); defer t.mu.Unlock(); return t.rollbackLocked() }
func (t *Transaction) Confirm() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return errors.Join(fmt.Errorf("transaction already rolled back"), t.rollbackErr)
	}
	if !time.Now().Before(t.deadline) {
		return errors.Join(fmt.Errorf("confirmation deadline expired"), t.rollbackLocked())
	}
	t.timer.Stop()
	t.done = true
	return nil
}
