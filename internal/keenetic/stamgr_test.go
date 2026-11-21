// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestMinimumRSSIMappedByBandAndConfinedToSlots(t *testing.T) {
	second := strings.ReplaceAll(strings.ReplaceAll(wlan, ".1.", ".2."), "wifi0", "wifi1")
	cfg := strings.ReplaceAll(wlan+second, "Home WiFi", "UKN-Test-RSSI") + "stamgr.1.radio=ng\nstamgr.1.minrssi.status=false\nstamgr.5.radio=na\nstamgr.5.minrssi.status=true\nstamgr.5.minrssi.rssi=20\n"
	p, err := BuildWLANTestPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint6", "2": "WifiMaster1/AccessPoint6"})
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("%v %v", err, p.Unsupported)
	}
	var thresholds []string
	for _, c := range p.Commands {
		if strings.HasPrefix(c.Text, "interface WifiMaster0 ") || strings.HasPrefix(c.Text, "interface WifiMaster1 ") {
			t.Fatal("station-manager request changed a shared radio")
		}
		if strings.Contains(c.Text, "rssi-threshold") {
			thresholds = append(thresholds, c.Text)
		}
	}
	if len(thresholds) != 2 || thresholds[0] != "interface WifiMaster0/AccessPoint6 no rssi-threshold" || thresholds[1] != "interface WifiMaster1/AccessPoint6 rssi-threshold -75" {
		t.Fatal(thresholds)
	}
}

func TestMinimumRSSIValidationAndOmission(t *testing.T) {
	off, unsupported, err := minimumRSSI(map[string]string{"stamgr.status": "disabled", "stamgr.1.radio": "ng", "stamgr.1.minrssi.status": "true", "stamgr.1.minrssi.rssi": "bad-stale-value"})
	if err != nil || len(unsupported) != 0 || off["ng"] != "no rssi-threshold" || off["na"] != "no rssi-threshold" {
		t.Fatal("global station-manager disable did not override stale entries")
	}
	for input, want := range map[string]string{"-100": "rssi-threshold -100", "-75": "rssi-threshold -75", "20": "rssi-threshold -75", "0": "no rssi-threshold"} {
		thresholds, unsupported, err := minimumRSSI(map[string]string{"stamgr.1.radio": "ng", "stamgr.1.minrssi.status": "true", "stamgr.1.minrssi.rssi": input})
		if err != nil || len(unsupported) != 0 || thresholds["ng"] != want {
			t.Fatalf("%q: %v %v %v", input, thresholds, unsupported, err)
		}
	}
	for _, input := range []string{"", "-101", "95", "100", "-75; reboot"} {
		if _, _, err := minimumRSSI(map[string]string{"stamgr.1.radio": "ng", "stamgr.1.minrssi.status": "true", "stamgr.1.minrssi.rssi": input}); err == nil {
			t.Fatalf("invalid RSSI accepted: %q", input)
		}
	}
	thresholds, _, err := minimumRSSI(map[string]string{"stamgr.1.radio": "ng"})
	if err != nil || len(thresholds) != 0 {
		t.Fatal("absent request changed a native threshold")
	}
	if _, _, err := minimumRSSI(map[string]string{"stamgr.1.radio": "ng", "stamgr.2.radio": "ng"}); err == nil {
		t.Fatal("ambiguous station-manager band accepted")
	}
	_, unsupported, err = minimumRSSI(map[string]string{"stamgr.1.radio": "na", "stamgr.1.loadbalance.status": "true"})
	if err != nil || len(unsupported) != 1 {
		t.Fatal("unsupported load balancing silently accepted")
	}
}

func TestMinimumRSSIRollbackAndReadback(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0/AccessPoint5 rssi-threshold -75"}}}
	running := "interface WifiMaster0/AccessPoint5\n    rssi-threshold -80\n    down\n!\n"
	changes, err := Preimage(p, running)
	if err != nil || len(changes) != 1 || changes[0].Undo.Text != "interface WifiMaster0/AccessPoint5 rssi-threshold -80" {
		t.Fatalf("%v %v", changes, err)
	}
	if VerifyWLANSettings(p, running) == nil || VerifyWLANSettings(p, strings.ReplaceAll(running, "-80", "-75")) != nil {
		t.Fatal("minimum RSSI confirmation ignored actual threshold")
	}
	p.Commands[0].Text = "interface WifiMaster0/AccessPoint5 no rssi-threshold"
	for _, block := range []string{strings.ReplaceAll(running, "-80", "0"), strings.ReplaceAll(running, "    rssi-threshold -80\n", "")} {
		if err := VerifyWLANSettings(p, block); err != nil {
			t.Fatal(err)
		}
	}
}
