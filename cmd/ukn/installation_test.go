// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/installation"
)

func TestInstallationStatusCLIIsOfflineAndStateFree(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "entware")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".ukn-install.old-format")
	if err := os.WriteFile(marker, []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(dir, "unexpected-native-call")
	if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte("#!/bin/sh\ntouch '"+events+"'\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state-must-not-exist")
	for _, conflict := range []string{"", "-apply", "-probe", "-wifi-system-status", "-build-info", "-provision-status", "-plan"} {
		args := []string{"-test.run=^TestStartupCLIHelper$", "--", "-installation-status", root,
			"-state", state, "-url", "http://127.0.0.1:1/inform"}
		if conflict != "" {
			args = append(args, conflict)
			if conflict == "-plan" {
				args = append(args, "nonexistent-plan")
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], args...)
		cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "PATH="+dir+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		cancel()
		if (err == nil) != (conflict == "") {
			t.Fatalf("%s: %v %s", conflict, err, out)
		}
		if conflict == "" {
			var status installation.Status
			if json.Unmarshal(out, &status) != nil || !status.Pending || status.UnknownMarkers != 1 {
				t.Fatalf("%s", out)
			}
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("created state")
		}
		if _, err := os.Stat(events); !os.IsNotExist(err) {
			t.Fatal("called native CLI")
		}
		b, err := os.ReadFile(marker)
		if err != nil || string(b) != "private fixture" {
			t.Fatal("modified installation marker")
		}
	}
}
