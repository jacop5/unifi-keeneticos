// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// ApplyPlan journals the preimage before applying. The caller's health check
// must establish Wi-Fi operation and controller communication after changes.
// A journal surviving process termination is recovered at daemon startup.
func ApplyPlan(ctx context.Context, run Runner, plan Plan, journal string, health func(context.Context) error) error {
	return ApplyPlanWatched(ctx, run, plan, journal, health, nil)
}

func ApplyPlanWatched(ctx context.Context, run Runner, plan Plan, journal string, health func(context.Context) error, arm func(string) error) error {
	return ApplyPlanWatchedWithReceipts(ctx, run, plan, journal, health, arm, nil)
}

// Receipts can omit only previously confirmed credential writes. The original
// plan is still checked by the caller's health callback, including on a no-op.
func ApplyPlanWatchedWithReceipts(ctx context.Context, run Runner, plan Plan, journal string, health func(context.Context) error, arm func(string) error, receipts *CredentialReceipts) error {
	if health == nil {
		return fmt.Errorf("post-change health check is required")
	}
	b, err := run(ctx, "show running-config")
	if err != nil {
		return err
	}
	prepared := plan
	entries := map[string]credentialBinding{}
	if receipts != nil {
		entries = receipts.read()
		prepared = receipts.filter(plan, string(b), entries)
	}
	changes, err := Preimage(prepared, string(b))
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
	written := map[string]credentialRequest{}
	reused := map[string]credentialRequest{}
	for target, request := range credentialRequests(plan) {
		present := false
		for _, c := range prepared.Commands {
			if c.Text == "interface "+target+" "+request.setting {
				present = true
			}
		}
		if !present {
			reused[target] = request
		}
		for _, change := range changes {
			if change.Forward.Secret && change.Forward.Text == "interface "+target+" "+request.setting {
				written[target] = request
			}
		}
	}
	pending := map[string]credentialBinding{}
	checkedHealth := health
	if receipts != nil && (len(written) > 0 || len(reused) > 0) {
		checkedHealth = func(c context.Context) error {
			if err := health(c); err != nil {
				return err
			}
			if err := c.Err(); err != nil {
				return err
			}
			readCtx, cancel := context.WithTimeout(c, 3*time.Second)
			defer cancel()
			native, err := run(readCtx, "show running-config")
			if err != nil {
				if len(reused) > 0 {
					return fmt.Errorf("credential receipt readback unavailable: %w", err)
				}
				return nil
			}
			for target, request := range reused {
				binding, ok := receipts.binding(target, request, string(native))
				if !ok || !binding.matches(entries[target]) {
					return fmt.Errorf("credential receipt readback changed during health check")
				}
			}
			for target, request := range written {
				if binding, ok := receipts.binding(target, request, string(native)); ok {
					pending[target] = binding
				}
			}
			return nil
		}
	}
	if err := ApplyChangesWatched(ctx, run, changes, journal, checkedHealth, arm); err != nil {
		return err
	}
	if receipts != nil {
		changed := false
		for _, c := range plan.Commands {
			p := strings.SplitN(c.Text, " ", 3)
			if len(p) == 3 && p[0] == "interface" && p[2] == "no authentication wpa-psk" {
				if _, ok := entries[p[1]]; ok {
					delete(entries, p[1])
					changed = true
				}
			}
		}
		for target, binding := range pending {
			entries[target] = binding
			changed = true
		}
		if changed {
			if err := receipts.write(entries); err != nil {
				log.Printf("credential receipt persistence failed: %v", err)
			}
		}
	}
	return nil
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
		return ctx.Err()
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
	healthErr := health(ctx)
	if healthErr == nil {
		healthErr = ctx.Err()
	}
	if healthErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr == nil {
			rollbackErr = owner.clearBefore(time.Time{})
		}
		return errors.Join(fmt.Errorf("post-change health check failed: %w", healthErr), rollbackErr)
	}
	return tx.confirm(func() error { return owner.clear(ctx, tx.deadline) })
}
