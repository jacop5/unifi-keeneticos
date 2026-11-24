// SPDX-License-Identifier: GPL-2.0-or-later
package device

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestStateSaveIgnoresLegacyTempSymlinkAndRepairsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	other := filepath.Join(dir, "other-private-file")
	if err := os.WriteFile(other, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	want := State{Key: "synthetic-private-key", Adopted: true, CfgVersion: "preserved-version"}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, found, err := Load(path)
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Fatal("state did not round-trip")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("state file is not private")
	}
	b, err := os.ReadFile(other)
	if err != nil || string(b) != "untouched" {
		t.Fatal("followed legacy temporary symlink")
	}
	if target, err := os.Readlink(path + ".tmp"); err != nil || target != other {
		t.Fatal("changed unrelated legacy path")
	}
	staged, _ := filepath.Glob(filepath.Join(dir, ".state-*.tmp"))
	if len(staged) != 0 {
		t.Fatal("left temporary credentials behind")
	}
}

func TestFailedStateReplacementCleansPrivateStage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "keep")
	if err := os.WriteFile(marker, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (State{Key: "synthetic-private-key"}).Save(path); err == nil {
		t.Fatal("replaced a directory")
	}
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != "untouched" {
		t.Fatal("damaged previous target")
	}
	staged, _ := filepath.Glob(filepath.Join(dir, ".state-*.tmp"))
	if len(staged) != 0 {
		t.Fatal("failed replacement retained temporary credentials")
	}
}

func TestConcurrentStateSavesKeepCompletePrivateRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := (State{CfgVersion: "initial", Key: "key-initial"}).Save(path); err != nil {
		t.Fatal(err)
	}
	var writers sync.WaitGroup
	for i := 0; i < 8; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			version := fmt.Sprint(i)
			for j := 0; j < 2; j++ {
				if err := (State{CfgVersion: version, Key: "key-" + version}).Save(path); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	defer writers.Wait()
	done := make(chan struct{})
	go func() { writers.Wait(); close(done) }()
	for {
		got, found, err := Load(path)
		if err != nil || !found || got.Key != "key-"+got.CfgVersion {
			t.Fatal("read an incomplete state record")
		}
		select {
		case <-done:
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("concurrent save lost private permissions")
			}
			staged, _ := filepath.Glob(filepath.Join(dir, ".state-*.tmp"))
			if len(staged) != 0 {
				t.Fatal("concurrent saves retained stages")
			}
			return
		default:
		}
	}
}
