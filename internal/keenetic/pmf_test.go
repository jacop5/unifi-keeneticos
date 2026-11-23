// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestPMFExplicitStatusAndMandatorySecurity(t *testing.T) {
	for _, tc := range []struct {
		status, mode         string
		sae, transition, owe bool
		want                 string
	}{
		{"", "2", false, false, false, "no pmf"},
		{"disabled", "2", false, false, false, "no pmf"},
		{"enabled", "2", false, false, false, "pmf force"},
		{"enabled", "1", false, false, false, "pmf"},
		{"enabled", "", false, false, false, "pmf"},
		{"disabled", "0", true, false, false, "pmf force"},
		{"disabled", "0", false, false, true, "pmf force"},
		{"disabled", "0", true, true, false, "pmf"},
	} {
		got, err := nativePMF(tc.status, tc.mode, tc.sae, tc.transition, tc.owe)
		if err != nil || got != tc.want {
			t.Fatalf("%+v got=%q err=%v", tc, got, err)
		}
	}
}

func TestPlannerRejectsUnknownPMFWithoutDowngrade(t *testing.T) {
	slots := map[string]string{"1": "WifiMaster0/AccessPoint6"}
	for _, extra := range []string{"aaa.1.pmf.status=invalid\n", "aaa.1.pmf.status=enabled\naaa.1.pmf.mode=3\n"} {
		if _, err := BuildPlan(wlan+extra, slots); err == nil {
			t.Fatal("unknown PMF accepted")
		}
	}
	p, err := BuildPlan(wlan+"aaa.1.pmf.status=disabled\naaa.1.pmf.mode=2\n", slots)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, c := range p.Commands {
		text += c.Text + "\n"
	}
	if !strings.Contains(text, " no pmf\n") || strings.Contains(text, " pmf force\n") {
		t.Fatal("stale PMF mode overrode disable")
	}
}
