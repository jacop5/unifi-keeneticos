// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"testing"
)

func TestOfflinePreflightRequiresReversibleOwnedTargets(t *testing.T) {
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	p := keenetic.Plan{Commands: []keenetic.Command{{Text: "interface WifiMaster0/AccessPoint6 down"}}}
	running := "interface WifiMaster0/AccessPoint6\n    ssid UKN-Test-Preflight\n    up\n!\n"
	n, err := preflightPlan(p, running, slots, true)
	if err != nil || n != 1 {
		t.Fatalf("valid preflight: %d %v", n, err)
	}
	for _, bad := range []string{"", "interface WifiMaster0/AccessPoint6\n    ssid Serving\n    up\n!\n"} {
		if _, err := preflightPlan(p, bad, slots, true); err == nil {
			t.Fatal("unsafe snapshot accepted")
		}
	}
	p.Unsupported = []string{"wireless.1.security"}
	if _, err := preflightPlan(p, running, slots, true); err == nil {
		t.Fatal("unsupported plan accepted")
	}
}

func TestOfflinePreflightRejectsConflictingModesBeforeDispatch(t *testing.T) {
	for _, mode := range []string{"rollback-watch", "rollback-probe", "crash-probe", "apply", "led-control", "probe", "terminal-probe", "models", "provision-status"} {
		if err := validateOfflineMode("native.cfg", "controller.cfg", map[string]bool{mode: true}); err == nil {
			t.Fatalf("conflicting %s accepted", mode)
		}
		if err := validateOfflineMode("native.cfg", "controller.cfg", map[string]bool{mode: false}); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateOfflineMode("native.cfg", "", nil); err == nil {
		t.Fatal("missing plan accepted")
	}
	if err := validateOfflineMode("", "", map[string]bool{"apply": true}); err != nil {
		t.Fatal("normal live mode rejected")
	}
}
