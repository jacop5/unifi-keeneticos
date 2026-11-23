// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFastRoamingDisableIsExplicitAndScoped(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-FT")
	for _, status := range []string{"", "disabled", "enabled", "invalid"} {
		p, err := BuildWLANTestPlan(base+"aaa.1.ft.status="+status+"\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if status == "invalid" {
			if err == nil {
				t.Fatal("invalid FT status accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range p.Commands {
			if strings.Contains(c.Text, " ft ") {
				if c.Text != "interface WifiMaster0/AccessPoint6 no ft enable" {
					t.Fatal("unexpected FT mutation")
				}
				found = true
			}
		}
		if found != (status == "disabled") {
			t.Fatalf("status=%q disable=%v", status, found)
		}
		if (len(p.Unsupported) > 0) != (status == "enabled") {
			t.Fatal("FT enable support was misreported")
		}
	}
}

func TestFastRoamingDisableReadbackAndRollback(t *testing.T) {
	const root = "interface WifiMaster0/AccessPoint6"
	before := root + "\n    ft enable\n    ft mdid AB\n    ft otd\n    down\n!\n"
	p := Plan{Commands: []Command{{Text: root + " no ft enable"}}}
	if VerifyWLANSettings(p, before) == nil {
		t.Fatal("ignored FT disable passed verification")
	}
	after := strings.ReplaceAll(before, "    ft enable\n", "")
	if err := VerifyWLANSettings(p, after); err != nil {
		t.Fatal(err)
	}
	changes, err := Preimage(p, before)
	if err != nil || len(changes) != 1 || changes[0].Undo.Text != root+" ft enable" {
		t.Fatalf("missing FT inverse: %v", err)
	}
	if changes, err := Preimage(p, after); err != nil || len(changes) != 0 {
		t.Fatal("disabled FT caused redundant mutation")
	}
	var calls []string
	journal := filepath.Join(t.TempDir(), "rollback.json")
	run := func(_ context.Context, command string) ([]byte, error) {
		if command == "show running-config" {
			return []byte(before), nil
		}
		calls = append(calls, command)
		return nil, nil // Simulate native command acknowledgement without application.
	}
	err = ApplyPlan(context.Background(), run, p, journal, func(context.Context) error { return VerifyWLANSettings(p, before) })
	if err == nil || len(calls) != 2 || calls[0] != root+" no ft enable" || calls[1] != root+" ft enable" {
		t.Fatalf("FT failure did not recover: %v %v", err, calls)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("recovered FT journal was not cleared")
	}
}

// These transaction primitives do not enable controller FT translation.
func TestFastRoamingDomainAndTransportTransaction(t *testing.T) {
	const root = "interface WifiMaster0/AccessPoint6"
	before := root + "\n    ft mdid AB\n    ft otd\n    down\n!\n"
	p := Plan{Commands: []Command{{Text: root + " ft mdid CD"}, {Text: root + " no ft otd"}}}
	if VerifyWLANSettings(p, before) == nil {
		t.Fatal("stale domain and transport accepted")
	}
	wrongTransport := strings.ReplaceAll(before, "ft mdid AB", "ft mdid CD")
	if VerifyWLANSettings(p, wrongTransport) == nil {
		t.Fatal("stale over-the-DS mode accepted")
	}
	after := strings.ReplaceAll(wrongTransport, "    ft otd\n", "")
	if err := VerifyWLANSettings(p, after); err != nil {
		t.Fatal(err)
	}
	changes, err := Preimage(p, before)
	if err != nil || len(changes) != 2 {
		t.Fatalf("changes=%v error=%v", changes, err)
	}
	if changes[0].Undo.Text != root+" ft mdid AB" || changes[1].Undo.Text != root+" ft otd" {
		t.Fatal("FT preimage not preserved")
	}
	if changes, err := Preimage(p, after); err != nil || len(changes) != 0 {
		t.Fatalf("not idempotent: %v %v", changes, err)
	}
	reset := Plan{Commands: []Command{{Text: root + " no ft mdid"}, {Text: root + " ft otd"}}}
	if VerifyWLANSettings(reset, after) == nil {
		t.Fatal("domain reset ignored")
	}
	resetAfter := root + "\n    ft otd\n    down\n!\n"
	if err := VerifyWLANSettings(reset, resetAfter); err != nil {
		t.Fatal(err)
	}
	changes, err = Preimage(reset, after)
	if err != nil || len(changes) != 2 || changes[0].Undo.Text != root+" ft mdid CD" || changes[1].Undo.Text != root+" no ft otd" {
		t.Fatalf("reset inverse: %v %v", changes, err)
	}
}
