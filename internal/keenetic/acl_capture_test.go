// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"os"
	"strings"
	"testing"
)

func TestCapturedEmptyAllowListApplyAndRemoval(t *testing.T) {
	data, err := os.ReadFile("testdata/network-10.5.67-empty-allow.cfg")
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6", "4": "WifiMaster1/AccessPoint6"}
	plan, err := BuildWLANTestPlan(string(data), slots)
	if err != nil || len(plan.Unsupported) != 0 || len(plan.AccessLists) != 2 {
		t.Fatalf("%v %v", err, plan.Unsupported)
	}
	before := "interface Bridge0\n    include WifiMaster0/AccessPoint6\n    include WifiMaster1/AccessPoint6\n    up\n!\n"
	for _, ap := range []string{slots["3"], slots["4"]} {
		before += "interface " + ap + "\n    ssid \"UKN-Test-ACL\"\n    authentication wpa-psk \"synthetic-acl-key\"\n    encryption enable\n    encryption wpa2\n    up\n!\n"
	}
	// Empty permit must be preserved. An empty deny policy has different semantics.
	for _, a := range plan.AccessLists {
		if a.Policy != "permit" || len(a.MACs) != 0 {
			t.Fatal("empty allow-list became permissive")
		}
	}
	changes, err := Preimage(plan, before)
	if err != nil || len(changes) != 6 {
		t.Fatalf("expected only six guarded ACL changes without host registration: %d %v", len(changes), err)
	}
	for i, a := range plan.AccessLists {
		for j, pair := range [][2]string{{"down", "up"}, {"mac access-list type permit", "mac access-list type none"}, {"up", "down"}} {
			change := changes[i*3+j]
			prefix := "interface " + a.Target + " "
			if change.Forward.Text != prefix+pair[0] || change.Undo.Text != prefix+pair[1] {
				t.Fatal("ACL pause/inverse ordering changed")
			}
		}
	}
	applied := before
	// Only AP blocks acquire the policy; Bridge0 remains untouched.
	for _, ap := range []string{slots["3"], slots["4"]} {
		applied = strings.ReplaceAll(applied, "interface "+ap+"\n", "interface "+ap+"\n    mac access-list type permit\n")
	}
	if VerifyAccessLists(plan, before) == nil {
		t.Fatal("missing permit policy passed readback")
	}
	if err := VerifyAccessLists(plan, applied); err != nil {
		t.Fatal(err)
	}
	if changes, err := Preimage(plan, applied); err != nil || len(changes) != 0 {
		t.Fatalf("repeat apply interrupted APs: %v %v", changes, err)
	}
	removed, err := BuildWLANTestPlan(strings.ReplaceAll(string(data), "macacl.status=enabled", "macacl.status=disabled"), slots)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range removed.AccessLists {
		if a.Policy != "none" {
			t.Fatal("disabled ACL retained permit")
		}
	}
	if VerifyAccessLists(removed, applied) == nil {
		t.Fatal("stale blocking policy passed removal readback")
	}
	if err := VerifyAccessLists(removed, before); err != nil {
		t.Fatal(err)
	}
	if changes, err := Preimage(removed, applied); err != nil || len(changes) != 6 {
		t.Fatalf("removal preflight: %d %v", len(changes), err)
	}
}
