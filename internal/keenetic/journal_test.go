// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
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
