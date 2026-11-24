// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestConcurrentRecoveryWaitsAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	if err := SaveUndo(path, []Change{{Undo: Command{Text: "restore"}}}); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	defer func() {
		close(release)
		select {
		case err := <-done:
			if err == nil {
				t.Error("injected failure unexpectedly succeeded")
			}
		case <-time.After(5 * time.Second):
			t.Error("recovery did not exit")
		}
	}()
	go func() {
		done <- RecoverUndo(context.Background(), path, func(context.Context, string) ([]byte, error) {
			close(entered)
			<-release
			return nil, errors.New("injected failure retains journal")
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := RecoverUndo(ctx, path, func(context.Context, string) ([]byte, error) {
		t.Error("second recovery overlapped the first")
		return nil, nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock wait: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("waiting recovery removed journal")
	}
}

func TestJournalLockAcrossProcesses(t *testing.T) {
	if path := os.Getenv("UKN_JOURNAL_LOCK_HELPER"); path != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		lock, err := lockJournal(ctx, path)
		if os.Getenv("UKN_EXPECT_LOCKED") == "1" {
			if lock != nil {
				lock.Close()
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("other process lock ignored: %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			lock.Close()
		}
		return
	}
	path := filepath.Join(t.TempDir(), "rollback.json")
	lock, err := lockJournal(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, locked := range []string{"1", "0"} {
		if locked == "0" {
			if err := lock.Close(); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestJournalLockAcrossProcesses$")
		cmd.Env = append(os.Environ(), "UKN_JOURNAL_LOCK_HELPER="+path, "UKN_EXPECT_LOCKED="+locked)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("locked=%s: %v %s", locked, err, out)
		}
	}
}

func TestStaleWatcherChecksIdentityAfterAcquiringLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	changes := []Change{{Undo: Command{Text: "restore"}}}
	if err := SaveUndo(path, changes); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	id := sha256.Sum256(old)
	if err := ClearUndo(path); err != nil {
		t.Fatal(err)
	}
	if err := SaveUndo(path, changes); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Even a stale identity must wait for the lock rather than reading a file
	// that another owner may still be replacing.
	lock, err := lockJournal(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = RecoverUndoMatching(ctx, path, hex.EncodeToString(id[:]), func(context.Context, string) ([]byte, error) {
		t.Error("watcher entered locked recovery")
		return nil, nil
	})
	cancel()
	lock.Close()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("identity checked outside lock: %v", err)
	}
	if err := RecoverUndoMatching(context.Background(), path, hex.EncodeToString(id[:]), func(context.Context, string) ([]byte, error) {
		t.Error("stale watcher replayed a newer transaction")
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(path)
	if err != nil || string(retained) != string(current) {
		t.Fatal("stale watcher changed newer journal")
	}
	id = sha256.Sum256(current)
	calls := 0
	if err := RecoverUndoMatching(context.Background(), path, hex.EncodeToString(id[:]), func(context.Context, string) ([]byte, error) {
		calls++
		return nil, nil
	}); err != nil || calls != 1 {
		t.Fatalf("current watcher: %v calls=%d", err, calls)
	}
}

func TestJournalLockPersistsAndRejectsLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.json")
	if err := SaveUndo(path, []Change{{Undo: Command{Text: "restore"}}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path + ".lock")
	if err != nil || before.Mode().Perm() != 0600 {
		t.Fatal("lock is not private")
	}
	if err := ClearUndo(path); err != nil {
		t.Fatal(err)
	}
	if err := RecoverUndo(context.Background(), path, nil); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path + ".lock")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("lock inode replaced")
	}
	if err := os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(external, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path+".lock"); err != nil {
		t.Fatal(err)
	}
	if err := SaveUndo(path, []Change{{Undo: Command{Text: "restore"}}}); err == nil {
		t.Fatal("followed lock symlink")
	}
	contents, err := os.ReadFile(external)
	if err != nil || string(contents) != "unchanged" {
		t.Fatal("modified external file")
	}
}
