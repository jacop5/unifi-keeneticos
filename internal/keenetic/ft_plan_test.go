// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

const ftPair = `wireless.3.ssid=UKN-Test-FT
wireless.3.parent=wifi0
wireless.3.usage=user
aaa.3.ssid=UKN-Test-FT
aaa.3.wpa=2
aaa.3.wpa.psk=synthetic-ft-key
aaa.3.wpa.key.1.mgmt=WPA-PSK
aaa.3.ft.status=enabled
wireless.4.ssid=UKN-Test-FT
wireless.4.parent=wifi1
wireless.4.usage=user
aaa.4.ssid=UKN-Test-FT
aaa.4.wpa=2
aaa.4.wpa.psk=synthetic-ft-key
aaa.4.wpa.key.1.mgmt=WPA-PSK
aaa.4.ft.status=enabled
`

func TestExplicitFTPair(t *testing.T) {
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6", "4": "WifiMaster1/AccessPoint6"}
	domains := map[string]string{"UKN-Test-FT": "AB"}
	p, err := BuildWLANTestPlanWithFT(ftPair, slots, domains)
	if err != nil || len(p.Unsupported) > 0 {
		t.Fatalf("%v %v", err, p.Unsupported)
	}
	for _, ap := range slots {
		for _, setting := range []string{"ft mdid AB", "no ft otd", "ft enable"} {
			found := false
			for _, c := range p.Commands {
				if c.Text == "interface "+ap+" "+setting {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s", setting)
			}
		}
	}
	for _, cfg := range []string{
		strings.ReplaceAll(ftPair, "aaa.4.ft.status=enabled", "aaa.4.ft.status=disabled"),
		strings.ReplaceAll(ftPair, "aaa.4.wpa.psk=synthetic-ft-key", "aaa.4.wpa.psk=different-ft-key"),
		ftPair + "aaa.4.wpa3.support=enabled\naaa.4.wpa.key.1.mgmt=SAE\n",
		ftPair + "wireless.4.status=disabled\n",
		ftPair + "aaa.4.pmf.status=enabled\naaa.4.pmf.mode=2\n",
		ftPair + "wireless.4.schedule_enabled=enabled\n",
		strings.ReplaceAll(ftPair, "wireless.4.parent=wifi1", "wireless.4.parent=wifi0"),
		strings.ReplaceAll(ftPair, "wireless.4.ssid=UKN-Test-FT", "wireless.4.ssid=UKN-Test-Other"),
	} {
		p, err := BuildWLANTestPlanWithFT(cfg, slots, domains)
		if err == nil && len(p.Unsupported) == 0 {
			t.Fatal("incompatible FT pair accepted")
		}
		if err == nil {
			for _, c := range p.Commands {
				if c.Text == "interface WifiMaster0/AccessPoint6 ft enable" || c.Text == "interface WifiMaster1/AccessPoint6 ft enable" {
					t.Fatal("incompatible pair emitted FT enable")
				}
			}
		}
	}
	p, err = BuildWLANTestPlanWithFT(ftPair, map[string]string{"3": slots["3"]}, domains)
	if err == nil && len(p.Unsupported) == 0 {
		t.Fatal("single-band FT accepted")
	}
	p, err = BuildWLANTestPlan(ftPair, slots)
	if err == nil && len(p.Unsupported) == 0 {
		t.Fatal("FT enabled without explicit opt-in")
	}
}

func TestExplicitFTDomainValidation(t *testing.T) {
	for _, domains := range []map[string]string{
		{"UKN-Test-FT": ""}, {"UKN-Test-FT": "ABC"}, {"UKN-Test-FT": "A\n"}, {"UKN-Test-FT": "A\""},
		{"UKN-Test-FT": "AB", "Other": "AB"}, {"": "AB"},
	} {
		if _, err := BuildPlanWithOptions("", nil, PlannerOptions{FTDomains: domains}); err == nil {
			t.Fatal("invalid domain mapping accepted")
		}
	}
}

func TestFTPairRequiresOneBridge(t *testing.T) {
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6", "4": "WifiMaster1/AccessPoint6"}
	if matchingFTPair(translate.ParseCfg(ftPair), slots, map[string]string{"3": "Bridge1", "4": "Bridge2"}, "UKN-Test-FT") {
		t.Fatal("FT pair across networks accepted")
	}
}

func TestFTPairCommandsPrepareExactInverses(t *testing.T) {
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6", "4": "WifiMaster1/AccessPoint6"}
	p, err := BuildWLANTestPlanWithFT(ftPair, slots, map[string]string{"UKN-Test-FT": "AB"})
	if err != nil {
		t.Fatal(err)
	}
	ft := Plan{}
	lastUp, firstFT := -1, -1
	before, after := "", ""
	for i, c := range p.Commands {
		if strings.HasSuffix(c.Text, " up") {
			lastUp = i
		}
		if strings.Contains(c.Text, " ft ") {
			if firstFT < 0 {
				firstFT = i
			}
			ft.Commands = append(ft.Commands, c)
		}
	}
	if firstFT <= lastUp {
		t.Fatal("FT enabled before both WLANs configured")
	}
	for _, ap := range []string{slots["3"], slots["4"]} {
		before += "interface " + ap + "\n    ft mdid XY\n    ft otd\n    up\n!\n"
		after += "interface " + ap + "\n    ft mdid AB\n    ft enable\n    up\n!\n"
	}
	changes, err := Preimage(ft, before)
	if err != nil || len(changes) != 6 {
		t.Fatalf("inverses: %v %v", changes, err)
	}
	for i, setting := range []string{"ft mdid XY", "ft otd", "no ft enable"} {
		if changes[i].Undo.Text != "interface "+slots["3"]+" "+setting {
			t.Fatalf("incorrect inverse %d", i)
		}
	}
	if VerifyWLANSettings(ft, before) == nil {
		t.Fatal("stale FT state accepted")
	}
	if err := VerifyWLANSettings(ft, after); err != nil {
		t.Fatal(err)
	}
}
