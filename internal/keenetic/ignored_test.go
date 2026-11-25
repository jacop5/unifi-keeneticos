// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"reflect"
	"strings"
	"testing"
)

func TestGroupRekeyRequestIsVisibleWithoutSharedRadioChanges(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Rekey")
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	for _, build := range []struct {
		name string
		plan func(string, map[string]string) (Plan, error)
	}{{"normal", BuildPlan}, {"connected-test", BuildWLANTestPlan}} {
		t.Run(build.name, func(t *testing.T) {
			baseline, err := build.plan(base, slots)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"3600", "0", "private-invalid-value"} {
				p, err := build.plan(base+"aaa.1.wpa.group_rekey="+value+"\naaa.99.wpa.group_rekey=unowned-value\n", slots)
				if err != nil || !reflect.DeepEqual(p.Ignored, []string{"aaa.1.wpa.group_rekey"}) {
					t.Fatalf("rekey diagnostics: %v %v", p.Ignored, err)
				}
				if !reflect.DeepEqual(p.Commands, baseline.Commands) || !reflect.DeepEqual(p.Unsupported, baseline.Unsupported) {
					t.Fatal("ignored rekey request changed native planning")
				}
			}
		})
	}
}

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

func TestUntranslatedRatesAndShapingAreVisibleWithoutChangingCommands(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Ignored") + "wireless.1.devname=wifi0ap6\n"
	extra := `wireless.1.minrate_data=12000
wireless.1.mgmt_rate=12000
wireless.1.beacon_rate=6000
wireless.1.minrate_cck_rates.status=false
qos.status=enabled
qos.vap.7.devname=wifi0ap6
qos.vap.7.dwnlink.maxspeed=12345
qos.vap.7.uplink.0.maxspeed=6789
qos.vap.7.uplink.23.maxspeed=private-invalid-value
qos.vap.8.devname=wifi1ap0
qos.vap.8.dwnlink.maxspeed=unrelated-private-value
qos.vap.8.uplink.1.maxspeed=9999
`
	want := []string{"wireless.1.minrate_data", "wireless.1.mgmt_rate", "wireless.1.beacon_rate",
		"wireless.1.minrate_cck_rates.status", "qos.vap.7.dwnlink.maxspeed",
		"qos.vap.7.uplink.0.maxspeed", "qos.vap.7.uplink.23.maxspeed"}
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	for _, build := range []struct {
		name string
		plan func(string, map[string]string) (Plan, error)
	}{{"normal", BuildPlan}, {"connected-test", BuildWLANTestPlan}} {
		t.Run(build.name, func(t *testing.T) {
			baseline, err := build.plan(base, slots)
			if err != nil {
				t.Fatal(err)
			}
			p, err := build.plan(base+extra, slots)
			if err != nil || !reflect.DeepEqual(p.Ignored, want) {
				t.Fatalf("ignored=%v err=%v", p.Ignored, err)
			}
			if !reflect.DeepEqual(p.Commands, baseline.Commands) || len(p.Unsupported) != 0 {
				t.Fatal("reporting changed native planning")
			}
			disabled := strings.ReplaceAll(extra, "qos.status=enabled", "qos.status=disabled")
			p, err = build.plan(base+disabled, slots)
			if err != nil || !reflect.DeepEqual(p.Ignored, want[:4]) {
				t.Fatalf("disabled shaping reported as requested: %v %v", p.Ignored, err)
			}
		})
	}
}
