// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

func TestDHCPHostnamesUseUnexpiredLeases(t *testing.T) {
	lease := "lease:\n mac: 02:AA:00:00:00:01\n ip: 192.0.2.4\n hostname: Example-Mac\n name: Registered description\n expires: 600\n"
	for _, expires := range []string{"600", "infinity"} {
		names := ParseDHCPHostnames(strings.ReplaceAll(lease, "600", expires))
		if len(names) != 1 || names["02:aa:00:00:00:01"] != "Example-Mac" {
			t.Fatal(names)
		}
	}
	for _, invalid := range []string{
		strings.ReplaceAll(lease, "600", "0"), strings.ReplaceAll(lease, "600", "-1"), strings.ReplaceAll(lease, "600", "unknown"),
		strings.ReplaceAll(lease, "02:AA:00:00:00:01", "invalid"), strings.ReplaceAll(lease, "02:AA:00:00:00:01", "01:00:5e:00:00:01"),
		strings.ReplaceAll(lease, "192.0.2.4", "fe80::1"), strings.ReplaceAll(lease, "Example-Mac", strings.Repeat("a", 254)),
		strings.ReplaceAll(lease, "Example-Mac", "bad\x00name"), strings.ReplaceAll(lease, " hostname: Example-Mac\n", ""),
	} {
		if len(ParseDHCPHostnames(invalid)) != 0 {
			t.Fatal("invalid/expired lease supplied a hostname")
		}
	}
	if len(ParseDHCPHostnames(lease+strings.ReplaceAll(lease, "Example-Mac", "Other"))) != 0 {
		t.Fatal("conflicting names accepted")
	}
	s := &Snapshot{Hostnames: ParseDHCPHostnames(lease), Stations: []map[string]string{{"mac": "02:AA:00:00:00:01", "authenticated": "yes", "ap": "WifiMaster0/AccessPoint0"}, {"mac": "02:aa:00:00:00:02", "authenticated": "yes", "ap": "WifiMaster0/AccessPoint0"}}}
	rows := s.StationFields()
	if rows[0]["hostname"] != "Example-Mac" {
		t.Fatal(rows)
	}
	if _, exists := rows[1]["hostname"]; exists {
		t.Fatal("unrelated lease supplied hostname")
	}
}

func TestActiveHostnameFallbackExcludesOfflineHosts(t *testing.T) {
	text := "host:\n mac: 02:00:00:00:00:01\n ip: 192.0.2.2\n hostname: Active-Client\n link: up\n"
	if got := ParseActiveHostnames(text); got["02:00:00:00:00:01"] != "Active-Client" {
		t.Fatal(got)
	}
	for _, state := range []string{"down", "unknown", ""} {
		if len(ParseActiveHostnames(strings.ReplaceAll(text, "link: up", "link: "+state))) != 0 {
			t.Fatal("inactive host supplied name")
		}
	}
	if len(ParseActiveHostnames(text+strings.ReplaceAll(text, "Active-Client", "Conflicting"))) != 0 {
		t.Fatal("conflicting hostnames accepted")
	}
}

func TestActiveHostIPValidation(t *testing.T) {
	host := "host:\n mac: 02:00:00:00:00:01\n ip: 192.0.2.2\n link: up\n"
	if got := ParseActiveHostIPs(host); got["02:00:00:00:00:01"] != "192.0.2.2" {
		t.Fatal(got)
	}
	for _, bad := range []string{"0.0.0.0", "255.255.255.255", "224.0.0.1", "127.0.0.1", "fe80::1", "invalid"} {
		if len(ParseActiveHostIPs(strings.ReplaceAll(host, "192.0.2.2", bad))) != 0 {
			t.Fatal("invalid client address accepted")
		}
	}
	if len(ParseActiveHostIPs(strings.ReplaceAll(host, "link: up", "link: down"))) != 0 {
		t.Fatal("offline address accepted")
	}
	if len(ParseActiveHostIPs(host+strings.ReplaceAll(host, "192.0.2.2", "192.0.2.3"))) != 0 {
		t.Fatal("ambiguous address accepted")
	}
}
