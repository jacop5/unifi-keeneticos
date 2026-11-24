// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestWiFiSystemCLIReadOnlyAndConflictChecks(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events")
	state := filepath.Join(dir, "state-must-not-exist")
	script := `#!/bin/sh
printf '%s\n' "$2" >> "$UKN_WIFI_SYSTEM_EVENTS"
case "$2" in
 'show mws wlan') printf 'wlan, id = Test:\ndisabled-by-schedule: no\nband, id = 0:\nenabled: yes\naccess-point, name = TestAP:\nwps:\nself-pin: 87654321\n';;
 'show interface') printf 'Interface, name = "TestAP"\n id: WifiMaster0/AccessPoint6\n';;
 *) exit 99;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, conflict := range []string{"", "-apply", "-provision-status", "-plan"} {
		if err := os.Remove(events); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		args := []string{"-test.run=^TestStartupCLIHelper$", "--", "-state", state, "-wifi-system-status", "-url", "http://127.0.0.1:1/inform"}
		if conflict != "" {
			args = append(args, conflict)
			if conflict == "-plan" {
				args = append(args, "nonexistent-plan")
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], args...)
		cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "UKN_WIFI_SYSTEM_EVENTS="+events, "PATH="+dir+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		cancel()
		if (err == nil) != (conflict == "") {
			t.Fatalf("conflict=%q err=%v output=%s", conflict, err, out)
		}
		if conflict == "" {
			var r keenetic.WiFiSystemInventory
			if json.Unmarshal(out, &r) != nil || r.SchemaVersion != 1 || len(r.WLANs) != 1 || r.UnresolvedAccessPoints != 0 {
				t.Fatalf("invalid inventory: %s", out)
			}
			if strings.Contains(string(out), "87654321") {
				t.Fatal("WPS data leaked")
			}
			b, err := os.ReadFile(events)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(strings.Split(strings.TrimSpace(string(b)), "\n"), []string{"show mws wlan", "show interface"}) {
				t.Fatal("unexpected native calls")
			}
		} else if _, err := os.Stat(events); !os.IsNotExist(err) {
			t.Fatal("conflicting mode contacted native runner")
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("read-only mode wrote state directory")
		}
	}
}
