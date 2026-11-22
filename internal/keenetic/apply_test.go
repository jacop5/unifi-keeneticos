// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWatchdogFailurePreventsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	run := func(_ context.Context, c string) ([]byte, error) {
		if c != "show running-config" {
			t.Fatal("mutation without watchdog")
		}
		return []byte("interface WifiMaster0\n    channel 11\n!\n"), nil
	}
	err := ApplyPlanWatched(context.Background(), run, Plan{Commands: []Command{{Text: "interface WifiMaster0 channel 6"}}}, path, func(context.Context) error { return nil }, func(string) error { return errors.New("cannot start") })
	if err == nil {
		t.Fatal("expected watchdog error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unused journal not cleared")
	}
}

func TestApplyHealthFailureRestoresPreimage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	var calls []string
	run := func(_ context.Context, c string) ([]byte, error) {
		calls = append(calls, c)
		if c == "show running-config" {
			return []byte("interface WifiMaster0\n    channel 11\n!\n"), nil
		}
		return nil, nil
	}
	healthErr := errors.New("controller unreachable")
	err := ApplyPlan(context.Background(), run, Plan{Commands: []Command{{Text: "interface WifiMaster0 channel 6"}}}, path, func(context.Context) error { return healthErr })
	if !errors.Is(err, healthErr) {
		t.Fatal("health failure cause was lost")
	}
	if len(calls) != 3 || calls[2] != "interface WifiMaster0 channel 11" {
		t.Fatal(calls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("journal remains after successful rollback")
	}
}

func TestNoopCannotConfirmWhileRecoveryIsPending(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "rollback.json")
	if err := SaveUndo(journal, []Change{{Forward: Command{Text: "new"}, Undo: Command{Text: "old"}}}); err != nil {
		t.Fatal(err)
	}
	err := ApplyChangesWatched(context.Background(), func(context.Context, string) ([]byte, error) {
		t.Fatal("no-op pending recovery invoked native commands")
		return nil, nil
	}, nil, journal, func(context.Context) error { return nil }, nil)
	if err == nil {
		t.Fatal("no-op was confirmed while a watchdog could still restore old settings")
	}
	if _, err := os.Stat(journal); err != nil {
		t.Fatal("pending recovery journal was discarded")
	}
}
func TestApplyJournalsBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	run := func(_ context.Context, c string) ([]byte, error) {
		if c == "show running-config" {
			return []byte("interface WifiMaster0\n    channel 11\n!\n"), nil
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("mutation before journal")
		}
		return nil, nil
	}
	if err := ApplyPlan(context.Background(), run, Plan{Commands: []Command{{Text: "interface WifiMaster0 channel 6"}}}, path, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("journal remains after confirmation")
	}
}
