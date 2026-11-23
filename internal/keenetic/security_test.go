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

func TestPersonalAuthenticationMethodsRequireExactTokens(t *testing.T) {
	for _, method := range []string{"SAE-UNKNOWN", "NOT-SAE", "FT-SAE", "WPA-EAP", "WPA-PSK UNKNOWN", "UNKNOWN"} {
		p, err := BuildPlan(wlan+"aaa.1.wpa.key.1.mgmt="+method+"\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if err == nil && len(p.Unsupported) == 0 {
			t.Fatalf("unsupported method %q was accepted", method)
		}
	}
	for _, method := range []string{"WPA-PSK", "SAE", "WPA-PSK SAE"} {
		p, err := BuildPlan(wlan+"aaa.1.wpa.key.1.mgmt="+method+"\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if err != nil || len(p.Unsupported) != 0 {
			t.Fatalf("supported method %q rejected: %v %v", method, err, p.Unsupported)
		}
	}
	p, err := BuildPlan(wlan+"aaa.1.wpa.key.1.mgmt=WPA-PSK\naaa.1.wpa.key.4.mgmt=UNKNOWN\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err == nil && len(p.Unsupported) == 0 {
		t.Fatal("unknown secondary method was ignored")
	}
}

func TestControllerSAEWireShapeAndWPA3FastRoaming(t *testing.T) {
	// Sanitized shape observed from Network 10.5.67, with a synthetic key.
	cfg := `wireless.3.ssid=UKN-Test-SAE
wireless.3.parent=wifi1
wireless.3.usage=user
aaa.3.ssid=UKN-Test-SAE
aaa.3.wpa=2
aaa.3.wpa.psk=synthetic-test-key
aaa.3.wpa.key.1.mgmt=SAE
aaa.3.wpa3.support=enabled
aaa.3.wpa3.transition=disabled
aaa.3.wpa3.ft.status=disabled
aaa.3.pmf.status=enabled
aaa.3.pmf.mode=2
aaa.3.bss_transition=disabled
aaa.3.ft.status=disabled
wireless.1.ssid=Unmapped
wireless.1.parent=wifi0
wireless.1.usage=user
aaa.1.bss_transition=enabled
`
	slots := map[string]string{"3": "WifiMaster1/AccessPoint6"}
	p, err := BuildWLANTestPlan(cfg, slots)
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("captured SAE shape rejected: %v %v", err, p.Unsupported)
	}
	text := ""
	for _, c := range p.Commands {
		text += c.String() + "\n"
	}
	for _, want := range []string{" encryption wpa3\n", " no encryption wpa2\n", " pmf force\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(text, "synthetic-test-key") || strings.Contains(text, "WifiMaster0/") {
		t.Fatal("key leaked or unrelated WLAN included")
	}
	for _, status := range []string{"enabled", "invalid"} {
		changed := strings.ReplaceAll(cfg, "wpa3.ft.status=disabled", "wpa3.ft.status="+status)
		p, err := BuildWLANTestPlan(changed, slots)
		if status == "enabled" && err == nil && len(p.Unsupported) == 0 {
			t.Fatal("WPA3 FT silently ignored")
		}
		if status == "invalid" && err == nil {
			t.Fatal("invalid WPA3 FT accepted")
		}
	}
}

func TestMalformedWPA3SwitchCannotDowngradeOWEToOpen(t *testing.T) {
	open := "wireless.1.ssid=UKN-Test-OWE\nwireless.1.parent=wifi0\nwireless.1.usage=user\naaa.1.ssid=UKN-Test-OWE\naaa.1.security=open\n"
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	for _, base := range []string{open, wlan} {
		for _, key := range []string{"wpa3.support", "wpa3.transition"} {
			for _, value := range []string{"enable", "true", "unknown", "enabled; reboot"} {
				if _, err := BuildPlan(base+"aaa.1."+key+"="+value+"\n", slots); err == nil {
					t.Fatalf("ambiguous %s=%q accepted", key, value)
				}
			}
		}
	}
	for _, value := range []string{"enabled", "disabled", ""} {
		p, err := BuildPlan(open+"aaa.1.wpa3.support="+value+"\n", slots)
		if err != nil || len(p.Unsupported) != 0 {
			t.Fatalf("valid switch rejected: %s %v", value, err)
		}
		text := ""
		for _, c := range p.Commands {
			text += c.Text + "\n"
		}
		if (value == "enabled") != strings.Contains(text, "interface WifiMaster0/AccessPoint6 encryption owe\n") {
			t.Fatal("OWE/open selection changed")
		}
	}
}
