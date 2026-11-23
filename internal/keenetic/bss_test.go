// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestBSSTransitionIsNotSilentlyTreatedAsRRM(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-BSS")
	for _, status := range []string{"", "disabled", "enabled", "invalid"} {
		p, err := BuildWLANTestPlan(base+"aaa.1.bss_transition="+status+"\naaa.1.11k.status=enabled\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if status == "invalid" {
			if err == nil {
				t.Fatal("invalid BSS transition status accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if (len(p.Unsupported) > 0) != (status == "enabled") {
			t.Fatalf("BSS transition support misreported: %q %v", status, p.Unsupported)
		}
		if status == "enabled" {
			if _, err := Preimage(p, "interface WifiMaster0/AccessPoint6\n    down\n!\n"); err == nil {
				t.Fatal("unsupported BSS transition reached application preparation")
			}
		}
		found := false
		for _, c := range p.Commands {
			found = found || c.Text == "interface WifiMaster0/AccessPoint6 rrm"
		}
		if !found {
			t.Fatal("separate 802.11k request lost")
		}
	}
}
