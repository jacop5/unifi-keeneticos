// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestConnectedTestPlanExcludesRadioAndServingAP(t *testing.T) {
	cfg := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Hero") + "wireless.2.ssid=Serving\nwireless.2.parent=wifi1\nwireless.2.usage=user\n"
	p, err := BuildWLANTestPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Commands {
		if strings.Contains(c.Text, " channel ") || strings.Contains(c.Text, "Serving") || strings.Contains(c.Text, "AccessPoint0") {
			t.Fatal(c)
		}
	}
	if _, err := BuildWLANTestPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint0"}); err == nil {
		t.Fatal("primary AP accepted")
	}
	if _, err := BuildWLANTestPlan(wlan, map[string]string{"1": "WifiMaster0/AccessPoint6"}); err == nil {
		t.Fatal("non-test SSID accepted")
	}
}

func TestPausedWLANDisablesOnlyMappedSlots(t *testing.T) {
	p, err := BuildWLANTestPlan("radio.1.phyname=wifi0\nradio.1.channel=auto\n", map[string]string{"1": "WifiMaster0/AccessPoint6", "2": "WifiMaster1/AccessPoint6"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Commands) != 2 || p.Commands[0].Text != "interface WifiMaster0/AccessPoint6 down" || p.Commands[1].Text != "interface WifiMaster1/AccessPoint6 down" {
		t.Fatal(p.Commands)
	}
	for _, test := range []struct {
		ssid string
		good bool
	}{{"UKN-Test-Hero", true}, {`"UKN-Test-Hero"`, true}, {"Serving", false}} {
		err := ValidateTestOwnership("interface WifiMaster0/AccessPoint6\n    ssid "+test.ssid+"\n!\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if (err == nil) != test.good {
			t.Fatalf("SSID %s: %v", test.ssid, err)
		}
	}
}
