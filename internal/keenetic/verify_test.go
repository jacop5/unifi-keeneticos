// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyDoesNotAcceptUnrelatedWLAN(t *testing.T) {
	p := Plan{Commands: []Command{{Text: `interface WifiMaster0/AccessPoint1 ssid "Managed"`}, {Text: "interface WifiMaster0/AccessPoint1 up"}}}
	s := &Snapshot{VAPs: []map[string]string{{"id": "WifiMaster0/AccessPoint0", "ssid": "Other"}}}
	if err := VerifyWLANs(p, s); err == nil {
		t.Fatal("unrelated WLAN accepted")
	}
	s.VAPs = append(s.VAPs, map[string]string{"id": "WifiMaster0/AccessPoint1", "ssid": "Wrong"})
	if err := VerifyWLANs(p, s); err == nil {
		t.Fatal("wrong SSID accepted")
	}
	s.VAPs[1]["ssid"] = "Managed"
	if err := VerifyWLANs(p, s); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyWLANRejectsIgnoredSecuritySettings(t *testing.T) {
	const target = "WifiMaster0/AccessPoint5"
	p := Plan{}
	for _, setting := range []string{"encryption enable", "encryption wpa3", "no encryption wpa2", "no encryption wpa", "no encryption owe", "pmf force", "no peer-isolation", "no hide-ssid", "no rrm", "authentication wpa-psk requested-secret"} {
		p.Commands = append(p.Commands, Command{Text: "interface " + target + " " + setting})
	}
	good := "interface " + target + "\n    encryption enable\n    encryption wpa3\n    authentication wpa-psk native-stored-key\n    pmf force\n    down\n!\n"
	if err := VerifyWLANSettings(p, good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"",
		strings.ReplaceAll(good, "    encryption enable\n", ""),
		strings.ReplaceAll(good, "    encryption wpa3\n", "    encryption wpa2\n"),
		strings.ReplaceAll(good, "    pmf force\n", "    pmf\n"),
		strings.ReplaceAll(good, "    authentication wpa-psk native-stored-key\n", ""),
		strings.ReplaceAll(good, "!\n", "    encryption wpa\n!\n"),
		strings.ReplaceAll(good, "!\n", "    encryption owe\n!\n"),
		strings.ReplaceAll(good, "!\n", "    hide-ssid\n!\n"),
		strings.ReplaceAll(good, "!\n", "    peer-isolation\n!\n"),
		strings.ReplaceAll(good, "!\n", "    rrm\n!\n"),
	} {
		if err := VerifyWLANSettings(p, bad); err == nil {
			t.Fatal("incorrect native security flags passed confirmation")
		}
	}
}

func TestVerifyOpenRequiresKeyRemovalAndEncryptionOff(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0/AccessPoint5 no authentication wpa-psk"}, {Text: "interface WifiMaster0/AccessPoint5 no encryption enable"}, {Text: "interface WifiMaster0/AccessPoint5 no pmf"}}}
	good := "interface WifiMaster0/AccessPoint5\n    down\n!\n"
	if err := VerifyWLANSettings(p, good); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{"authentication wpa-psk stale-secret", "encryption enable", "pmf force"} {
		if VerifyWLANSettings(p, strings.ReplaceAll(good, "!", "    "+stale+"\n!")) == nil {
			t.Fatal("stale native security setting accepted")
		}
	}
}

func TestIgnoredSecurityCommandTriggersJournaledRollback(t *testing.T) {
	running := "interface WifiMaster0/AccessPoint5\n    encryption enable\n    encryption wpa2\n    down\n!\n"
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0/AccessPoint5 encryption wpa3"}, {Text: "interface WifiMaster0/AccessPoint5 no encryption wpa2"}}}
	var commands []string
	run := func(_ context.Context, command string) ([]byte, error) {
		if command == "show running-config" {
			return []byte(running), nil
		}
		// Simulate a driver acknowledging commands without changing its state.
		commands = append(commands, command)
		return nil, nil
	}
	journal := filepath.Join(t.TempDir(), "rollback.json")
	err := ApplyPlan(context.Background(), run, p, journal, func(context.Context) error {
		return VerifyWLANSettings(p, running)
	})
	if err == nil || len(commands) != 4 || commands[2] != "interface WifiMaster0/AccessPoint5 encryption wpa2" || commands[3] != "interface WifiMaster0/AccessPoint5 no encryption wpa3" {
		t.Fatalf("ignored command did not roll back: err=%v commands=%v", err, commands)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatalf("completed recovery left a journal: %v", err)
	}
}
