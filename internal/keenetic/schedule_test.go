// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

func draftSchedule(t *testing.T, windows string, onDuring bool) *BroadcastSchedule {
	t.Helper()
	cfg := "wireless.1.schedule_enabled=enabled\n" + windows
	if onDuring {
		cfg += "wireless.1.schedule_invert=enabled\n"
	}
	s, err := compileSchedule(translate.ParseCfg(cfg), "wireless.1.", "WifiMaster0/AccessPoint5")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestScheduleWeekdaysAndWindowEdges(t *testing.T) {
	var windows string
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri"} {
		windows += "wireless.1.schedule_" + day + ".0=0800-1700\n"
	}
	s := draftSchedule(t, windows, true)
	if len(s.Actions) != 2 || s.Actions[0].String() != "action start 0 8 1,2,3,4,5" || s.Actions[1].String() != "action stop 0 17 1,2,3,4,5" {
		t.Fatal(s.Actions)
	}
	for _, tc := range []struct {
		date string
		on   bool
	}{{"2026-11-23T07:59", false}, {"2026-11-23T08:00", true}, {"2026-11-23T16:59", true}, {"2026-11-23T17:00", false}, {"2026-11-22T12:00", false}} {
		now, _ := time.Parse("2006-01-02T15:04", tc.date)
		if s.ActiveAt(now) != tc.on {
			t.Fatalf("incorrect schedule state at %s", tc.date)
		}
	}
}

func TestScheduleOverlapsOvernightAndWeekWrap(t *testing.T) {
	s := draftSchedule(t, "wireless.1.schedule_sat.0=2200-0200\nwireless.1.schedule_sat.1=2300-0100\n", true)
	if len(s.Actions) != 2 {
		t.Fatal("overlapping windows created extra state changes")
	}
	for hour, want := range map[int]bool{0: true, 1: true, 2: false, 12: false} {
		if s.ActiveAt(time.Date(2026, 11, 22, hour, 0, 0, 0, time.UTC)) != want {
			t.Fatalf("Saturday overnight did not carry into Sunday at %d", hour)
		}
	}
	off := draftSchedule(t, "wireless.1.schedule_sat.0=2200-0200\n", false)
	if off.ActiveAt(time.Date(2026, 11, 22, 1, 0, 0, 0, time.UTC)) || !off.ActiveAt(time.Date(2026, 11, 22, 2, 0, 0, 0, time.UTC)) {
		t.Fatal("off-during schedule has wrong midnight/end behavior")
	}
}

func TestScheduleAllDayAndEmptyWindows(t *testing.T) {
	s := draftSchedule(t, "wireless.1.schedule_mon.0=0000-2400\n", true)
	if !s.ActiveAt(time.Date(2026, 11, 23, 23, 59, 0, 0, time.UTC)) || s.ActiveAt(time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("24:00 end was not treated as next midnight")
	}
	for _, onDuring := range []bool{false, true} {
		empty := draftSchedule(t, "", onDuring)
		if len(empty.Actions) != 1 || empty.ActiveAt(time.Now()) == onDuring {
			t.Fatal("empty schedule does not have a constant weekly state")
		}
	}
}

func TestScheduleRejectsMalformedWindowsAndCannotApplyDraft(t *testing.T) {
	for _, window := range []string{"2900-3000", "2400-0100", "0800-2460", "0800-0900; reboot", "0800-"} {
		_, err := compileSchedule(translate.ParseCfg("wireless.1.schedule_enabled=enabled\nwireless.1.schedule_mon.0="+window+"\n"), "wireless.1.", "WifiMaster0/AccessPoint5")
		if err == nil {
			t.Fatalf("invalid window accepted: %q", window)
		}
	}
	cfg := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Schedule") + "wireless.1.schedule_enabled=enabled\nwireless.1.schedule_mon.0=0800-0900\n"
	p, err := BuildWLANTestPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint5"})
	if err != nil || len(p.Schedules) != 1 || len(p.Unsupported) == 0 {
		t.Fatalf("draft schedule was not classified: %v %v", p, err)
	}
	if _, err := Preimage(Plan{Schedules: p.Schedules}, ""); err == nil {
		t.Fatal("draft schedule allowed to apply without a native binding backend")
	}
	for _, action := range p.Schedules[0].Actions {
		if strings.Contains(fmt.Sprint(action), ";") {
			t.Fatal("schedule command contains unvalidated input")
		}
	}
}
