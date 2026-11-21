// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

const aclConfig = "wireless.1.devname=ath0\nmacacl.status=enabled\nmacacl.7.devname=ath0\nmacacl.7.status=enabled\nmacacl.7.acl.status=enabled\nmacacl.7.acl.policy=allow\nmacacl.7.acl.1.mac=02:00:00:00:00:0A\nmacacl.7.acl.2.mac=02:00:00:00:00:0b\nmacacl.7.acl.2.status=disabled\n"

func TestControllerACLUsesDeviceNotIndex(t *testing.T) {
	p, err := BuildPlan(wlan+aclConfig, map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil || len(p.AccessLists) != 1 {
		t.Fatalf("plan=%#v err=%v", p, err)
	}
	a := p.AccessLists[0]
	if a.Target != "WifiMaster0/AccessPoint6" || a.Policy != "permit" || len(a.MACs) != 1 || a.MACs[0] != "02:00:00:00:00:0a" {
		t.Fatal(a)
	}
	p, err = BuildWLANTestPlan(strings.ReplaceAll(wlan+aclConfig, "Home WiFi", "UKN-Test-ACL"), map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil || len(p.AccessLists) != 1 {
		t.Fatalf("connected test lost ACL: %#v %v", p, err)
	}
}

func TestControllerACLRejectsInvalidRequests(t *testing.T) {
	for _, cfg := range []string{
		strings.ReplaceAll(aclConfig, "02:00:00:00:00:0A", "ff:ff:ff:ff:ff:ff"),
		strings.ReplaceAll(aclConfig, "02:00:00:00:00:0A", "02:00:00:00:00:00:00:01"),
		strings.ReplaceAll(aclConfig, "02:00:00:00:00:0A", "00:00:00:00:00:00"),
		strings.ReplaceAll(aclConfig, "policy=allow", "policy=bogus"),
		aclConfig + "macacl.8.devname=ath0\n",
	} {
		if _, err := BuildPlan(wlan+cfg, map[string]string{"1": "WifiMaster0/AccessPoint6"}); err == nil {
			t.Fatal("invalid ACL accepted")
		}
	}
}

func TestACLReplacementQuiescesAndRestoresAP(t *testing.T) {
	a := AccessList{Target: "WifiMaster0/AccessPoint6", Policy: "permit", MACs: []string{"02:00:00:00:00:02"}}
	running := "known host old 02:00:00:00:00:01\nknown host new 02:00:00:00:00:02\ninterface WifiMaster0/AccessPoint6\n    up\n    mac access-list type deny\n    mac access-list address 02:00:00:00:00:01\n!\n"
	changes, err := Preimage(Plan{AccessLists: []AccessList{a}}, running)
	if err != nil || len(changes) != 5 {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
	if _, err := Preimage(Plan{AccessLists: []AccessList{a}}, strings.ReplaceAll(running, "known host new 02:00:00:00:00:02\n", "")); err == nil {
		t.Fatal("unregistered host passed preflight")
	}
	for i, want := range []string{"down", "no mac access-list address 02:00:00:00:00:01", "mac access-list address 02:00:00:00:00:02", "mac access-list type permit", "up"} {
		if changes[i].Forward.Text != "interface "+a.Target+" "+want {
			t.Fatalf("wrong forward order: %#v", changes)
		}
	}
	for i, want := range []string{"up", "mac access-list address 02:00:00:00:00:01", "no mac access-list address 02:00:00:00:00:02", "mac access-list type deny", "down"} {
		if changes[i].Undo.Text != "interface "+a.Target+" "+want {
			t.Fatalf("wrong inverse: %#v", changes)
		}
	}
	if VerifyAccessLists(Plan{AccessLists: []AccessList{a}}, running) == nil {
		t.Fatal("wrong ACL passed health check")
	}
	good := strings.ReplaceAll(strings.ReplaceAll(running, "type deny", "type permit"), "00:01", "00:02")
	if err := VerifyAccessLists(Plan{AccessLists: []AccessList{a}}, good); err != nil {
		t.Fatal(err)
	}
	changes, err = Preimage(Plan{AccessLists: []AccessList{a}}, good)
	if err != nil || len(changes) != 0 {
		t.Fatalf("matching ACL caused AP interruption: %#v %v", changes, err)
	}
}

func TestACLAbsentDisabledAndEmptyPolicies(t *testing.T) {
	for _, tc := range []struct{ cfg, policy string }{
		{"", ""}, {"macacl.status=disabled\n", "none"},
		{strings.ReplaceAll(aclConfig, "macacl.7.acl.1.mac=02:00:00:00:00:0A\n", ""), "permit"},
		{strings.ReplaceAll(strings.ReplaceAll(aclConfig, "policy=allow", "policy=deny"), "macacl.7.acl.1.mac=02:00:00:00:00:0A\n", ""), "none"},
	} {
		p, err := BuildPlan(wlan+tc.cfg, map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if err != nil {
			t.Fatal(err)
		}
		if tc.policy == "" {
			if len(p.AccessLists) != 0 {
				t.Fatal("absent request altered native ACL")
			}
		} else if len(p.AccessLists) != 1 || p.AccessLists[0].Policy != tc.policy || len(p.AccessLists[0].MACs) != 0 {
			t.Fatal(p.AccessLists)
		}
	}
}
