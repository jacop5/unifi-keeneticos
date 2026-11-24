// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"reflect"
	"strings"
	"testing"
)

func TestGuestPolicyCannotBecomeOrdinaryLAN(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Guest")
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
			ordinary, err := build.plan(base+"aaa.1.is_guest=false\n", slots)
			if err != nil || !reflect.DeepEqual(ordinary, baseline) {
				t.Fatalf("explicit non-guest changed plan: %v", err)
			}
			for _, status := range []string{"enabled", "disabled"} {
				p, err := build.plan(base+"aaa.1.is_guest=true\naaa.1.status="+status+"\n", slots)
				if err != nil || !reflect.DeepEqual(p.Unsupported, []string{"wireless.1.guest-policy"}) {
					t.Fatalf("guest request accepted: %v %v", p.Unsupported, err)
				}
				if _, err := Preimage(p, ""); err == nil {
					t.Fatal("guest request reached apply preparation")
				}
			}
			_, err = build.plan(base+"aaa.1.is_guest=invalid-private-value\n", slots)
			if err == nil || strings.Contains(err.Error(), "invalid-private-value") {
				t.Fatal("invalid guest flag accepted or value leaked")
			}
		})
	}
}

func TestConnectedGuestRejectionStaysScoped(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Guest")
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	unrelated := strings.ReplaceAll(base, ".1.", ".99.") + "aaa.99.is_guest=true\n"
	p, err := BuildWLANTestPlan(base+unrelated, slots)
	if err != nil || len(p.Unsupported) != 0 {
		t.Fatalf("unrelated guest WLAN affected scoped plan: %v %v", p.Unsupported, err)
	}
}
