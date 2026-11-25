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
	original, err := os.ReadFile("../../packaging/keenetic/S99unifi-keenetic")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// These option tests mock process observation only. Dedicated status tests
	// and the Linux container exercise the actual observer.
	text := strings.Replace(string(original), "\ncase \"${1:-}\" in\n", "\ndaemon_running() { [ \"${test_started:-0}\" = 1 ]; }\ncase \"${1:-}\" in\n", 1)
	if text == string(original) {
		t.Fatal("service dispatch not found")
	}
	script := filepath.Join(dir, "service")
	if err := os.WriteFile(script, []byte(text), 0700); err != nil {
		t.Fatal(err)
	}
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
		{"experimental SAE", "EXPERIMENTAL_SAE=1\n", true, []string{"-experimental-sae"}},
		{"experimental FT mapping", "EXPERIMENTAL_FT_DOMAINS='" + slots + "'\n", true, []string{"-experimental-ft-domains", slots}},
		{"missing FT mapping", "EXPERIMENTAL_FT_DOMAINS='/missing-ukn-ft'\n", false, nil},
		{"invalid SAE", "EXPERIMENTAL_SAE=yes\n", false, nil},
		{"custom state", "STATE_DIR='/tmp/private state'\nPID_FILE='/tmp/private pid/agent.pid'\n", true, []string{"-state", "/tmp/private state"}},
		{"relative state", "STATE_DIR=relative\n", false, nil},
		{"newline state", "STATE_DIR='/tmp/first\nsecond'\n", false, nil},
		{"empty pid", "PID_FILE=''\n", false, nil},
		{"test apply", "APPLY=1\nWLAN_TEST_ONLY=1\nSLOTS='" + slots + "'\n", true, []string{"-apply", "-wlan-test-only", "-slots", slots}},
		{"reboot opt-in", "REBOOT_CONTROL=1\n", true, []string{"-reboot-control"}},
		{"invalid reboot switch", "REBOOT_CONTROL=yes\n", false, nil},
		{"reboot in test scope", "REBOOT_CONTROL=1\nWLAN_TEST_ONLY=1\nSLOTS='" + slots + "'\n", false, nil},
		{"LED only", "LED_CONTROL=1\n", true, []string{"-led-control"}},
		{"client block preview", "CLIENT_BLOCK_CONTROL=1\nSLOTS='" + slots + "'\n", true, []string{"-client-block-control"}},
		{"invalid client block switch", "CLIENT_BLOCK_CONTROL=yes\n", false, nil},
		{"client blocking without slots", "CLIENT_BLOCK_CONTROL=1\n", false, nil},
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
start-stop-daemon() { printf 'ARG:%s\n' "$@"; test_started=1; }
sleep() { :; }
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
				for _, flag := range []string{"-apply", "-wlan-test-only", "-led-control", "-networks", "-slots", "-report-upgrade-version", "-experimental-sae", "-experimental-ft-domains", "-reboot-control"} {
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
	for _, mode := range []string{"exits", "stuck", "zombie"} {
		stuck := mode == "stuck"
		dir := t.TempDir()
		pid := filepath.Join(dir, "daemon.pid")
		script := filepath.Join(dir, "service")
		if err := os.WriteFile(pid, []byte("12345\n"), 0600); err != nil {
			t.Fatal(err)
		}
		text := string(original)
		config := filepath.Join(dir, "config")
		if err := os.WriteFile(config, []byte("PID_FILE='"+pid+"'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		proc := filepath.Join(dir, "proc")
		os.MkdirAll(filepath.Join(proc, "12345"), 0700)
		status := "State:\tS (sleeping)\n"
		if mode == "zombie" {
			status = "State:\tZ (zombie)\n"
		}
		if err := os.WriteFile(filepath.Join(proc, "12345", "status"), []byte(status), 0600); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(proc, "12345", "cmdline"), []byte("/opt/sbin/ukn\x00-state\x00/opt/var/lib/unifi-keenetic\x00"), 0600)
		text = strings.ReplaceAll(text, "/proc/", proc+"/")
		if err := os.WriteFile(script, []byte(text), 0700); err != nil {
			t.Fatal(err)
		}
		behavior := `n=$((n+1)); [ "$n" -lt 5 ]`
		if mode != "exits" {
			behavior = "return 0"
		}
		cmd := exec.Command("bash", "-c", `
n=0
kill() { if [ "$1" = -TERM ]; then printf 'TERM requested\n'; return 0; fi; `+behavior+`; }
sleep() { :; }
readlink() { printf '%s' /opt/sbin/ukn; }
script=$1
set -- stop
. "$script"
`, "stop-test", script)

		cmd.Env = append(os.Environ(), "UKN_CONFIG="+config)
		out, err := cmd.CombinedOutput()
		if (err != nil) != stuck {
			t.Fatalf("stuck=%v err=%v output=%s", stuck, err, out)
		}
		if mode != "zombie" && !strings.Contains(string(out), "TERM requested") {
			t.Fatal("stop did not request graceful termination")
		}
		if stuck && !strings.Contains(string(out), "refusing immediate restart") {
			t.Fatal("shutdown timeout not explicit")
		}
	}
}

func TestEntwareStatusRejectsStaleAndUnrelatedPIDs(t *testing.T) {
	original, err := os.ReadFile("../../packaging/keenetic/S99unifi-keenetic")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"running", "spaces", "missing", "invalid", "zombie", "unrelated", "other-state", "missing-state", "late-state", "override", "diagnostic", "watchdog", "truncated"} {
		dir := t.TempDir()
		pid := filepath.Join(dir, "pid")
		proc := filepath.Join(dir, "proc")
		if err := os.MkdirAll(filepath.Join(proc, "12345"), 0700); err != nil {
			t.Fatal(err)
		}
		state := "State:\tS (sleeping)\n"
		if mode == "zombie" {
			state = "State:\tZ (zombie)\n"
		}
		if err := os.WriteFile(filepath.Join(proc, "12345", "status"), []byte(state), 0600); err != nil {
			t.Fatal(err)
		}
		if mode != "missing" {
			value := "12345"
			if mode == "invalid" {
				value = "12345; reboot"
			}
			if err := os.WriteFile(pid, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
		}
		config := filepath.Join(dir, "config")
		stateDir := "/opt/var/lib/unifi-keenetic"
		if mode == "spaces" {
			stateDir = "/tmp/state with spaces"
		}
		os.WriteFile(config, []byte("PID_FILE='"+pid+"'\nSTATE_DIR='"+stateDir+"'\n"), 0600)
		args := []string{"/opt/sbin/ukn", "-state", stateDir}
		switch mode {
		case "other-state":
			args[2] = "/tmp/other-state"
		case "missing-state":
			args = args[:1]
		case "late-state":
			args = []string{"/opt/sbin/ukn", "-url", "http://127.0.0.1:9/inform", "-state", stateDir}
		case "override":
			args = append(args, "-state=/tmp/other-state")
		case "diagnostic":
			args = append(args, "--build-info")
		case "watchdog":
			args = append(args, "-rollback-watch=/tmp/journal")
		}
		cmdline := strings.Join(args, "\x00") + "\x00"
		if mode == "truncated" {
			cmdline = strings.TrimSuffix(cmdline, "\x00")
		}
		os.WriteFile(filepath.Join(proc, "12345", "cmdline"), []byte(cmdline), 0600)
		script := filepath.Join(dir, "service")
		os.WriteFile(script, []byte(strings.ReplaceAll(string(original), "/proc/", proc+"/")), 0700)
		exe := "/opt/sbin/ukn"
		if mode == "unrelated" {
			exe = "/bin/other"
		}
		cmd := exec.Command("bash", "-c", `
kill() { return 0; }
script=$1
# Pass the simulated executable through a wrapper without invoking real readlink.
reported=$2
readlink() { if [ "$1" = -f ]; then printf '%s' /opt/sbin/ukn; else printf '%s' "$reported"; fi; }
set -- status
. "$script"
`, "status-test", script, exe)
		cmd.Env = append(os.Environ(), "UKN_CONFIG="+config)
		out, err := cmd.CombinedOutput()
		if (err == nil) != (mode == "running" || mode == "spaces") {
			t.Fatalf("mode=%s err=%v output=%s", mode, err, out)
		}
	}
}
