// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the shipped service itself. Shell functions intercept filesystem
// preparation and process launch so these checks cannot touch a host's /opt.
func TestEntwareServiceOptions(t *testing.T) {
	script, err := filepath.Abs("../../packaging/keenetic/S99unifi-keenetic")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	slots := filepath.Join(dir, "slots with spaces.json")
	if err := os.WriteFile(slots, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, config string
		ok           bool
		flags        []string
	}{
		{"defaults", "", true, nil},
		{"test apply", "APPLY=1\nWLAN_TEST_ONLY=1\nSLOTS='" + slots + "'\n", true, []string{"-apply", "-wlan-test-only", "-slots", slots}},
		{"LED only", "LED_CONTROL=1\n", true, []string{"-led-control"}},
		{"version reporting", "REPORT_UPGRADE_VERSION=1\n", true, []string{"-report-upgrade-version"}},
		{"invalid version mode", "REPORT_UPGRADE_VERSION=yes\n", false, nil},
		{"missing slots", "APPLY=1\n", false, nil},
		{"invalid switch", "APPLY=yes\n", false, nil},
		{"missing file", "SLOTS='/missing-ukn-slots'\n", false, nil},
		{"VLAN in WiFi mode", "WLAN_TEST_ONLY=1\nSLOTS='" + slots + "'\nNETWORKS='" + slots + "'\n", false, nil},
		{"mapped VLAN", "SLOTS='" + slots + "'\nNETWORKS='" + slots + "'\n", true, []string{"-networks", slots}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := filepath.Join(dir, "config")
			if err := os.WriteFile(config, []byte("ENABLED=1\nINFORM_URL='http://192.0.2.1:8080/inform'\n"+tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", "-c", `
mkdir() { :; }
chmod() { :; }
start-stop-daemon() { printf 'ARG:%s\n' "$@"; }
script=$1
set -- start
. "$script"
`, "service-test", script)
			cmd.Env = append(os.Environ(), "UKN_CONFIG="+config)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v output=%s", err, out)
			}
			if !tc.ok && strings.Contains(string(out), "ARG:") {
				t.Fatal("invalid configuration launched daemon")
			}
			for _, flag := range tc.flags {
				if !strings.Contains(string(out), "ARG:"+flag+"\n") {
					t.Fatalf("missing argument %q: %s", flag, out)
				}
			}
			if tc.name == "defaults" {
				for _, flag := range []string{"-apply", "-wlan-test-only", "-led-control", "-networks", "-slots", "-report-upgrade-version"} {
					if strings.Contains(string(out), "ARG:"+flag+"\n") {
						t.Fatalf("default service enabled %s", flag)
					}
				}
			}
		})
	}
}

func TestEntwareStopWaitsForProcessExit(t *testing.T) {
	original, err := os.ReadFile("../../packaging/keenetic/S99unifi-keenetic")
	if err != nil {
		t.Fatal(err)
	}
	for _, stuck := range []bool{false, true} {
		dir := t.TempDir()
		pid := filepath.Join(dir, "daemon.pid")
		script := filepath.Join(dir, "service")
		if err := os.WriteFile(pid, []byte("12345\n"), 0600); err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(original), "/opt/var/run/unifi-keenetic.pid", pid)
		if err := os.WriteFile(script, []byte(text), 0700); err != nil {
			t.Fatal(err)
		}
		behavior := `n=$((n+1)); [ "$n" -lt 3 ]`
		if stuck {
			behavior = "return 0"
		}
		cmd := exec.Command("bash", "-c", `
n=0
kill() { `+behavior+`; }
sleep() { :; }
start-stop-daemon() { printf 'TERM requested\n'; }
script=$1
set -- stop
. "$script"
`, "stop-test", script)

		cmd.Env = append(os.Environ(), "UKN_CONFIG="+filepath.Join(dir, "absent-config"))
		out, err := cmd.CombinedOutput()
		if (err != nil) != stuck {
			t.Fatalf("stuck=%v err=%v output=%s", stuck, err, out)
		}
		if !strings.Contains(string(out), "TERM requested") {
			t.Fatal("stop did not request graceful termination")
		}
		if stuck && !strings.Contains(string(out), "refusing immediate restart") {
			t.Fatal("shutdown timeout not explicit")
		}
	}
}
