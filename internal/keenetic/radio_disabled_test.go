// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestDisabledParentRadioDoesNotRequireActiveChild(t *testing.T) {
	p := Plan{Commands: []Command{
		{Text: "interface WifiMaster0 down"}, {Text: "interface WifiMaster1 up"},
		{Text: `interface WifiMaster0/AccessPoint6 ssid "Inactive"`}, {Text: "interface WifiMaster0/AccessPoint6 up"},
		{Text: `interface WifiMaster1/AccessPoint6 ssid "Active"`}, {Text: "interface WifiMaster1/AccessPoint6 up"},
	}}
	s := &Snapshot{Radios: []map[string]string{{"id": "WifiMaster0", "state": "down"}, {"id": "WifiMaster1", "state": "up"}}, VAPs: []map[string]string{{"id": "WifiMaster1/AccessPoint6", "ssid": "Active"}}}
	if err := VerifyWLANs(p, s); err != nil {
		t.Fatal(err)
	}
	s.Radios[0]["state"] = "up"
	if VerifyWLANs(p, s) == nil {
		t.Fatal("ignored radio disable accepted")
	}
	s.Radios[0]["state"] = "down"
	s.VAPs = nil
	if VerifyWLANs(p, s) == nil {
		t.Fatal("disabled first radio waived active second radio")
	}
	s.Radios = nil
	if VerifyWLANs(p, s) == nil {
		t.Fatal("missing radio measurement accepted")
	}
	running := "interface WifiMaster0\n    down\n!\ninterface WifiMaster1\n    up\n!\ninterface WifiMaster0/AccessPoint6\n    ssid Inactive\n    up\n!\ninterface WifiMaster1/AccessPoint6\n    ssid Active\n    up\n!\n"
	if err := VerifyRadioSettings(p, running); err != nil {
		t.Fatal(err)
	}
	if err := VerifyWLANSettings(p, running); err != nil {
		t.Fatal(err)
	}
	if VerifyWLANSettings(p, strings.ReplaceAll(running, "ssid Inactive", "ssid Stale")) == nil {
		t.Fatal("inactive WLAN retained stale SSID")
	}
	if VerifyWLANSettings(p, strings.ReplaceAll(running, "ssid Inactive\n    up", "ssid Inactive\n    down")) == nil {
		t.Fatal("child administrative disable hidden by parent")
	}
}

func TestNativeSSIDReadbackHandlesQuotedAndInactiveValues(t *testing.T) {
	const ap = "interface WifiMaster0/AccessPoint6"
	p := Plan{Commands: []Command{{Text: ap + ` ssid "Test Network"`}, {Text: ap + " down"}}}
	for _, ssid := range []string{`"Test Network"`, `Test Network`} {
		if err := VerifyWLANSettings(p, ap+"\n    ssid "+ssid+"\n    down\n!\n"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lines := range []string{"    down\n", "    ssid Other\n    down\n", "    ssid \"broken\n    down\n", "    ssid Test Network\n    up\n", "    ssid Test Network\n    down\n    up\n"} {
		if VerifyWLANSettings(p, ap+"\n"+lines+"!\n") == nil {
			t.Fatal("incorrect inactive native SSID/state accepted")
		}
	}
}

func TestControllerRadioDisableRetainsChildConfiguration(t *testing.T) {
	p, err := BuildPlan(wlan+"radio.1.status=disabled\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("%v %v", err, p.Unsupported)
	}
	s := &Snapshot{Radios: []map[string]string{{"id": "WifiMaster0", "state": "down"}}}
	if err := VerifyWLANs(p, s); err != nil {
		t.Fatal(err)
	}
	s.Radios[0]["state"] = "up"
	if VerifyWLANs(p, s) == nil {
		t.Fatal("controller-requested radio disable ignored")
	}
}
