// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strings"
	"testing"
)

func TestRadioWidthPrimaryChannelPairing(t *testing.T) {
	for _, pair := range [][2]int{{36, 40}, {44, 48}, {52, 56}, {60, 64}, {100, 104}, {108, 112}, {116, 120}, {124, 128}, {132, 136}, {140, 144}, {149, 153}, {157, 161}, {165, 169}, {173, 177}} {
		for _, width := range []string{"40", "80"} {
			for i, ch := range pair {
				side := "above"
				if i == 1 {
					side = "below"
				}
				want := "40-" + side
				if width == "80" {
					want += "/80"
				}
				cfg := fmt.Sprintf("radio.1.phyname=wifi1\nradio.1.channel=%d\nradio.1.ieee_mode=11acvht%s\n", ch, width)
				p, err := BuildPlan(cfg, nil)
				if err != nil || len(p.Unsupported) > 0 {
					t.Fatalf("channel %d width %s: %v %v", ch, width, err, p.Unsupported)
				}
				found := false
				for _, c := range p.Commands {
					if c.Text == "interface WifiMaster1 channel width "+want {
						found = true
					}
				}
				if !found {
					t.Fatalf("channel %d width %s missing %s", ch, width, want)
				}
			}
		}
	}
}

func TestRadioWidthRejectsUnknownModeAndWrongBand(t *testing.T) {
	for _, tc := range []struct{ phy, mode string }{
		{"wifi0", "garbageht20"}, {"wifi1", "garbageht80"}, {"wifi0", "11nght80"},
		{"wifi0", "11nght160"}, {"wifi0", "11acvht40"}, {"wifi1", "11nght40"},
		{"wifi1", "11acvht80-extra"},
	} {
		p, err := BuildPlan("radio.1.phyname="+tc.phy+"\nradio.1.ieee_mode="+tc.mode+"\n", nil)
		if err == nil && len(p.Unsupported) == 0 {
			t.Fatalf("accepted %s %s", tc.phy, tc.mode)
		}
		for _, c := range p.Commands {
			if strings.Contains(c.Text, " channel width ") {
				t.Fatal("unsupported mode emitted width change")
			}
		}
	}
	for _, tc := range []struct{ mode, master, ch, want string }{
		{"11nght40", "WifiMaster0", "6", "40-above"},
		{"11nght40", "WifiMaster0", "12", "40-below"},
		{"11naht40", "WifiMaster1", "auto", "40-above"},
		{"11acvht80", "WifiMaster1", "0", "40-above/80"},
		{"11acvht160", "WifiMaster1", "36", "160"},
	} {
		got, ok := nativeRadioWidth(tc.mode, tc.master, tc.ch)
		if !ok || got != tc.want {
			t.Fatalf("%+v -> %s %v", tc, got, ok)
		}
	}
}

func TestRadioWidthNewDirectionHasExactPreimage(t *testing.T) {
	p, err := BuildPlan("radio.1.phyname=wifi1\nradio.1.channel=153\nradio.1.ieee_mode=11acvht80\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Preimage(p, "interface WifiMaster1\n    channel 149\n    channel width 40-above/80\n    up\n!\n")
	if err != nil || len(changes) != 2 {
		t.Fatalf("%v %v", changes, err)
	}
	if changes[1].Forward.Text != "interface WifiMaster1 channel width 40-below/80" || changes[1].Undo.Text != "interface WifiMaster1 channel width 40-above/80" {
		t.Fatal("width inverse lost")
	}
	scoped, err := BuildWLANTestPlan("radio.1.phyname=wifi1\nradio.1.channel=153\nradio.1.ieee_mode=11acvht80\n", map[string]string{"3": "WifiMaster0/AccessPoint6"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range scoped.Commands {
		if strings.HasPrefix(c.Text, "interface WifiMaster1 ") {
			t.Fatal("test mode emitted radio command")
		}
	}
}
