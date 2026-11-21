// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestExplicitOpenAndOWE(t *testing.T) {
	base := "wireless.1.ssid=UKN-Test-Security\nwireless.1.parent=wifi0\nwireless.1.usage=user\naaa.1.ssid=UKN-Test-Security\naaa.1.security=open\n"
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	for _, owe := range []bool{false, true} {
		cfg := base
		if owe {
			cfg += "aaa.1.wpa3.support=enabled\n"
		}
		p, err := BuildPlan(cfg, slots)
		if err != nil || len(p.Unsupported) != 0 {
			t.Fatalf("%v %v", err, p.Unsupported)
		}
		text := ""
		for _, c := range p.Commands {
			text += c.Text + "\n"
		}
		if !strings.Contains(text, "no authentication wpa-psk\n") {
			t.Fatal("old key retained")
		}
		if owe {
			if !strings.Contains(text, " encryption owe\n") || !strings.Contains(text, " pmf force\n") || strings.Contains(text, " no encryption enable\n") {
				t.Fatal(text)
			}
		} else {
			if !strings.Contains(text, " no encryption enable\n") {
				t.Fatal(text)
			}
		}
	}
	for _, cfg := range []string{strings.ReplaceAll(base, "security=open", "security=wep128"), strings.ReplaceAll(base, "aaa.1.security=open\n", ""), base + "aaa.1.wpa.key.1.mgmt=WPA-EAP\n"} {
		p, err := BuildPlan(cfg, slots)
		if err == nil && len(p.Unsupported) == 0 {
			t.Fatal("unsupported/ambiguous security accepted")
		}
	}
}

func TestRawWPA2Key(t *testing.T) {
	cfg := strings.ReplaceAll(wlan, "secret-passphrase", strings.Repeat("a", 64))
	if _, err := BuildPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint6"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.ReplaceAll(cfg, strings.Repeat("a", 64), strings.Repeat("z", 64)), cfg + "aaa.1.wpa.key.1.mgmt=SAE\n"} {
		if _, err := BuildPlan(bad, map[string]string{"1": "WifiMaster0/AccessPoint6"}); err == nil {
			t.Fatal("invalid raw key accepted")
		}
	}
}

func TestOWEPreimageRestoresKeyAndEncryption(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0/AccessPoint6 no authentication wpa-psk", Secret: true}, {Text: "interface WifiMaster0/AccessPoint6 encryption owe"}}}
	c, err := Preimage(p, "interface WifiMaster0/AccessPoint6\n    authentication wpa-psk stored-key\n    encryption wpa2\n!\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c[0].Undo.Text != "interface WifiMaster0/AccessPoint6 authentication wpa-psk stored-key" || !c[0].Undo.Secret || c[1].Undo.Text != "interface WifiMaster0/AccessPoint6 no encryption owe" {
		t.Fatalf("%#v", c)
	}
}
