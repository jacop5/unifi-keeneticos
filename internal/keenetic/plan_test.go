// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

const wlan = `radio.1.phyname=wifi0
radio.1.channel=6
radio.1.ieee_mode=11nght20
wireless.1.ssid=Home WiFi
wireless.1.parent=wifi0
wireless.1.usage=user
aaa.1.ssid=Home WiFi
aaa.1.wpa=2
aaa.1.wpa.psk=secret-passphrase
`

func TestPlanRequiresExplicitSlot(t *testing.T) {
	p, err := BuildPlan(wlan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) != 1 || p.Unsupported[0] != "wireless.1.slot" {
		t.Fatalf("%#v", p)
	}
}
func TestPlanWPA2(t *testing.T) {
	p, err := BuildPlan(wlan, map[string]string{"1": "WifiMaster0/AccessPoint0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) != 0 || len(p.Commands) != 15 {
		t.Fatalf("%#v", p)
	}
	for _, c := range p.Commands {
		if strings.Contains(c.String(), "secret-passphrase") {
			t.Fatal("credential exposed")
		}
	}
	if p.Commands[2].Text != `interface WifiMaster0/AccessPoint0 ssid "Home WiFi"` {
		t.Fatal(p.Commands[2])
	}
}
func TestPlanRejectsNDMInjection(t *testing.T) {
	for _, value := range []string{"bad\"; reboot", "bad\\escape", "bad\rreboot"} {
		if _, err := quoted(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if _, err := BuildPlan(wlan, map[string]string{"1": "WifiMaster1/AccessPoint0"}); err == nil {
		t.Fatal("accepted wrong radio")
	}
}

func TestPlanSAEAndHidden(t *testing.T) {
	for _, transition := range []bool{false, true} {
		cfg := wlan + "aaa.1.wpa.key.1.mgmt=SAE\naaa.1.wpa3.support=enabled\nwireless.1.hide_ssid=true\n"
		if transition {
			cfg += "aaa.1.wpa3.transition=enabled\n"
		}
		p, err := BuildPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint0"})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Unsupported) > 0 {
			t.Fatal(p.Unsupported)
		}
		commands := ""
		for _, c := range p.Commands {
			commands += c.String() + "\n"
		}
		for _, want := range []string{" encryption wpa3\n", " hide-ssid\n"} {
			if !strings.Contains(commands, want) {
				t.Fatal(commands)
			}
		}
		if transition {
			if !strings.Contains(commands, " encryption wpa2\n") {
				t.Fatal(commands)
			}
		} else {
			if !strings.Contains(commands, " no encryption wpa2\n") || !strings.Contains(commands, " pmf force\n") {
				t.Fatal(commands)
			}
		}
	}
}

func TestPlanWLANDisabled(t *testing.T) {
	p, err := BuildPlan(wlan+"wireless.1.status=disabled\n", map[string]string{"1": "WifiMaster0/AccessPoint0"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Commands[len(p.Commands)-1].Text != "interface WifiMaster0/AccessPoint0 down" {
		t.Fatal(p.Commands)
	}
	if _, err := BuildPlan(wlan, map[string]string{"1": "WifiMaster0/AccessPoint0", "2": "WifiMaster0/AccessPoint0"}); err == nil {
		t.Fatal("duplicate ownership accepted")
	}
}

func TestIsolationAndRRM(t *testing.T) {
	p, err := BuildPlan(wlan+"wireless.1.l2_isolation=enabled\naaa.1.11k.status=enabled\n", map[string]string{"1": "WifiMaster0/AccessPoint0"})
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, c := range p.Commands {
		text += c.String() + "\n"
	}
	if !strings.Contains(text, " peer-isolation\n") || !strings.Contains(text, " rrm\n") {
		t.Fatal(text)
	}
	p, err = BuildPlan(wlan+"aaa.1.ft.status=enabled\n", map[string]string{"1": "WifiMaster0/AccessPoint0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) != 1 || p.Unsupported[0] != "wireless.1.fast-roaming" {
		t.Fatal(p.Unsupported)
	}
}

func TestTaggedNetworkCannotApplyAsUntagged(t *testing.T) {
	p, err := BuildPlan(wlan+"vlan.1.id=77\n", map[string]string{"1": "WifiMaster0/AccessPoint0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) != 1 || p.Unsupported[0] != "vlan.1.id" {
		t.Fatal(p.Unsupported)
	}
	if _, err := Preimage(p, ""); err == nil {
		t.Fatal("VLAN plan allowed to apply untagged")
	}
}

func TestTaggedBridgeWithoutVLANSectionIsRejected(t *testing.T) {
	text := wlan + "bridge.2.devname=br0.77\nbridge.2.port.1.devname=wifi0ap5\nbridge.2.port.2.devname=eth0.77\n"
	for _, build := range []func(string, map[string]string) (Plan, error){BuildPlan, BuildWLANTestPlan} {
		p, err := build(strings.ReplaceAll(text, "Home WiFi", "UKN-Test-Example"), map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Unsupported) == 0 {
			t.Fatal("tagged bridge accepted as Home network")
		}
		if _, err := Preimage(p, ""); err == nil {
			t.Fatal("tagged bridge allowed to apply")
		}
	}
}
