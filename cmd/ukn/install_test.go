// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerSpaceGuard(t *testing.T) {
	helper, err := filepath.Abs("../../packaging/keenetic/space-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "binary with spaces")
	if err := os.WriteFile(binary, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		free string
		ok   bool
	}{{"50000", true}, {"0", false}, {"10000", false}, {"unknown", false}, {"", false}} {
		cmd := exec.Command("sh", "-c", `
# Capture the test measurement independently of df's arguments.
free=$3
df() { printf 'Filesystem blocks Used Available Use%% Mounted\n'; printf 'tmpfs 100000 0 %s 0%% /opt\n' "$free"; }
. "$1"
check_install_space "$2" /opt
`, "space-test", helper, binary, row.free)
		out, err := cmd.CombinedOutput()
		if (err == nil) != row.ok {
			t.Fatalf("free=%q err=%v output=%s", row.free, err, out)
		}
		if !row.ok && !strings.Contains(string(out), "space") {
			t.Fatal("missing space refusal")
		}
	}
}
