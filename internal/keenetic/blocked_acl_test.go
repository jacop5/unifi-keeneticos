// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"reflect"
	"testing"
)

func TestBlockOverlayPreservesBaselineAndAuthorization(t *testing.T) {
	const a, b, c = "02:00:00:00:00:0a", "02:00:00:00:00:0b", "02:00:00:00:00:0c"
	allowed := func(acl AccessList, mac string) bool {
		member := false
		for _, value := range acl.MACs {
			member = member || value == mac
		}
		return acl.Policy == "none" || acl.Policy == "permit" && member || acl.Policy == "deny" && !member
	}
	for _, policy := range []string{"none", "permit", "deny"} {
		t.Run(policy, func(t *testing.T) {
			base := AccessList{Target: "WifiMaster0/AccessPoint6", Policy: policy, MACs: []string{a, b}}
			original := AccessList{Target: base.Target, Policy: policy, MACs: append([]string(nil), base.MACs...)}
			for _, blocked := range [][]string{nil, {a}, {c}, {a, b, c}, {"02:00:00:00:00:0A", a}} {
				got, err := ComposeBlockedACL(base, blocked)
				if err != nil {
					t.Fatal(err)
				}
				for _, mac := range []string{a, b, c, "02:00:00:00:00:0d"} {
					isBlocked := false
					for _, value := range blocked {
						normalized, _ := clientMAC(value)
						isBlocked = isBlocked || normalized == mac
					}
					if allowed(got, mac) != (allowed(base, mac) && !isBlocked) {
						t.Fatalf("authorization changed beyond block set: policy=%s client=%s", policy, mac)
					}
				}
				if !reflect.DeepEqual(base, original) {
					t.Fatal("overlay mutated baseline needed for unblock")
				}
			}
			unblocked, err := ComposeBlockedACL(base, nil)
			if err != nil || !reflect.DeepEqual(unblocked, original) {
				t.Fatal("unblock did not restore baseline")
			}
			unblocked.MACs[0] = c
			if !reflect.DeepEqual(base, original) {
				t.Fatal("result aliases baseline storage")
			}
		})
	}
}

func TestBlockOverlayRejectsMalformedInputs(t *testing.T) {
	base := AccessList{Target: "WifiMaster1/AccessPoint6", Policy: "permit"}
	for _, value := range []string{"invalid", "ff:ff:ff:ff:ff:ff", "00:00:00:00:00:00", "02:00:00:00:00:01; reboot"} {
		if _, err := ComposeBlockedACL(base, []string{value}); err == nil {
			t.Fatal("invalid blocked address accepted")
		}
		bad := base
		bad.MACs = []string{value}
		if _, err := ComposeBlockedACL(bad, nil); err == nil {
			t.Fatal("invalid baseline address accepted")
		}
	}
	for _, bad := range []AccessList{{Target: "WifiMaster1", Policy: "deny"}, {Target: base.Target, Policy: "unknown"}} {
		if _, err := ComposeBlockedACL(bad, nil); err == nil {
			t.Fatal("invalid baseline accepted")
		}
	}
}

func TestBlockingLastPermittedClientKeepsDenyAllThroughTransaction(t *testing.T) {
	const ap, mac = "WifiMaster0/AccessPoint6", "02:00:00:00:00:0a"
	base := AccessList{Target: ap, Policy: "permit", MACs: []string{mac}}
	composed, err := ComposeBlockedACL(base, []string{mac})
	if err != nil || composed.Policy != "permit" || len(composed.MACs) != 0 {
		t.Fatal("last permitted block weakened deny-all policy")
	}
	before := "known host client " + mac + "\ninterface " + ap + "\n    up\n    mac access-list type permit\n    mac access-list address " + mac + "\n!\n"
	after := "known host client " + mac + "\ninterface " + ap + "\n    up\n    mac access-list type permit\n!\n"
	p := Plan{AccessLists: []AccessList{composed}}
	changes, err := Preimage(p, before)
	if err != nil || len(changes) != 3 {
		t.Fatalf("block preparation failed: %v %v", changes, err)
	}
	if changes[0].Forward.Text != "interface "+ap+" down" ||
		changes[1].Forward.Text != "interface "+ap+" no mac access-list address "+mac ||
		changes[1].Undo.Text != "interface "+ap+" mac access-list address "+mac ||
		changes[2].Forward.Text != "interface "+ap+" up" {
		t.Fatal("block transaction lost quiescing or baseline inverse")
	}
	if VerifyAccessLists(p, before) == nil || VerifyAccessLists(p, after) != nil {
		t.Fatal("block readback did not distinguish original and deny-all")
	}
	unblocked, err := ComposeBlockedACL(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err = Preimage(Plan{AccessLists: []AccessList{unblocked}}, after)
	if err != nil || len(changes) != 3 || changes[1].Forward.Text != "interface "+ap+" mac access-list address "+mac {
		t.Fatal("unblock did not restore original permitted client")
	}
}

func TestBlockedPlanRequiresExplicitACLOnEveryConfiguredWLAN(t *testing.T) {
	const ap = "WifiMaster0/AccessPoint6"
	for _, cfg := range []string{wlan, wlan + "macacl.status=disabled\n"} {
		base, err := BuildPlan(cfg, map[string]string{"1": ap})
		if err != nil {
			t.Fatal(err)
		}
		for _, blocked := range [][]string{nil, {"02:00:00:00:00:0a"}} {
			p, err := ComposeBlockedPlan(base, blocked)
			if err != nil {
				t.Fatal(err)
			}
			if len(base.AccessLists) == 0 {
				if !reflect.DeepEqual(p.Unsupported, []string{"client-block." + ap + ".baseline-missing"}) {
					t.Fatal("missing ownership did not prevent the entire plan")
				}
			} else if len(p.Unsupported) != 0 || len(p.AccessLists) != 1 || len(p.AccessLists[0].MACs) != len(blocked) {
				t.Fatal("explicit controller baseline did not compose")
			}
			if !reflect.DeepEqual(p.Commands, base.Commands) || len(base.Unsupported) != 0 {
				t.Fatal("overlay changed unrelated commands or input diagnostics")
			}
		}
	}
	paused := Plan{Commands: []Command{{Text: "interface " + ap + " down"}}}
	if p, err := ComposeBlockedPlan(paused, nil); err != nil || len(p.Unsupported) != 0 {
		t.Fatal("omitted/paused slot could not remain down")
	}
}
