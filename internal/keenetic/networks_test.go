// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"strings"
	"testing"
)

func TestEnabledNetworkMappingsRequireMembershipWithoutVLANDeclarations(t *testing.T) {
	const target = "WifiMaster0/AccessPoint6"
	base := wlan + "wireless.1.devname=wifi0ap5\n"
	slots, networks := map[string]string{"1": target}, map[string]string{"77": "Bridge2"}
	for _, cfg := range []string{base, base + "bridge.2.devname=br0\nbridge.2.port.1.devname=unrelated\n"} {
		p, err := BuildPlanWithNetworks(cfg, slots, networks)
		if err != nil || !strings.Contains(strings.Join(p.Unsupported, ","), "wireless.1.network") {
			t.Fatalf("missing membership did not block plan: %v %v", p.Unsupported, err)
		}
		mutations := 0
		err = ApplyPlan(context.Background(), func(_ context.Context, command string) ([]byte, error) {
			if command != "show running-config" {
				mutations++
			}
			return []byte("interface Bridge0\n    up\n!\ninterface WifiMaster0\n    up\n!\ninterface " + target + "\n    down\n!\n"), nil
		}, p, t.TempDir()+"/rollback.json", func(context.Context) error { t.Fatal("unsupported plan reached health"); return nil })
		if err == nil || mutations != 0 {
			t.Fatal("unsupported network plan reached native mutation")
		}
	}
	for _, tc := range []struct {
		cfg     string
		mapping map[string]string
	}{
		{base, nil}, // Existing untagged behavior remains when mappings are off.
		{base + "bridge.2.devname=br0\nbridge.2.port.1.devname=wifi0ap5\n", networks},
	} {
		p, err := BuildPlanWithNetworks(tc.cfg, slots, tc.mapping)
		if err != nil || len(p.Unsupported) != 0 {
			t.Fatalf("valid untagged control rejected: %v %v", p.Unsupported, err)
		}
		found := false
		for _, command := range p.Commands {
			found = found || command.Text == "interface Bridge0 include "+target
		}
		if !found {
			t.Fatal("valid untagged binding lost")
		}
	}
}

const taggedWLAN = wlan + "wireless.1.devname=wifi0ap5\nvlan.1.id=77\nvlan.1.devname=eth0\nbridge.2.devname=br0.77\nbridge.2.port.1.devname=wifi0ap5\nbridge.2.port.2.devname=eth0.77\n"

func TestExplicitVLANBridgePlanAndRollback(t *testing.T) {
	p, err := BuildPlanWithNetworks(taggedWLAN, map[string]string{"1": "WifiMaster0/AccessPoint6"}, map[string]string{"77": "Bridge2"})
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("plan=%#v err=%v", p, err)
	}
	var include Command
	for _, command := range p.Commands {
		if strings.Contains(command.Text, " include ") {
			include = command
		}
	}
	if include.Text != "interface Bridge2 include WifiMaster0/AccessPoint6" {
		t.Fatal(include)
	}
	if _, err := Preimage(Plan{Commands: []Command{include}}, "interface WifiMaster0/AccessPoint6\n    down\n!\n"); err == nil {
		t.Fatal("missing native VLAN bridge accepted")
	}
	changes, err := Preimage(Plan{Commands: []Command{include}}, "interface Bridge2\n    up\n!\ninterface WifiMaster0/AccessPoint6\n    down\n!\n")
	if err != nil || len(changes) != 1 || changes[0].Undo.Text != "interface Bridge2 no include WifiMaster0/AccessPoint6" {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
	if _, err := Preimage(Plan{Commands: []Command{include}}, "interface Bridge2\n    up\n!\ninterface Bridge0\n    include TestAP\n!\ninterface WifiMaster0/AccessPoint6\n    rename TestAP\n    down\n!\n"); err == nil {
		t.Fatal("moving an existing Home AP into a VLAN was allowed")
	}
}

func TestVLANRemainsBlockedInConnectedWiFiTestMode(t *testing.T) {
	p, err := BuildWLANTestPlan(strings.ReplaceAll(taggedWLAN, "Home WiFi", "UKN-Test-VLAN"), map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) == 0 {
		t.Fatal("VLAN configuration bypassed the connected Wi-Fi test guard")
	}
}

func TestVLANMappingRejectsPartialAndUnsafeNetworks(t *testing.T) {
	for _, networks := range []map[string]string{{"77": "Bridge0"}, {"077": "Bridge2"}, {"4095": "Bridge2"}, {"77": "Bridge2; reboot"}, {"77": "Bridge2", "78": "Bridge2"}} {
		if _, err := BuildPlanWithNetworks(taggedWLAN, nil, networks); err == nil {
			t.Fatalf("unsafe mapping accepted: %v", networks)
		}
	}
	for _, text := range []string{
		strings.ReplaceAll(taggedWLAN, "vlan.1.id=77\n", ""),
		strings.ReplaceAll(taggedWLAN, "bridge.2.devname=br0.77\n", ""),
		strings.ReplaceAll(taggedWLAN, "bridge.2.port.1.devname=wifi0ap5\n", ""),
		strings.ReplaceAll(taggedWLAN, "bridge.2.port.2.devname=eth0.77\n", "bridge.2.port.2.devname=eth0.78\n"),
		taggedWLAN + "bridge.3.devname=br0\nbridge.3.port.1.devname=wifi0ap5\n",
	} {
		p, err := BuildPlanWithNetworks(text, map[string]string{"1": "WifiMaster0/AccessPoint6"}, map[string]string{"77": "Bridge2"})
		if err == nil && len(p.Unsupported) == 0 {
			t.Fatal("partial or ambiguous VLAN configuration accepted")
		}
	}
}

func TestVLANHealthChecksActualBridge(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface Bridge2 include WifiMaster0/AccessPoint6"}}}
	good := "interface WifiMaster0/AccessPoint6\n    rename TestAP\n!\ninterface Bridge2\n    include TestAP\n!\n"
	if err := VerifyBridgeMembership(p, good); err != nil {
		t.Fatal(err)
	}
	for _, running := range []string{"", strings.ReplaceAll(good, "Bridge2", "Bridge0"), good + "interface Bridge0\n    include TestAP\n!\n"} {
		if err := VerifyBridgeMembership(p, running); err == nil {
			t.Fatal("missing or incorrect network binding passed health check")
		}
	}
}
