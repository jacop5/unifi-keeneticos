// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineBlockedPlanCLIHasNoNativeOrStateAccess(t *testing.T) {
	dir := t.TempDir()
	config, err := os.ReadFile("../../internal/keenetic/testdata/compatible-wpa2.cfg")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"controller.cfg": string(config) + "macacl.status=disabled\n",
		"slots.json":     `{"3":"WifiMaster0/AccessPoint6","4":"WifiMaster1/AccessPoint6"}`,
		"ndmc":           "#!/bin/sh\ntouch \"$UKN_PLAN_NATIVE_TOUCHED\"\nexit 99\n",
		"state":          "this is deliberately not a state directory",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, input string
		count       int
		fail        bool
		args        []string
	}{
		{name: "block", input: `["02:00:00:00:fa:ce"]`, count: 1},
		{name: "unblock", input: `[]`},
		{name: "invalid MAC", input: `["private-invalid-mac"]`, fail: true},
		{name: "invalid JSON", input: `["private-invalid-json"`, fail: true},
		{name: "null", input: `null`, fail: true},
		{name: "oversized input", input: strings.Repeat(" ", 65537) + `[]`, fail: true},
		{name: "missing plan", input: `[]`, fail: true, args: []string{"-plan", ""}},
		{name: "live block conflict", input: `[]`, fail: true, args: []string{"-client-block-control"}},
		{name: "apply conflict", input: `[]`, fail: true, args: []string{"-apply"}},
		{name: "probe conflict", input: `[]`, fail: true, args: []string{"-probe"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := filepath.Join(dir, "blocked.json")
			if err := os.WriteFile(input, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-test.run=^TestStartupCLIHelper$", "--", "-state", filepath.Join(dir, "state"),
				"-plan", filepath.Join(dir, "controller.cfg"), "-plan-json", "-wlan-test-only",
				"-slots", filepath.Join(dir, "slots.json"), "-plan-blocked", input}
			args = append(args, tc.args...)
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "UKN_PLAN_NATIVE_TOUCHED="+filepath.Join(dir, "native-touched"), "PATH="+dir+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected CLI result: %v %s", err, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "native-touched")); !os.IsNotExist(err) {
				t.Fatal("offline block planning invoked native commands")
			}
			if strings.Contains(string(out), "synthetic-compatible-key") || strings.Contains(string(out), "02:00:00:00:fa:ce") || strings.Contains(string(out), "private-invalid") {
				t.Fatal("block planning exposed private input")
			}
			if !tc.fail {
				var report struct {
					Supported bool `json:"planner_supported"`
					ACLs      []struct {
						Policy string `json:"policy"`
						Count  int    `json:"client_count"`
					} `json:"access_lists"`
				}
				if err := json.Unmarshal(out, &report); err != nil || !report.Supported || len(report.ACLs) != 2 {
					t.Fatalf("invalid composed report: %v", err)
				}
				for _, acl := range report.ACLs {
					want := "none"
					if tc.count != 0 {
						want = "deny"
					}
					if acl.Count != tc.count || acl.Policy != want {
						t.Fatal("wrong composed ACL")
					}
				}
			}
		})
	}
}
