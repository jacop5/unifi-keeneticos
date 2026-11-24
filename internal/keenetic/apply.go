// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// ApplyPlan journals the preimage before applying. The caller's health check
// must establish Wi-Fi operation and controller communication after changes.
// A journal surviving process termination is recovered at daemon startup.
func ApplyPlan(ctx context.Context, run Runner, plan Plan, journal string, health func(context.Context) error) error {
	return ApplyPlanWatched(ctx, run, plan, journal, health, nil)
}

func ApplyPlanWatched(ctx context.Context, run Runner, plan Plan, journal string, health func(context.Context) error, arm func(string) error) error {
	if health == nil {
		return fmt.Errorf("post-change health check is required")
	}
	b, err := run(ctx, "show running-config")
	if err != nil {
		return err
	}
	changes, err := Preimage(plan, string(b))
	if err != nil {
		return err
	}
	if len(enabledSchedules(plan)) > 0 {
		clock, err := run(ctx, "show clock date")
		if err != nil {
			return err
		}
		if _, err := RouterCivilTime(string(clock)); err != nil {
			return err
		}
	}
	return ApplyChangesWatched(ctx, run, changes, journal, health, arm)
}

// ApplyChangesWatched applies already-prepared native resource changes with
// the same durable journal and watchdog protocol as WLAN provisioning.
func ApplyChangesWatched(ctx context.Context, run Runner, changes []Change, journal string, health func(context.Context) error, arm func(string) error) error {
	if health == nil {
		return fmt.Errorf("post-change health check is required")
	}
	if len(changes) == 0 {
		if _, err := os.Stat(journal); err == nil {
			return fmt.Errorf("pending rollback journal exists")
		} else if !os.IsNotExist(err) {
			return err
		}
		// Idempotence is not evidence of WLAN operation or controller health.
		// Nothing changed, so a failed check needs neither journal nor rollback.
		if err := health(ctx); err != nil {
			return fmt.Errorf("unchanged plan health check failed: %w", err)
		}
		return nil
	}
	var owner journalOwner
	if err := saveUndo(journal, changes, &owner); err != nil {
		return err
	}
	if arm != nil {
		if err := arm(journal); err != nil {
			return errors.Join(fmt.Errorf("could not arm rollback watchdog"), owner.clearBefore(time.Time{}))
		}
	}
	tx, err := Begin(ctx, owner.runner(run), changes, 60*time.Second)
	if err != nil {
		// Retry from the persisted journal, including inverses that might have
		// failed during the transaction's immediate rollback.
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return errors.Join(err, owner.recover(recovery, run))
	}
	if err := health(ctx); err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr == nil {
			rollbackErr = owner.clearBefore(time.Time{})
		}
		return errors.Join(fmt.Errorf("post-change health check failed: %w", err), rollbackErr)
	}
	return tx.confirm(func() error { return owner.clearBefore(tx.deadline) })
}
