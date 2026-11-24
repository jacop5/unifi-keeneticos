// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestPreimageStringQuotingDoesNotCauseReprovision(t *testing.T) {
	const ap = "WifiMaster0/AccessPoint6"
	for _, family := range []string{"ssid", "authentication wpa-psk"} {
		for _, value := range []string{"UKN-Test-Example", "Test Network", "Réseau 日本語", "a'b$();!literal"} {
			if family == "authentication wpa-psk" && strings.Contains(value, " ") {
				continue // Multi-argument native credential syntax is ambiguous.
			}
			for _, reverse := range []bool{false, true} {
				want, previous := `"`+value+`"`, value
				if reverse {
					want, previous = previous, want
				}
				p := Plan{Commands: []Command{{Text: "interface " + ap + " " + family + " " + want, Secret: family == "authentication wpa-psk"}}}
				changes, err := Preimage(p, "interface "+ap+"\n    "+family+" "+previous+"\n    up\n!\n")
				if err != nil || len(changes) != 0 {
					t.Fatalf("family=%s reverse=%v rewrote equivalent value: changes=%d err=%v", family, reverse, len(changes), err)
				}
			}
		}
	}
}

func TestPreimageStringComparisonRetainsRealChanges(t *testing.T) {
	const ap = "WifiMaster0/AccessPoint6"
	for _, row := range []struct{ want, previous string }{
		{`ssid "New"`, `ssid Old`},
		{`ssid "Case"`, `ssid case`},
		{`ssid "two  spaces"`, `ssid two spaces`},
		{`ssid "Name"`, `no ssid`},
		{`no ssid`, `ssid "Name"`},
		{`ssid "broken`, `ssid broken`},
		{`ssid "a\tb"`, `ssid a\tb`},
		{`authentication wpa-psk "new-private-fixture"`, `authentication wpa-psk old-private-fixture`},
		{`authentication wpa-psk "ns3 synthetic-blob"`, `authentication wpa-psk ns3 synthetic-blob`},
		{`authentication wpa-psk "ns2 synthetic-blob"`, `authentication wpa-psk ns2 synthetic-blob`},
		{`authentication wpa-psk "ordinary space"`, `authentication wpa-psk ordinary space`},
		{`no authentication wpa-psk`, `authentication wpa-psk private-fixture`},
	} {
		p := Plan{Commands: []Command{{Text: "interface " + ap + " " + row.want, Secret: true}}}
		changes, err := Preimage(p, "interface "+ap+"\n    "+row.previous+"\n    up\n!\n")
		if err != nil || len(changes) != 1 {
			t.Fatalf("real/ambiguous change omitted: %v count=%d", err, len(changes))
		}
		if changes[0].Undo.Text != "interface "+ap+" "+row.previous || !changes[0].Undo.Secret {
			t.Fatal("inverse or credential redaction changed")
		}
	}
}
