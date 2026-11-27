// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

// Production WLAN-only config: arbitrary SSID, a serving unmapped WLAN and
// hostile/invalid radio-wide settings that must never reach the plan.
const wlanOnlyCfg = `radio.1.phyname=wifi0
radio.1.channel=999
radio.1.status=bogus
radio.1.countrycode=ZZ
radio.1.txpower=100
radio.1.ieee_mode=11axhe160
radio.2.phyname=wifi9
radio.2.channel=abc
wireless.1.ssid=Corp Net
wireless.1.parent=wifi0
wireless.1.usage=user
aaa.1.ssid=Corp Net
aaa.1.wpa=2
aaa.1.wpa.psk=production-passphrase
wireless.2.ssid=Serving
wireless.2.parent=wifi1
wireless.2.usage=user
aaa.2.ssid=Serving
aaa.2.wpa=2
aaa.2.wpa.psk=another-passphrase
`

func wlanOnlyCommands(p Plan) string {
	text := ""
	for _, c := range p.Commands {
		text += c.Text + "\n"
	}
	return text
}

func assertNoRadioCommands(t *testing.T, p Plan) {
	t.Helper()
	for _, c := range p.Commands {
		if strings.Contains(c.Text, "WifiMaster0 ") || strings.Contains(c.Text, "WifiMaster1 ") ||
			strings.Contains(c.Text, " channel") || strings.Contains(c.Text, "country-code") ||
			strings.Contains(c.Text, "txpower") {
			t.Fatalf("radio-wide command in wlan-only plan: %s", c.Text)
		}
	}
}

func TestWLANOnlyAcceptsArbitrarySSIDAndPrimarySlot(t *testing.T) {
	p, err := BuildWLANPlanWithOptions(wlanOnlyCfg, map[string]string{"1": "WifiMaster0/AccessPoint0"}, PlannerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) != 0 {
		t.Fatalf("unsupported settings leaked into plan: %v", p.Unsupported)
	}
	assertNoRadioCommands(t, p)
	text := wlanOnlyCommands(p)
	if !strings.Contains(text, `interface WifiMaster0/AccessPoint0 ssid "Corp Net"`+"\n") ||
		!strings.Contains(text, "interface WifiMaster0/AccessPoint0 up\n") {
		t.Fatalf("primary slot provisioning missing:\n%s", text)
	}
	if strings.Contains(text, "Serving") || strings.Contains(text, "AccessPoint6") {
		t.Fatalf("unmapped WLAN leaked into plan:\n%s", text)
	}
	for _, c := range p.Commands {
		if strings.Contains(c.String(), "production-passphrase") {
			t.Fatal("credential exposed")
		}
	}
}

func TestWLANOnlyRejectsEmptyInvalidAndDuplicateSlots(t *testing.T) {
	for _, slots := range []map[string]string{
		nil,
		{},
		{"1": ""},
		{"1": "WifiMaster0/AccessPoint9"},
		{"1": "WifiMaster2/AccessPoint1"},
		{"1": "AccessPoint1"},
		{"1": "eth0"},
		{"1": "WifiMaster0/AccessPoint0", "2": "WifiMaster0/AccessPoint0"},
	} {
		if _, err := BuildWLANPlanWithOptions(wlanOnlyCfg, slots, PlannerOptions{}); err == nil {
			t.Fatalf("invalid slots accepted: %v", slots)
		}
	}
}

func TestWLANOnlyExcludesAllRadioCommandsDespiteHostileSettings(t *testing.T) {
	p, err := BuildWLANPlanWithOptions(wlanOnlyCfg, map[string]string{"1": "WifiMaster0/AccessPoint7"}, PlannerOptions{})
	if err != nil {
		t.Fatalf("hostile radio settings broke wlan-only plan: %v", err)
	}
	assertNoRadioCommands(t, p)
	for _, c := range p.Commands {
		parts := strings.SplitN(c.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" && parts[1] != "WifiMaster0/AccessPoint7" && parts[1] != "Bridge0" {
			t.Fatalf("command outside mapped slot: %s", c.Text)
		}
	}
}

func TestWLANOnlyMissingMappedSSIDEmitsDown(t *testing.T) {
	slots := map[string]string{"1": "WifiMaster0/AccessPoint0", "3": "WifiMaster1/AccessPoint2", "4": "WifiMaster1/AccessPoint5"}
	p, err := BuildWLANPlanWithOptions(wlanOnlyCfg, slots, PlannerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text := wlanOnlyCommands(p)
	if !strings.Contains(text, "interface WifiMaster1/AccessPoint2 down\n") || !strings.Contains(text, "interface WifiMaster1/AccessPoint5 down\n") {
		t.Fatalf("missing mapped WLANs did not emit down:\n%s", text)
	}
	if strings.Contains(text, "interface WifiMaster1/AccessPoint2 ssid") {
		t.Fatal("down slot also configured")
	}
	// Sorted pause order, consistent with the test planner.
	downs := []string{}
	for _, c := range p.Commands {
		if strings.HasSuffix(c.Text, " down") {
			downs = append(downs, c.Text)
		}
	}
	if len(downs) != 2 || downs[0] != "interface WifiMaster1/AccessPoint2 down" || downs[1] != "interface WifiMaster1/AccessPoint5 down" {
		t.Fatalf("down commands unsorted: %v", downs)
	}
}

func TestWLANOnlyHonorsBridgeMappingGuards(t *testing.T) {
	slots := map[string]string{"1": "WifiMaster0/AccessPoint0"}
	tagged := wlanOnlyCfg + "wireless.1.devname=wifi0ap1\nvlan.1.id=77\nvlan.1.devname=eth0\nbridge.2.devname=br0.77\nbridge.2.port.1.devname=wifi0ap1\nbridge.2.port.2.devname=eth0.77\n"
	p, err := BuildWLANPlanWithOptions(tagged, slots, PlannerOptions{Networks: map[string]string{"77": "Bridge2"}})
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("valid tagged mapping rejected: %v %v", p.Unsupported, err)
	}
	if !strings.Contains(wlanOnlyCommands(p), "interface Bridge2 include WifiMaster0/AccessPoint0\n") {
		t.Fatalf("mapped bridge include missing:\n%s", wlanOnlyCommands(p))
	}
	// An undeclared/unused VLAN mapping must stay unsupported, never degrade.
	p, err = BuildWLANPlanWithOptions(wlanOnlyCfg+"wireless.1.devname=wifi0ap1\n", slots, PlannerOptions{Networks: map[string]string{"77": "Bridge2"}})
	if err != nil || !strings.Contains(strings.Join(p.Unsupported, ","), "wireless.1.network") {
		t.Fatalf("incomplete mapping guard lost: %v %v", p.Unsupported, err)
	}
	// Invalid mappings are rejected by the ordinary planner guard.
	if _, err := BuildWLANPlanWithOptions(wlanOnlyCfg, slots, PlannerOptions{Networks: map[string]string{"77": "notabridge"}}); err == nil {
		t.Fatal("invalid bridge mapping accepted")
	}
	if _, err := BuildWLANPlanWithOptions(wlanOnlyCfg, slots, PlannerOptions{Networks: map[string]string{"x": "Bridge2"}}); err == nil {
		t.Fatal("invalid VLAN id accepted")
	}
}

func TestWLANOnlyHonorsFTOptions(t *testing.T) {
	cfg := strings.ReplaceAll(ftPair, "UKN-Test-FT", "CorpFT")
	slots := map[string]string{"3": "WifiMaster0/AccessPoint0", "4": "WifiMaster1/AccessPoint1"}
	p, err := BuildWLANPlanWithOptions(cfg, slots, PlannerOptions{FTDomains: map[string]string{"CorpFT": "AB"}})
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("FT options not honored: %v %v", err, p.Unsupported)
	}
	for _, ap := range []string{"WifiMaster0/AccessPoint0", "WifiMaster1/AccessPoint1"} {
		if !strings.Contains(wlanOnlyCommands(p), "interface "+ap+" ft mdid AB\n") {
			t.Fatalf("FT command missing for %s:\n%s", ap, wlanOnlyCommands(p))
		}
	}
	// Without an explicit domain the ordinary unsupported guard applies.
	p, err = BuildWLANPlanWithOptions(cfg, slots, PlannerOptions{})
	if err != nil || !strings.Contains(strings.Join(p.Unsupported, ","), "fast-roaming") {
		t.Fatalf("FT without domain not unsupported: %v %v", p.Unsupported, err)
	}
}

func TestWLANTestModeStillRefusesPrimaryAndNonTestNames(t *testing.T) {
	if _, err := BuildWLANTestPlan(wlanOnlyCfg, map[string]string{"1": "WifiMaster0/AccessPoint0"}); err == nil {
		t.Fatal("test mode accepted a primary slot")
	}
	if _, err := BuildWLANTestPlan(wlanOnlyCfg, map[string]string{"1": "WifiMaster0/AccessPoint6"}); err == nil {
		t.Fatal("test mode accepted a non-test SSID")
	}
}
