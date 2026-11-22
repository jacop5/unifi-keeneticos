// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestWMMRequestsAreExplicitScopedAndValidated(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-WMM")
	for _, value := range []string{"", "enabled", "disabled", "invalid", "enabled; reboot"} {
		p, err := BuildWLANTestPlan(base+"wireless.1.wmm="+value+"\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if value == "invalid" || value == "enabled; reboot" {
			if err == nil {
				t.Fatal("unrecognized WMM request accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, c := range p.Commands {
			if strings.HasSuffix(c.Text, " wmm") {
				got = append(got, c.Text)
			}
		}
		if value == "" {
			if len(got) != 0 {
				t.Fatal("unspecified WMM overwrote native state")
			}
			continue
		}
		want := "interface WifiMaster0/AccessPoint6 wmm"
		if value == "disabled" {
			want = "interface WifiMaster0/AccessPoint6 no wmm"
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("WMM escaped its explicitly mapped AP: %v", got)
		}
	}
}

func TestWMMInverseAndNativeReadback(t *testing.T) {
	const root = "interface WifiMaster0/AccessPoint5"
	for _, enabled := range []bool{false, true} {
		setting, old := "wmm", ""
		if !enabled {
			setting, old = "no wmm", "    wmm\n"
		}
		p := Plan{Commands: []Command{{Text: root + " " + setting}}}
		before := root + "\n" + old + "    down\n!\n"
		changes, err := Preimage(p, before)
		if err != nil || len(changes) != 1 {
			t.Fatalf("WMM inverse missing: %v", err)
		}
		if VerifyWLANSettings(p, before) == nil {
			t.Fatal("ignored native WMM change passed verification")
		}
		after := root + "\n    down\n!\n"
		if enabled {
			after = root + "\n    wmm\n    down\n!\n"
		}
		if err := VerifyWLANSettings(p, after); err != nil {
			t.Fatal(err)
		}
		inverse := "wmm"
		if enabled {
			inverse = "no wmm"
		}
		if changes[0].Undo.Text != root+" "+inverse {
			t.Fatal("WMM rollback lost previous state")
		}
		if changes, err := Preimage(p, after); err != nil || len(changes) != 0 {
			t.Fatal("unchanged WMM generated mutations")
		}
	}
}
