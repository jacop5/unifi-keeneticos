// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
	"time"
)

// Network 10.5.67 generated these repeated, unindexed allowed windows for
// an 11:00–12:00 blackout on U7PG2 reporting 6.8.2.15592. Keep the wire
// representation rather than a map fixture that would hide duplicate loss.
func TestControllerLegacyBlackoutRoundtripEncoding(t *testing.T) {
	for _, build := range []func(string, map[string]string) (Plan, error){BuildPlan, BuildWLANTestPlan} {
		var cfg strings.Builder
		cfg.WriteString(strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Schedule"))
		cfg.WriteString("wireless.1.schedule_enabled=enabled\n")
		for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
			cfg.WriteString("wireless.1.schedule_" + day + "=0000-1100\nwireless.1.schedule_" + day + "=1200-2359\n")
		}
		p, err := build(cfg.String(), map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if err != nil || len(p.Schedules) != 1 || len(p.Unsupported) != 0 {
			t.Fatalf("controller legacy windows rejected: %v", err)
		}
		for _, day := range []int{22, 23, 24, 25, 26, 27, 28} {
			for _, at := range []struct {
				h, m int
				on   bool
			}{{0, 0, true}, {10, 59, true}, {11, 0, false}, {11, 59, false}, {12, 0, true}, {23, 59, true}} {
				if p.Schedules[0].ActiveAt(time.Date(2026, 11, day, at.h, at.m, 0, 0, time.UTC)) != at.on {
					t.Fatalf("blackout encoding lost/inverted a window at day=%d %02d:%02d", day, at.h, at.m)
				}
			}
		}
	}
}

func TestControllerFullBlackoutAndInvertedBlackout(t *testing.T) {
	base := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Schedule") + "wireless.1.schedule_enabled=enabled\n"
	for _, suffix := range []string{"", "wireless.1.schedule_invert=enabled\nwireless.1.schedule_sun.0=0000-2359\nwireless.1.schedule_mon.0=0000-2359\nwireless.1.schedule_tue.0=0000-2359\nwireless.1.schedule_wed.0=0000-2359\nwireless.1.schedule_thu.0=0000-2359\nwireless.1.schedule_fri.0=0000-2359\nwireless.1.schedule_sat.0=0000-2359\n"} {
		p, err := BuildWLANTestPlan(base+suffix, map[string]string{"1": "WifiMaster0/AccessPoint6"})
		if err != nil || len(p.Schedules) != 1 || len(p.Schedules[0].Actions) != 1 || p.Schedules[0].Actions[0].Start {
			t.Fatalf("full blackout became always-on: %v", err)
		}
	}
	p, err := BuildWLANTestPlan(base+"wireless.1.schedule_invert=enabled\nwireless.1.schedule_sun.0=1100-1200\n", map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil || len(p.Schedules) != 1 {
		t.Fatalf("indexed inverted blackout rejected: %v", err)
	}
	for _, at := range []struct {
		h  int
		on bool
	}{{10, true}, {11, false}, {12, true}} {
		if p.Schedules[0].ActiveAt(time.Date(2026, 11, 22, at.h, 0, 0, 0, time.UTC)) != at.on {
			t.Fatal("indexed invert flag did not produce blackout semantics")
		}
	}
}

func TestScheduleNormalizationRejectsAmbiguousOrExcessiveWindows(t *testing.T) {
	for _, cfg := range []string{
		"wireless.1.schedule_mon=0800-0900\nwireless.1.schedule_mon.0=1000-1100\n",
		"wireless.1.schedule_mon.0=1000-1100\nwireless.1.schedule_mon=0800-0900\n",
		"wireless.1.schedule_mon.0=1000-1100\nwireless.1.schedule_mon.0=1200-1300\n",
		strings.Repeat("wireless.1.schedule_mon=0800-0900\n", 33),
	} {
		if _, err := normalizeScheduleWindows(cfg); err == nil {
			t.Fatal("ambiguous or excessive windows accepted")
		}
	}
}
