// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"strings"
	"testing"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestDisabledFTProbeHasExactRecoveryAndNoServingCommands(t *testing.T) {
	const ap = "WifiMaster0/AccessPoint5"
	plan := disabledFTProbePlan(ap)
	for _, c := range plan.Commands {
		if !strings.HasPrefix(c.Text, "interface "+ap+" ") || strings.HasSuffix(c.Text, " up") {
			t.Fatal("probe escaped disabled AP scope")
		}
	}
	before := "interface " + ap + "\n    ft mdid XY\n    ft otd\n    down\n!\n"
	after := "interface " + ap + "\n    ssid ukn-rollback-probe\n    ft mdid AB\n    ft enable\n    down\n!\n"
	changes, err := keenetic.Preimage(plan, before)
	if err != nil || len(changes) != 4 {
		t.Fatalf("%v %v", changes, err)
	}
	for i, undo := range []string{"no ssid", "ft mdid XY", "ft otd", "no ft enable"} {
		if changes[i].Undo.Text != "interface "+ap+" "+undo {
			t.Fatalf("inverse %d missing", i)
		}
	}
	if err := keenetic.VerifyWLANSettings(plan, after); err != nil {
		t.Fatal(err)
	}
	if keenetic.VerifyWLANSettings(plan, before) == nil {
		t.Fatal("unchanged settings accepted")
	}
}
