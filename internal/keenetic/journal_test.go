// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "undo.json")
	changes := []Change{{Undo: Command{Text: "undo1"}}, {Undo: Command{Text: "undo2", Secret: true}}}
	if err := SaveUndo(path, changes); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if err := SaveUndo(path, changes); err == nil {
		t.Fatal("overwrote pending journal")
	}
	if err := RecoverUndo(context.Background(), path, func(context.Context, string) ([]byte, error) { return nil, errors.New("failed") }); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("lost recovery journal")
	}
	var calls []string
	if err := RecoverUndo(context.Background(), path, func(_ context.Context, c string) ([]byte, error) { calls = append(calls, c); return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "undo2" || calls[1] != "undo1" {
		t.Fatal(calls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("journal not cleared")
	}
}

func TestMalformedJournalDoesNotPartiallyRecover(t *testing.T) {
	for _, data := range []string{
		`{"ID":"fixture","Commands":[{"Text":"undo-first"},{"Text":""}]}`,
		`{"ID":"fixture","Commands":[{"Text":"undo-first"},{"Text":"  \t"}]}`,
		`{"ID":"fixture","Commands":[{"Text":"undo-first"},{}]}`,
		`{"ID":"fixture","Commands":[{"Text":"undo-first"}`,
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "undo.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			err := RecoverUndo(context.Background(), path, func(context.Context, string) ([]byte, error) {
				calls++
				return nil, nil
			})
			if err == nil || calls != 0 {
				t.Fatalf("invalid journal executed %d commands: %v", calls, err)
			}
			retained, err := os.ReadFile(path)
			if err != nil || string(retained) != data {
				t.Fatal("invalid journal not retained unchanged")
			}
		})
	}
}

func TestSaveUndoRejectsUnrecoverableSequence(t *testing.T) {
	for _, changes := range [][]Change{nil, {{Undo: Command{Text: "undo"}}, {Undo: Command{Text: " \t"}}}} {
		path := filepath.Join(t.TempDir(), "undo.json")
		if err := SaveUndo(path, changes); err == nil {
			t.Fatal("invalid undo accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid sequence created a journal")
		}
	}
}

func TestInterruptedRecoveryRetainsSequenceForNextStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "undo.json")
	changes := []Change{{Undo: Command{Text: "restore-final"}}, {Undo: Command{Text: "restore-first"}}}
	if err := SaveUndo(path, changes); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err = RecoverUndo(ctx, path, func(ctx context.Context, command string) ([]byte, error) {
		calls++
		if command == "restore-first" {
			cancel()
			return nil, nil
		}
		return nil, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("interruption not retained: %v calls=%d", err, calls)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("lost recovery sequence")
	}
	var retried []string
	if err := RecoverUndo(context.Background(), path, func(_ context.Context, command string) ([]byte, error) {
		retried = append(retried, command)
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(retried) != 2 || retried[0] != "restore-first" || retried[1] != "restore-final" {
		t.Fatal(retried)
	}
	if err := SaveUndo(path, changes); err != nil {
		t.Fatal(err)
	}
	next, err := os.ReadFile(path)
	if err != nil || bytes.Equal(next, before) {
		t.Fatal("identical changes reused the old journal identity")
	}
}
