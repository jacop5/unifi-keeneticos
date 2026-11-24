// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRadioReadbackRequiresChannelWidthAndAdministrativeState(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster1 channel 153"}, {Text: "interface WifiMaster1 channel width 40-below/80"}, {Text: "interface WifiMaster1 up"}}}
	running := "interface WifiMaster1\n    channel 153\n    channel width 40-below/80\n    up\n!\n"
	if err := VerifyRadioSettings(p, running); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"", strings.ReplaceAll(running, "channel 153", "channel 149"),
		strings.ReplaceAll(running, "40-below/80", "40-above/80"),
		strings.ReplaceAll(running, "    channel width 40-below/80\n", ""),
		strings.ReplaceAll(running, "    up\n", "    down\n"),
		strings.ReplaceAll(running, "    up\n", "    up\n    down\n"),
	} {
		if VerifyRadioSettings(p, bad) == nil {
			t.Fatal("incorrect or ambiguous radio state accepted")
		}
	}
	// A country-only or AP-only plan makes no channel/width assertion.
	if err := VerifyRadioSettings(Plan{Commands: []Command{{Text: "interface WifiMaster0 country-code IT"}, {Text: "interface WifiMaster0/AccessPoint6 up"}}}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRadioAutoChannelDoesNotReadWidthOrRescanAsFixedChannel(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster1 no channel"}}}
	for _, lines := range []string{"", "    channel auto\n", "    channel width 40-above/80\n    channel auto-rescan interval 1\n"} {
		if err := VerifyRadioSettings(p, "interface WifiMaster1\n"+lines+"    up\n!\n"); err != nil {
			t.Fatal(err)
		}
	}
	if VerifyRadioSettings(p, "interface WifiMaster1\n    channel 36\n    up\n!\n") == nil {
		t.Fatal("fixed channel passed auto request")
	}
}

func TestIgnoredRadioChangeTriggersExactRollback(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster1 channel 153"}, {Text: "interface WifiMaster1 channel width 40-below/80"}}}
	before := "interface WifiMaster1\n    channel 149\n    channel width 40-above/80\n    up\n!\n"
	journal := filepath.Join(t.TempDir(), "rollback.json")
	var calls []string
	run := func(_ context.Context, command string) ([]byte, error) {
		if command == "show running-config" {
			return []byte(before), nil
		}
		calls = append(calls, command)
		return nil, nil // Acknowledge without applying.
	}
	err := ApplyPlan(context.Background(), run, p, journal, func(context.Context) error { return VerifyRadioSettings(p, before) })
	if err == nil || !strings.Contains(err.Error(), "post-change health check failed") {
		t.Fatalf("ignored change confirmed: %v", err)
	}
	want := []string{"interface WifiMaster1 channel 153", "interface WifiMaster1 channel width 40-below/80", "interface WifiMaster1 channel width 40-above/80", "interface WifiMaster1 channel 149"}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("incorrect recovery commands: %v", calls)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("recovered journal retained")
	}
}
