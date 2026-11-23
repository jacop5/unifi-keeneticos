// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerIntegrityGateRejectsCorruptionAndMissingHashes(t *testing.T) {
	script, err := os.ReadFile("../../packaging/keenetic/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	start := strings.Index(text, "command -v sha256sum")
	end := strings.Index(text, "for file in LICENSE NOTICE")
	if start < 0 || end <= start {
		t.Fatal("installer integrity gate not found")
	}
	gate := text[start:end]
	files := []string{"ukn", "install.sh", "space-check.sh", "S99unifi-keenetic", "unifi-keenetic.conf", "LICENSE", "NOTICE", "licenses/Go-BSD-3-Clause.txt", "licenses/model-catalogue-MIT.txt"}
	for _, mode := range []string{"valid", "corrupt", "missing-hash", "missing-file"} {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "licenses"), 0700)
		var manifest strings.Builder
		for _, name := range files {
			data := []byte("fixture " + name)
			if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "missing-hash" && name == "ukn" {
				continue
			}
			fmt.Fprintf(&manifest, "%x  %s\n", sha256.Sum256(data), name)
		}
		if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(manifest.String()), 0600); err != nil {
			t.Fatal(err)
		}
		if mode == "corrupt" {
			os.WriteFile(filepath.Join(dir, "ukn"), []byte("changed"), 0600)
		}
		if mode == "missing-file" {
			os.Remove(filepath.Join(dir, "space-check.sh"))
		}
		cmd := exec.Command("sh", "-c", "cd \"$1\"\n"+gate+"\nprintf 'VERIFIED\\n'", "integrity-test", dir)
		out, err := cmd.CombinedOutput()
		if (err == nil) != (mode == "valid") {
			t.Fatalf("mode=%s err=%v output=%s", mode, err, out)
		}
		if mode != "valid" && strings.Contains(string(out), "VERIFIED") {
			t.Fatal("invalid package passed integrity gate")
		}
	}
}
