// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestWLANFlagsRejectMalformedValues(t *testing.T) {
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	for _, key := range []string{"wireless.1.hide_ssid", "aaa.1.hide_ssid", "wireless.1.l2_isolation", "aaa.1.11k.status"} {
		for _, value := range []string{"invalid", "on", "1"} {
			if _, err := BuildPlan(wlan+key+"="+value+"\n", slots); err == nil {
				t.Fatalf("malformed %s accepted", key)
			}
		}
	}
	p, err := BuildPlan(wlan+"wireless.1.hide_ssid=true\nwireless.1.l2_isolation=enabled\naaa.1.11k.status=enabled\n", slots)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, c := range p.Commands {
		text += c.Text + "\n"
	}
	for _, suffix := range []string{"hide-ssid", "peer-isolation", "rrm"} {
		if !strings.Contains(text, "interface WifiMaster0/AccessPoint6 "+suffix+"\n") {
			t.Fatalf("valid %s flag lost", suffix)
		}
	}
}
