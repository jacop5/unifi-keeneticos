// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"strings"
	"testing"
)

func TestPersonalSecurityProbeIsConfinedAndRedacted(t *testing.T) {
	const key = "private-probe-passphrase"
	for _, id := range []string{"WifiMaster0/AccessPoint5", "WifiMaster1/AccessPoint5"} {
		for _, mode := range []string{"wpa3", "transition"} {
			p, err := securityProbePlan(id, mode, key)
			if err != nil || len(p.Unsupported) != 0 {
				t.Fatalf("plan failed: %v %v", err, p.Unsupported)
			}
			credentials := 0
			var text strings.Builder
			for _, c := range p.Commands {
				if !strings.HasPrefix(c.Text, "interface "+id+" ") || strings.HasSuffix(c.Text, " up") {
					t.Fatalf("probe escaped disabled target: %s", c.String())
				}
				if strings.Contains(c.Text, key) {
					credentials++
					if !c.Secret || strings.Contains(c.String(), key) {
						t.Fatal("probe credential is not redacted")
					}
				}
				text.WriteString(c.Text + "\n")
			}
			if credentials != 1 || !strings.Contains(text.String(), " encryption wpa3\n") {
				t.Fatal("security probe omitted required settings")
			}
		}
	}
	if _, err := securityProbePlan("WifiMaster0/AccessPoint0", "wpa3", key); err == nil {
		t.Fatal("primary AP allowed as a probe target")
	}
}

func TestPersonalSecurityProbeVerification(t *testing.T) {
	base := "interface WifiMaster0/AccessPoint5\n    authentication wpa-psk private\n    encryption enable\n    encryption wpa3\n"
	for _, tc := range []struct{ mode, suffix string }{{"wpa3", "    pmf force\n"}, {"transition", "    encryption wpa2\n    pmf\n"}} {
		block := base + tc.suffix
		if err := verifySecurityProbe(tc.mode, block, "down"); err != nil {
			t.Fatal(err)
		}
		if verifySecurityProbe(tc.mode, block, "up") == nil || verifySecurityProbe(tc.mode, strings.ReplaceAll(block, "    encryption wpa3\n", ""), "down") == nil {
			t.Fatal("probe accepted an enabled AP or missing WPA3")
		}
	}
	if verifySecurityProbe("wpa3", base+"    encryption wpa2\n    pmf force\n", "down") == nil {
		t.Fatal("WPA3-only probe accepted WPA2")
	}
}
