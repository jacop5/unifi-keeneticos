// SPDX-License-Identifier: GPL-2.0-or-later
package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
)

func write(t *testing.T, root, name, text string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func sum(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := ".ukn-install.Ab12Cd"
	write(t, root, ".ukn-install.lock/work-directory", filepath.Join(root, dir)+"\n")
	write(t, root, dir+"/phase", "committing\n")
	write(t, root, dir+"/service-before", "running\n")
	order := []string{"etc/init.d/S99unifi-keenetic"}
	var rest []string
	for target := range targets {
		if target != order[0] && target != "etc/unifi-keenetic.conf" {
			rest = append(rest, target)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)
	var old, next strings.Builder
	for _, target := range order {
		write(t, root, dir+"/backup/"+target, "old-private-contents")
		write(t, root, target, "new-private-contents")
		old.WriteString(sum("old-private-contents") + "  " + target + "\n")
		next.WriteString(sum("new-private-contents") + "  " + target + "\n")
	}
	write(t, root, dir+"/backup-hashes", old.String())
	write(t, root, dir+"/new-hashes", next.String())
	write(t, root, dir+"/targets", strings.Join(order, "\n")+"\n")
	return root, dir
}

func TestInspectConsistentMixedAndChangedFiles(t *testing.T) {
	root, dir := fixture(t)
	write(t, root, "sbin/ukn", "old-private-contents")
	write(t, root, "share/licenses/unifi-keeneticos/NOTICE", "user-private-edit")
	s, err := Inspect(root)
	if err != nil || !s.Pending || !s.LockPresent || !s.LockMatchesTransaction || len(s.Transactions) != 1 {
		t.Fatalf("%+v %v", s, err)
	}
	tr := s.Transactions[0]
	if !tr.MetadataValid || tr.Phase != "committing" || tr.ServiceBefore != "running" {
		t.Fatalf("%+v", tr)
	}
	for _, f := range tr.Files {
		want := "matches_new"
		if f.Target == "sbin/ukn" {
			want = "matches_backup"
		}
		if strings.HasSuffix(f.Target, "/NOTICE") {
			want = "hash_mismatch"
		}
		if f.Backup != "verified" || f.Current != want {
			t.Fatalf("%+v", f)
		}
	}
	b, _ := json.Marshal(s)
	for _, secret := range []string{"old-private-contents", "new-private-contents", "user-private-edit", sum("old-private-contents"), dir, root} {
		if strings.Contains(string(b), secret) {
			t.Fatal("private data in report")
		}
	}
}

func TestInspectAbsenceAndCorruption(t *testing.T) {
	root, dir := fixture(t)
	target := "sbin/ukn"
	oldFile := filepath.Join(root, dir, "backup-hashes")
	b, _ := os.ReadFile(oldFile)
	write(t, root, dir+"/backup-hashes", strings.ReplaceAll(string(b), sum("old-private-contents")+"  "+target+"\n", ""))
	if err := os.Remove(filepath.Join(root, dir, "backup", target)); err != nil {
		t.Fatal(err)
	}
	write(t, root, dir+"/backup/"+target+".absent", "")
	write(t, root, dir+"/backup/share/licenses/unifi-keeneticos/NOTICE", "corrupt")
	s, err := Inspect(root)
	if err != nil || !s.Transactions[0].MetadataValid {
		t.Fatal(err)
	}
	for _, f := range s.Transactions[0].Files {
		if f.Target == target && f.Backup != "originally_absent" {
			t.Fatalf("%+v", f)
		}
		if strings.HasSuffix(f.Target, "/NOTICE") && f.Backup != "hash_mismatch" {
			t.Fatalf("%+v", f)
		}
	}
}

func TestInspectMalformedMetadataDoesNotFollowPaths(t *testing.T) {
	for _, change := range []string{"traversal", "duplicate", "missing-hash", "missing-target", "phase-secret", "symlink", "fifo", "oversized"} {
		t.Run(change, func(t *testing.T) {
			root, dir := fixture(t)
			switch change {
			case "traversal":
				write(t, root, dir+"/targets", "../../private-secret\n")
			case "duplicate":
				b, _ := os.ReadFile(filepath.Join(root, dir, "targets"))
				write(t, root, dir+"/targets", string(b)+"sbin/ukn\n")
			case "missing-hash":
				write(t, root, dir+"/new-hashes", "")
			case "missing-target":
				write(t, root, dir+"/targets", "etc/init.d/S99unifi-keenetic\n")
			case "phase-secret":
				write(t, root, dir+"/phase", "private-secret")
			case "symlink", "fifo":
				p := filepath.Join(root, dir, "phase")
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				if change == "symlink" {
					if err := os.Symlink("/dev/zero", p); err != nil {
						t.Fatal(err)
					}
				} else if err := syscall.Mkfifo(p, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				write(t, root, dir+"/phase", strings.Repeat("s", 65537))
			}
			s, err := Inspect(root)
			if err != nil || len(s.Transactions) != 1 || s.Transactions[0].MetadataValid || len(s.Transactions[0].Files) != 0 {
				t.Fatalf("%+v %v", s, err)
			}
			b, _ := json.Marshal(s)
			if strings.Contains(string(b), "private-secret") {
				t.Fatal("untrusted metadata echoed")
			}
		})
	}
}

func TestInspectRejectsBackupAndCurrentSymlinks(t *testing.T) {
	root, dir := fixture(t)
	for _, name := range []string{"sbin/ukn", dir + "/backup/sbin/ukn"} {
		p := filepath.Join(root, name)
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/dev/zero", p); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range s.Transactions[0].Files {
		if f.Target == "sbin/ukn" && (f.Backup != "unsafe_or_unreadable" || f.Current != "unsafe_or_unreadable") {
			t.Fatalf("%+v", f)
		}
	}
}

func TestInspectEmptyAndUnknownMarkers(t *testing.T) {
	root := t.TempDir()
	before, _ := os.ReadDir(root)
	s, err := Inspect(root)
	after, _ := os.ReadDir(root)
	if err != nil || s.Pending || !reflect.DeepEqual(before, after) {
		t.Fatalf("%+v %v", s, err)
	}
	if err := os.Symlink("/nonexistent", filepath.Join(root, ".ukn-install.Ab12Cd")); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".ukn-install.unknown-format", "private")
	s, err = Inspect(root)
	if err != nil || !s.Pending || s.UnknownMarkers != 2 || len(s.Transactions) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}
