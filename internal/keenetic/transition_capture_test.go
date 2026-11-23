// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"os"
	"strings"
	"testing"
)

func TestCapturedTransitionModePreservesWPA2AndOptionalPMF(t *testing.T) {
	data, err := os.ReadFile("testdata/network-10.5.67-transition.cfg")
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6", "4": "WifiMaster1/AccessPoint6"}
	plan, err := BuildWLANTestPlan(string(data), slots)
	if err != nil || len(plan.Unsupported) != 0 {
		t.Fatalf("captured transition shape rejected: %v %v", err, plan.Unsupported)
	}
	if len(plan.Ignored) != 4 {
		t.Fatalf("DTIM/U-APSD diagnostics lost: %v", plan.Ignored)
	}
	text := ""
	for _, c := range plan.Commands {
		text += c.String() + "\n"
	}
	if strings.Contains(text, "synthetic-transition-key") {
		t.Fatal("credential leaked")
	}
	before := "interface Bridge0\n    include WifiMaster0/AccessPoint6\n    include WifiMaster1/AccessPoint6\n    up\n!\n"
	after := before
	for _, ap := range []string{slots["3"], slots["4"]} {
		root := "interface " + ap
		for _, setting := range []string{"encryption wpa2", "encryption wpa3", "pmf"} {
			if !strings.Contains(text, root+" "+setting+"\n") {
				t.Fatalf("missing %s on %s", setting, ap)
			}
		}
		if strings.Contains(text, root+" no encryption wpa2\n") || strings.Contains(text, root+" pmf force\n") {
			t.Fatal("transition mode degraded to WPA3-only")
		}
		common := root + "\n    ssid UKN-Test-Transition\n    authentication wpa-psk synthetic-transition-key\n    encryption enable\n    encryption wpa2\n    wmm\n    up\n"
		before += common + "!\n"
		after += common + "    encryption wpa3\n    pmf\n!\n"
	}
	changes, err := Preimage(plan, before)
	if err != nil || len(changes) == 0 {
		t.Fatalf("rollback preparation failed: %v", err)
	}
	if VerifyWLANSettings(plan, before) == nil {
		t.Fatal("unapplied transition accepted")
	}
	if err := VerifyWLANSettings(plan, after); err != nil {
		t.Fatal(err)
	}
	if err := VerifyWLANSettings(plan, strings.ReplaceAll(after, "    pmf\n", "    pmf force\n")); err == nil {
		t.Fatal("required PMF accepted for optional request")
	}
	if err := VerifyWLANSettings(plan, strings.ReplaceAll(after, "    encryption wpa2\n", "")); err == nil {
		t.Fatal("WPA2 removal accepted")
	}
}
