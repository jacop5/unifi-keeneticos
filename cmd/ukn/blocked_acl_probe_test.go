// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestBlockedACLProbeReusesRegistrationAndHasReversibleDisabledScope(t *testing.T) {
	const ap, mac = "WifiMaster0/AccessPoint5", "02:00:00:00:00:0a"
	running := "known host invalid ff:ff:ff:ff:ff:ff\nknown host zero 00:00:00:00:00:00\nknown host lab 02:00:00:00:00:0A\ninterface " + ap + "\n    down\n    mac access-list type none\n!\n"
	p, err := blockedACLProbePlan(ap, running)
	if err != nil || len(p.AccessLists) != 1 || p.AccessLists[0].Policy != "deny" || len(p.AccessLists[0].MACs) != 1 || p.AccessLists[0].MACs[0] != mac {
		t.Fatalf("wrong overlay plan: %v", err)
	}
	changes, err := keenetic.Preimage(p, running)
	if err != nil || len(changes) != 3 {
		t.Fatalf("probe rollback preparation failed: %v", err)
	}
	for _, change := range changes {
		for _, command := range []keenetic.Command{change.Forward, change.Undo} {
			if !strings.HasPrefix(command.Text, "interface "+ap+" ") || command.Text == "interface "+ap+" up" {
				t.Fatal("probe escaped target or enabled the unused AP")
			}
		}
	}
	for _, snapshot := range []string{"", "known host invalid ff:ff:ff:ff:ff:ff\n"} {
		if _, err := blockedACLProbePlan(ap, snapshot); err == nil {
			t.Fatal("probe fabricated a known host")
		}
	}
}

func TestBlockedACLProbeRefusesServingOrConfiguredSlotsBeforeMutation(t *testing.T) {
	for _, mode := range []string{"primary", "up", "status-ssid", "running-ssid"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			fake := `#!/bin/sh
printf '%s\n' "$2" >> "$UKN_BLOCK_PROBE_EVENTS"
case "$2" in
 'show interface WifiMaster0/AccessPoint5')
  printf 'id: WifiMaster0/AccessPoint5\n'
  if [ "$UKN_BLOCK_PROBE_MODE" = up ]; then printf 'state: up\n'; else printf 'state: down\n'; fi
  if [ "$UKN_BLOCK_PROBE_MODE" = status-ssid ]; then printf 'ssid: Serving\n'; fi;;
 'show running-config') printf 'interface WifiMaster0/AccessPoint5\n    ssid Serving\n    down\n!\n';;
 *) exit 99;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte(fake), 0700); err != nil {
				t.Fatal(err)
			}
			target := "WifiMaster0/AccessPoint5"
			if mode == "primary" {
				target = "WifiMaster0/AccessPoint0"
			}
			events, state := filepath.Join(dir, "events"), filepath.Join(dir, "state")
			cmd := exec.Command(os.Args[0], "-test.run=^TestStartupCLIHelper$", "--", "-state", state,
				"-rollback-probe", target, "-rollback-probe-mode", "blocked-acl")
			cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "UKN_BLOCK_PROBE_MODE="+mode,
				"UKN_BLOCK_PROBE_EVENTS="+events, "PATH="+dir+":"+os.Getenv("PATH"))
			if out, err := cmd.CombinedOutput(); err == nil || strings.Contains(string(out), "probe passed") {
				t.Fatalf("unsafe slot accepted: %v %s", err, out)
			}
			b, err := os.ReadFile(events)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, command := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				if command != "" && !strings.HasPrefix(command, "show ") {
					t.Fatal("guard allowed mutation")
				}
			}
			if _, err := os.Stat(filepath.Join(state, "rollback-probe.json")); !os.IsNotExist(err) {
				t.Fatal("unsafe slot created journal")
			}
		})
	}
}
