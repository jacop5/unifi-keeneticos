// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnchangedPlanStillChecksWLANOperation(t *testing.T) {
	const ap = "WifiMaster0/AccessPoint6"
	p := Plan{Commands: []Command{{Text: "interface " + ap + " up"}}}
	running := "interface " + ap + "\n    ssid Test\n    up\n!\n"
	for _, active := range []bool{false, true} {
		journal := filepath.Join(t.TempDir(), "rollback.json")
		checks := 0
		run := func(_ context.Context, command string) ([]byte, error) {
			if command != "show running-config" {
				t.Fatal("unchanged plan mutated native configuration")
			}
			return []byte(running), nil
		}
		err := ApplyPlanWatched(context.Background(), run, p, journal, func(context.Context) error {
			checks++
			s := &Snapshot{}
			if active {
				s.VAPs = []map[string]string{{"id": ap, "ssid": "Test"}}
			}
			return VerifyWLANs(p, s)
		}, func(string) error { t.Fatal("unchanged plan armed a watchdog"); return nil })
		if (err == nil) != active || checks != 1 {
			t.Fatalf("active=%v checks=%d error=%v", active, checks, err)
		}
		if _, err := os.Stat(journal); !os.IsNotExist(err) {
			t.Fatal("unchanged plan wrote a journal")
		}
	}
}

func TestUnchangedPlanHealthFailurePreservesRetryIdentity(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "rollback.json")
	err := ApplyChangesWatched(context.Background(), nil, nil, journal, func(context.Context) error { return context.DeadlineExceeded }, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("health deadline no longer classifiable for retry")
	}
}

func TestUnchangedPlanDoesNotCheckHealthDuringPendingRecovery(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "rollback.json")
	if err := os.WriteFile(journal, []byte("existing recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	err := ApplyChangesWatched(context.Background(), nil, nil, journal, func(context.Context) error { t.Fatal("checked health before resolving pending recovery"); return nil }, nil)
	if err == nil {
		t.Fatal("pending recovery ignored")
	}
	b, _ := os.ReadFile(journal)
	if string(b) != "existing recovery" {
		t.Fatal("pending recovery overwritten")
	}
}
