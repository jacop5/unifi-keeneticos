// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import "testing"

func TestVerifyDoesNotAcceptUnrelatedWLAN(t *testing.T) {
	p := Plan{Commands: []Command{{Text: `interface WifiMaster0/AccessPoint1 ssid "Managed"`}, {Text: "interface WifiMaster0/AccessPoint1 up"}}}
	s := &Snapshot{VAPs: []map[string]string{{"id": "WifiMaster0/AccessPoint0", "ssid": "Other"}}}
	if err := VerifyWLANs(p, s); err == nil {
		t.Fatal("unrelated WLAN accepted")
	}
	s.VAPs = append(s.VAPs, map[string]string{"id": "WifiMaster0/AccessPoint1", "ssid": "Wrong"})
	if err := VerifyWLANs(p, s); err == nil {
		t.Fatal("wrong SSID accepted")
	}
	s.VAPs[1]["ssid"] = "Managed"
	if err := VerifyWLANs(p, s); err != nil {
		t.Fatal(err)
	}
}
