// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"reflect"
	"strings"
	"testing"
)

func TestKnownUntranslatedSettingsAreVisibleAndScoped(t *testing.T) {
	cfg := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Ignored") + `radio.1.phyname=wifi0
radio.1.txpower=17
wireless.1.dtim_period=3
wireless.1.uapsd=disabled
wireless.99.dtim_period=5
wireless.99.uapsd=enabled
`
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	p, err := BuildPlan(cfg, slots)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"radio.1.txpower", "wireless.1.dtim_period", "wireless.1.uapsd"}
	if !reflect.DeepEqual(p.Ignored, want) {
		t.Fatalf("ignored=%v", p.Ignored)
	}
	scoped, err := BuildWLANTestPlan(cfg, slots)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scoped.Ignored, want[1:]) {
		t.Fatalf("test scope=%v", scoped.Ignored)
	}
	baseline, err := BuildWLANTestPlan(strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Ignored"), slots)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scoped.Commands, baseline.Commands) {
		t.Fatal("diagnostics changed native commands")
	}
}
