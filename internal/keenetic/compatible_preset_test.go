// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCompatibleWPA2PresetScopeAndBSSRefusal(t *testing.T) {
	data, err := os.ReadFile("testdata/compatible-wpa2.cfg")
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6", "4": "WifiMaster1/AccessPoint6"}
	p, err := BuildWLANTestPlan(string(data), slots)
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("preset rejected: %v %v", err, p.Unsupported)
	}
	wantIgnored := []string{"wireless.3.dtim_period", "wireless.3.uapsd", "wireless.4.dtim_period", "wireless.4.uapsd"}
	if !reflect.DeepEqual(p.Ignored, wantIgnored) {
		t.Fatalf("ignored settings changed: %v", p.Ignored)
	}
	commands := map[string]bool{}
	for _, c := range p.Commands {
		commands[c.Text] = true
		if strings.Contains(c.String(), "synthetic-compatible-key") {
			t.Fatal("credential exposed in diagnostics")
		}
		if !strings.HasPrefix(c.Text, "interface "+slots["3"]+" ") &&
			!strings.HasPrefix(c.Text, "interface "+slots["4"]+" ") &&
			c.Text != "interface Bridge0 include "+slots["3"] &&
			c.Text != "interface Bridge0 include "+slots["4"] {
			t.Fatalf("command escaped owned WLAN scope: %s", c.String())
		}
	}
	for _, target := range slots {
		for _, setting := range []string{"encryption wpa2", "no encryption wpa3", "no pmf", "no ft enable", "up"} {
			if !commands["interface "+target+" "+setting] {
				t.Fatalf("missing baseline setting %s on %s", setting, target)
			}
		}
		if !commands["interface Bridge0 include "+target] {
			t.Fatal("missing LAN membership")
		}
	}
	enabled := strings.ReplaceAll(string(data), "bss_transition=disabled", "bss_transition=enabled")
	refused, err := BuildWLANTestPlan(enabled, slots)
	want := []string{"wireless.3.bss-transition", "wireless.4.bss-transition"}
	if err != nil || !reflect.DeepEqual(refused.Unsupported, want) {
		t.Fatalf("BSS transition no longer explicitly refused: %v %v", err, refused.Unsupported)
	}
}
