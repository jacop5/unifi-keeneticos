// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestFailedApplyReversesPartialCommand(t *testing.T) {
	var commands []string
	run := func(_ context.Context, c string) ([]byte, error) {
		commands = append(commands, c)
		if c == "set2" {
			return nil, errors.New("secret must not appear in returned error")
		}
		return nil, nil
	}
	_, err := Begin(context.Background(), run, []Change{{Forward: Command{Text: "set1"}, Undo: Command{Text: "undo1"}}, {Forward: Command{Text: "set2"}, Undo: Command{Text: "undo2"}}}, time.Hour)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !reflect.DeepEqual(commands, []string{"set1", "set2", "undo2", "undo1"}) {
		t.Fatal(commands)
	}
}
func TestConfirmedTransactionDoesNotRollback(t *testing.T) {
	calls := 0
	run := func(context.Context, string) ([]byte, error) { calls++; return nil, nil }
	tx, err := Begin(context.Background(), run, []Change{{Forward: Command{Text: "set"}, Undo: Command{Text: "undo"}}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Confirm(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestLateConfirmationCannotBeatWatchdog(t *testing.T) {
	rolledBack := false
	tx := &Transaction{run: func(_ context.Context, c string) ([]byte, error) { rolledBack = c == "undo"; return nil, nil }, changes: []Change{{Undo: Command{Text: "undo"}}}, applied: 1, deadline: time.Now().Add(-time.Second)}
	if err := tx.Confirm(); err == nil {
		t.Fatal("late confirmation accepted")
	}
	if !rolledBack {
		t.Fatal("expired transaction not rolled back")
	}
}
