// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

const wlanOnlyControllerCfg = `radio.1.phyname=wifi0
radio.1.channel=999
radio.1.status=bogus
radio.2.phyname=wifi9
wireless.1.ssid=Corp Net
wireless.1.parent=wifi0
wireless.1.usage=user
aaa.1.ssid=Corp Net
aaa.1.wpa=2
aaa.1.wpa.psk=production-passphrase
wireless.2.ssid=Serving
wireless.2.parent=wifi1
wireless.2.usage=user
aaa.2.ssid=Serving
aaa.2.wpa=2
aaa.2.wpa.psk=another-passphrase
`

// Offline -plan with -wlan-only must stay pure: a native sentinel proves no
// ndmc execution, and conflicting or missing mapping arguments must fail.
func TestOfflineWLANOnlyCLI(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"controller.cfg": wlanOnlyControllerCfg,
		"slots.json":     `{"1":"WifiMaster0/AccessPoint0"}`,
		"bad-slots.json": `{"1":1}`,
		"ndmc":           "#!/bin/sh\ntouch \"$UKN_WLANONLY_NATIVE\"\nexit 99\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	slots := filepath.Join(dir, "slots.json")
	marker := filepath.Join(dir, "native-touched")
	for _, tc := range []struct {
		name string
		args []string
		fail bool
		json bool
	}{
		{name: "valid text plan", args: []string{"-plan", filepath.Join(dir, "controller.cfg"), "-slots", slots, "-wlan-only"}},
		{name: "valid json plan", args: []string{"-plan", filepath.Join(dir, "controller.cfg"), "-slots", slots, "-wlan-only", "-plan-json"}, json: true},
		{name: "missing slots flag", args: []string{"-plan", filepath.Join(dir, "controller.cfg"), "-wlan-only"}, fail: true},
		{name: "conflicts with test mode", args: []string{"-plan", filepath.Join(dir, "controller.cfg"), "-slots", slots, "-wlan-only", "-wlan-test-only"}, fail: true},
		{name: "missing slots file", args: []string{"-plan", filepath.Join(dir, "controller.cfg"), "-slots", filepath.Join(dir, "absent.json"), "-wlan-only"}, fail: true},
		{name: "invalid slots json", args: []string{"-plan", filepath.Join(dir, "controller.cfg"), "-slots", filepath.Join(dir, "bad-slots.json"), "-wlan-only"}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-test.run=^TestStartupCLIHelper$", "--", "-state", dir}, tc.args...)
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "UKN_WLANONLY_NATIVE="+marker, "PATH="+dir+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected CLI result: %v %s", err, out)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatal("offline wlan-only planning invoked native commands")
			}
			if strings.Contains(string(out), "production-passphrase") || strings.Contains(string(out), "another-passphrase") {
				t.Fatal("offline plan exposed credentials")
			}
			if tc.fail {
				return
			}
			text := string(out)
			if tc.json {
				var report map[string]any
				if err := json.Unmarshal(out, &report); err != nil {
					t.Fatalf("invalid JSON plan output: %v %s", err, out)
				}
				if report["planner_supported"] != true {
					t.Fatalf("plan not supported: %s", out)
				}
				commands, _ := report["commands"].([]any)
				for _, c := range commands {
					text += c.(string) + "\n"
				}
			}
			if !strings.Contains(text, `interface WifiMaster0/AccessPoint0 ssid "Corp Net"`+"\n") {
				t.Fatalf("mapped primary slot missing from plan:\n%s", text)
			}
			for _, banned := range []string{"WifiMaster0 channel", "WifiMaster1 channel", "country-code", "Serving", "AccessPoint6"} {
				if strings.Contains(text, banned) {
					t.Fatalf("radio-wide or unmapped content in wlan-only plan: %q\n%s", banned, text)
				}
			}
		})
	}
}

func TestWLANOnlyProvisionStatusScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provision-status.json")
	r := provisionReporter{path: path, report: emptyProvisionStatus(), apply: true, wlanOnly: true}
	plan := keenetic.Plan{Commands: []keenetic.Command{{Text: `interface WifiMaster0/AccessPoint0 ssid "Corp Net"`}}}
	if err := r.record("v1", "confirmed", &plan, time.Time{}); err != nil {
		t.Fatal(err)
	}
	got, err := readProvisionStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.WLANOnly || got.WLANTestOnly {
		t.Fatalf("top-level wlan_only flag wrong: %+v", got)
	}
	if got.LastConfirmedScope == nil || !got.LastConfirmedScope.WLANOnly || got.LastConfirmedScope.WLANTestOnly {
		t.Fatalf("last-confirmed scope flag wrong: %+v", got.LastConfirmedScope)
	}
	// Reports written before wlan_only existed must still read cleanly.
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(legacy, []byte(`{"schema_version":1,"phase":"confirmed","wlan_test_only":true,"last_confirmed_scope":{"wlan_test_only":true,"interfaces":["WifiMaster0/AccessPoint6"],"ignored_settings":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := readProvisionStatus(legacy)
	if err != nil || old.WLANOnly || !old.WLANTestOnly || old.LastConfirmedScope == nil || old.LastConfirmedScope.WLANOnly {
		t.Fatalf("legacy report incompatible: %+v %v", old, err)
	}
}

// The shipped service must validate WLAN_ONLY, require SLOTS for it, reject
// WLAN_TEST_ONLY+WLAN_ONLY, keep NETWORKS allowed, and forward the flag.
func TestEntwareServiceWLANOnly(t *testing.T) {
	original, err := os.ReadFile("../../packaging/keenetic/S99unifi-keenetic")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	text := strings.Replace(string(original), "\ncase \"${1:-}\" in\n", "\ndaemon_running() { [ \"${test_started:-0}\" = 1 ]; }\ncase \"${1:-}\" in\n", 1)
	if text == string(original) {
		t.Fatal("service dispatch not found")
	}
	script := filepath.Join(dir, "service")
	if err := os.WriteFile(script, []byte(text), 0700); err != nil {
		t.Fatal(err)
	}
	slots := filepath.Join(dir, "slots.json")
	if err := os.WriteFile(slots, []byte(`{"1":"WifiMaster0/AccessPoint0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, config string
		ok           bool
		flags        []string
		absent       []string
	}{
		{"default omits wlan-only", "", true, nil, []string{"-wlan-only"}},
		{"invalid switch", "WLAN_ONLY=yes\n", false, nil, nil},
		{"requires slots", "WLAN_ONLY=1\n", false, nil, nil},
		{"forwards flag", "WLAN_ONLY=1\nSLOTS='" + slots + "'\n", true, []string{"-wlan-only", "-slots", slots}, nil},
		{"networks allowed in production mode", "WLAN_ONLY=1\nSLOTS='" + slots + "'\nNETWORKS='" + slots + "'\n", true, []string{"-wlan-only", "-networks", slots}, nil},
		{"conflicts with test mode", "WLAN_ONLY=1\nWLAN_TEST_ONLY=1\nSLOTS='" + slots + "'\n", false, nil, nil},
		{"reboot policy independent", "WLAN_ONLY=1\nSLOTS='" + slots + "'\nREBOOT_CONTROL=1\n", true, []string{"-wlan-only", "-reboot-control"}, nil},
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
			for _, flag := range tc.absent {
				if strings.Contains(string(out), "ARG:"+flag+"\n") {
					t.Fatalf("unexpected argument %q: %s", flag, out)
				}
			}
		})
	}
}
