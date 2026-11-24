// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWatchdogRecoveryCannotBeConfirmedOrMutatedByOldTransaction(t *testing.T) {
	for _, mode := range []string{"recovered", "failed-recovery", "new-journal", "failed-health"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollback.json")
			var commands []string
			run := func(_ context.Context, command string) ([]byte, error) {
				commands = append(commands, command)
				if mode == "failed-recovery" && command == "undo" {
					return nil, errors.New("injected recovery failure")
				}
				return nil, nil
			}
			var replacement []byte
			err := ApplyChangesWatched(context.Background(), run, []Change{{Forward: Command{Text: "forward"}, Undo: Command{Text: "undo"}}}, path, func(context.Context) error {
				err := RecoverUndo(context.Background(), path, run)
				if (err != nil) != (mode == "failed-recovery") {
					t.Fatalf("recovery: %v", err)
				}
				if mode == "new-journal" {
					if err := SaveUndo(path, []Change{{Undo: Command{Text: "new-owner-undo"}}}); err != nil {
						t.Fatal(err)
					}
					replacement, err = os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				if mode == "failed-health" {
					return errors.New("injected health failure")
				}
				return nil
			}, nil)
			if err == nil {
				t.Fatal("recovered transaction reported confirmed")
			}
			if !reflect.DeepEqual(commands, []string{"forward", "undo"}) {
				t.Fatalf("old transaction continued: %v", commands)
			}
			if mode == "new-journal" {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(replacement) {
					t.Fatal("old confirmation touched new journal")
				}
			} else if mode == "failed-recovery" {
				b, err := os.ReadFile(path)
				var journal rollbackJournal
				if err != nil || json.Unmarshal(b, &journal) != nil || !journal.Recovering || len(journal.Commands) != 1 {
					t.Fatal("failed recovery was not retained and marked")
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("completed recovery journal remains")
			}
		})
	}
}

func TestRecoveredJournalBeforeBeginPreventsForwardCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	var commands []string
	run := func(_ context.Context, command string) ([]byte, error) {
		commands = append(commands, command)
		return nil, nil
	}
	err := ApplyChangesWatched(context.Background(), run, []Change{{Forward: Command{Text: "forward"}, Undo: Command{Text: "undo"}}}, path,
		func(context.Context) error { t.Error("health ran after lost ownership"); return nil },
		func(string) error { return RecoverUndo(context.Background(), path, run) })
	if err == nil || !reflect.DeepEqual(commands, []string{"undo"}) {
		t.Fatalf("commands=%v err=%v", commands, err)
	}
}

func TestRecoveryMarkerPrecedesNativeReplayAndRetainsRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	var owner journalOwner
	if err := saveUndo(path, []Change{{Undo: Command{Text: "restore"}}}, &owner); err != nil {
		t.Fatal(err)
	}
	err := RecoverUndo(context.Background(), path, func(context.Context, string) ([]byte, error) {
		b, err := os.ReadFile(path)
		var journal rollbackJournal
		if err != nil || json.Unmarshal(b, &journal) != nil || !journal.Recovering {
			t.Fatal("recovery executed before durable marker")
		}
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal("marked journal is not private")
		}
		return nil, errors.New("interrupted recovery")
	})
	if err == nil {
		t.Fatal("expected recovery failure")
	}
	if _, err := owner.runner(func(context.Context, string) ([]byte, error) {
		t.Error("lost owner executed native command")
		return nil, nil
	})(context.Background(), "forward"); err == nil {
		t.Fatal("owner remained valid after recovery began")
	}
	if err := owner.clearBefore(time.Time{}); err == nil {
		t.Fatal("lost owner cleared pending recovery")
	}
	if err := RecoverUndo(context.Background(), path, func(context.Context, string) ([]byte, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestNativeCommandExcludesWatchdogReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	var owner journalOwner
	if err := saveUndo(path, []Change{{Undo: Command{Text: "restore"}}}, &owner); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := owner.runner(func(context.Context, string) ([]byte, error) { close(entered); <-release; return nil, nil })(context.Background(), "forward")
		done <- err
	}()
	defer func() {
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("native fixture stuck")
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("native fixture never started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := RecoverUndo(ctx, path, func(context.Context, string) ([]byte, error) {
		t.Error("watchdog overlapped native change")
		return nil, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery did not wait: %v", err)
	}
}

func TestForwardDeadlineStopsSubsequentChanges(t *testing.T) {
	var commands []string
	_, err := Begin(context.Background(), func(ctx context.Context, command string) ([]byte, error) {
		commands = append(commands, command)
		if command == "first" {
			<-ctx.Done()
		}
		return nil, nil
	}, []Change{{Forward: Command{Text: "first"}, Undo: Command{Text: "undo-first"}}, {Forward: Command{Text: "second"}, Undo: Command{Text: "undo-second"}}}, 30*time.Millisecond)
	if err == nil || !reflect.DeepEqual(commands, []string{"first", "undo-first"}) {
		t.Fatalf("expired forward sequence continued: %v %v", commands, err)
	}
}
