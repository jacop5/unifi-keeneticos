// SPDX-License-Identifier: GPL-2.0-or-later
// Package installation inspects retained installer data without recovering it.
package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"
)

type File struct {
	Target  string `json:"target"`
	Backup  string `json:"backup"`
	Current string `json:"current"`
}

type Transaction struct {
	Phase         string `json:"phase"`
	ServiceBefore string `json:"service_before"`
	MetadataValid bool   `json:"metadata_valid"`
	Files         []File `json:"files"`
}

type Status struct {
	SchemaVersion          int           `json:"schema_version"`
	Pending                bool          `json:"pending_markers"`
	LockPresent            bool          `json:"lock_present"`
	LockMatchesTransaction bool          `json:"lock_matches_transaction"`
	Transactions           []Transaction `json:"transactions"`
	UnknownMarkers         int           `json:"unknown_markers"`
}

var targets = map[string]bool{
	"etc/init.d/S99unifi-keenetic":                            true,
	"sbin/ukn":                                                true,
	"etc/unifi-keenetic.conf":                                 true,
	"share/licenses/unifi-keeneticos/LICENSE":                 true,
	"share/licenses/unifi-keeneticos/NOTICE":                  true,
	"share/licenses/unifi-keeneticos/Go-BSD-3-Clause.txt":     true,
	"share/licenses/unifi-keeneticos/model-catalogue-MIT.txt": true,
}

// Inspect reports a best-effort snapshot, never permission to start or restore.
// OpenRoot confines even concurrently replaced paths to the requested tree.
// Child symlinks are rejected; the Entware root itself may be a mount/symlink.
// Errors from files are reduced to fixed statuses, not private path contents.
func Inspect(directory string) (Status, error) {
	s := Status{SchemaVersion: 1, Transactions: []Transaction{}}
	r, err := os.OpenRoot(directory)
	if err != nil {
		return s, err
	}
	defer r.Close()
	d, err := r.Open(".")
	if err != nil {
		return s, err
	}
	entries, err := d.ReadDir(-1)
	d.Close()
	if err != nil {
		return s, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	lockPointer := ""
	for _, e := range entries {
		if e.Name() == ".ukn-install.lock" {
			s.LockPresent = true
			lockPointer, _ = read(r, e.Name()+"/work-directory")
		}
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, ".ukn-install.") {
			continue
		}
		s.Pending = true
		if name == ".ukn-install.lock" {
			continue
		}
		if !e.IsDir() || !transactionName(name) {
			s.UnknownMarkers++
			continue
		}
		if lockPointer == path.Join(directory, name) {
			s.LockMatchesTransaction = true
		}
		s.Transactions = append(s.Transactions, inspectTransaction(r, name))
	}
	return s, nil
}

func transactionName(name string) bool {
	suffix := strings.TrimPrefix(name, ".ukn-install.")
	if len(suffix) != 6 {
		return false
	}
	for _, c := range suffix {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func safe(r *os.Root, name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := r.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return os.ErrPermission
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return os.ErrInvalid
		}
	}
	return nil
}

func read(r *os.Root, name string) (string, error) {
	if err := safe(r, name); err != nil {
		return "", err
	}
	f, err := r.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return "", os.ErrInvalid
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return "", err
	}
	if len(b) > 65536 {
		return "", os.ErrInvalid
	}
	return strings.TrimSpace(string(b)), nil
}

func hashes(r *os.Root, name string) (map[string]string, bool) {
	text, err := read(r, name)
	if err != nil {
		return nil, false
	}
	out := map[string]string{}
	if text == "" {
		return out, true
	}
	for _, line := range strings.Split(text, "\n") {
		p := strings.Fields(line)
		if len(p) != 2 || !targets[p[1]] || out[p[1]] != "" || len(p[0]) != 64 {
			return nil, false
		}
		if _, err := hex.DecodeString(p[0]); err != nil {
			return nil, false
		}
		out[p[1]] = strings.ToLower(p[0])
	}
	return out, true
}

func digest(r *os.Root, name string) (string, string) {
	if err := safe(r, name); err != nil {
		if os.IsNotExist(err) {
			return "", "absent"
		}
		return "", "unsafe_or_unreadable"
	}
	f, err := r.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", "unsafe_or_unreadable"
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return "", "unsafe_or_unreadable"
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, (128<<20)+1))
	if err != nil || n > 128<<20 {
		return "", "unsafe_or_unreadable"
	}
	return hex.EncodeToString(h.Sum(nil)), "read"
}

func inspectTransaction(r *os.Root, dir string) Transaction {
	t := Transaction{Phase: "unknown", ServiceBefore: "unknown", Files: []File{}}
	phase, pe := read(r, dir+"/phase")
	service, se := read(r, dir+"/service-before")
	switch phase {
	case "prepared", "stopping", "stopped", "committing", "installed", "restored":
		t.Phase = phase
	}
	switch service {
	case "absent", "stopped", "running":
		t.ServiceBefore = service
	}
	listed, err := read(r, dir+"/targets")
	old, okOld := hashes(r, dir+"/backup-hashes")
	newHashes, okNew := hashes(r, dir+"/new-hashes")
	if pe != nil || se != nil || t.Phase == "unknown" || t.ServiceBefore == "unknown" || err != nil || !okOld || !okNew {
		return t
	}
	seen := map[string]bool{}
	order := strings.Split(listed, "\n")
	for _, target := range order {
		if !targets[target] || seen[target] || newHashes[target] == "" {
			return t
		}
		seen[target] = true
	}
	for target := range old {
		if !seen[target] {
			return t
		}
	}
	if len(seen) != len(newHashes) {
		return t
	}
	if order[0] != "etc/init.d/S99unifi-keenetic" {
		return t
	}
	for target := range targets {
		if target != "etc/unifi-keenetic.conf" && !seen[target] {
			return t
		}
	}
	t.MetadataValid = true
	for _, target := range order {
		f := File{Target: target}
		backupHash, backupState := digest(r, dir+"/backup/"+target)
		markerHash, markerState := digest(r, dir+"/backup/"+target+".absent")
		empty := sha256.Sum256(nil)
		if old[target] != "" {
			switch {
			case markerState != "absent":
				f.Backup = "conflicting_or_unsafe_marker"
			case backupState != "read":
				f.Backup = backupState
			case backupHash != old[target]:
				f.Backup = "hash_mismatch"
			default:
				f.Backup = "verified"
			}
		} else if backupState == "absent" && markerState == "read" && markerHash == hex.EncodeToString(empty[:]) {
			f.Backup = "originally_absent"
		} else {
			f.Backup = "invalid_absence_record"
		}
		current, state := digest(r, target)
		switch {
		case state != "read":
			f.Current = state
		case current == newHashes[target] && current == old[target]:
			f.Current = "matches_both"
		case current == newHashes[target]:
			f.Current = "matches_new"
		case old[target] != "" && current == old[target]:
			f.Current = "matches_backup"
		default:
			f.Current = "hash_mismatch"
		}
		t.Files = append(t.Files, f)
	}
	return t
}
