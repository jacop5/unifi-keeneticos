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
	err := ApplyPlan(context.Background(), run, Plan{Commands: []Command{{Text: "interface WifiMaster0 channel 6"}}}, path, func(context.Context) error { return errors.New("controller unreachable") })
	if err == nil {
		t.Fatal("expected health failure")
	}
	if len(calls) != 3 || calls[2] != "interface WifiMaster0 channel 11" {
		t.Fatal(calls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("journal remains after successful rollback")
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
