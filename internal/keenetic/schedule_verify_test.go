// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduledCollectionRetriesCrossedTimerBoundary(t *testing.T) {
	s := *draftSchedule(t, "wireless.1.schedule_mon.0=0800-1700\n", true)
	p := Plan{Schedules: []BroadcastSchedule{s}, Commands: []Command{{Text: "interface " + s.Target + " up"}}}
	clocks := []string{"16:59:59", "17:00:00", "17:00:01", "17:00:02"}
	clockReads, samples := 0, 0
	run := func(_ context.Context, command string) ([]byte, error) {
		switch command {
		case "show clock date":
			if clockReads == len(clocks) {
				t.Fatal("unexpected additional sample")
			}
			parts := strings.Split(clocks[clockReads], ":")
			clockReads++
			return []byte("year: 2026\nmonth: 11\nday: 23\nweekday: 1\nhour: " + parts[0] + "\nmin: " + parts[1] + "\nsec: " + parts[2] + "\n"), nil
		case "show version", "show system", "show associations", "show ip arp":
			return nil, nil
		case "show interface Bridge0":
			return []byte("id: Bridge0\naddress: 192.0.2.1\nmac: 02:00:00:00:00:01\n"), nil
		case "show interface WifiMaster0", "show interface WifiMaster1":
			return []byte("id: " + strings.TrimPrefix(command, "show interface ") + "\n"), nil
		case "show interface":
			samples++
			if samples == 1 {
				return []byte("Interface, name = \"AP\"\n id: " + s.Target + "\n type: AccessPoint\n state: up\n ssid: Managed\n"), nil
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command")
		}
	}
	if _, err := CollectVerifiedWLANs(context.Background(), run, p); err != nil || samples != 2 || clockReads != 4 {
		t.Fatalf("cross-boundary sample was not discarded: %v samples=%d clocks=%d", err, samples, clockReads)
	}
}

func TestInvalidScheduleClockPreventsMutation(t *testing.T) {
	s := *draftSchedule(t, "", true)
	p := Plan{Schedules: []BroadcastSchedule{s}, Commands: []Command{{Text: "interface " + s.Target + " up"}}}
	journal := filepath.Join(t.TempDir(), "rollback.json")
	run := func(_ context.Context, command string) ([]byte, error) {
		switch command {
		case "show running-config":
			return []byte("interface " + s.Target + "\n    down\n!\n"), nil
		case "show clock date":
			return []byte("year: 1970\n"), nil
		default:
			t.Fatal("mutated router before validating native clock")
			return nil, nil
		}
	}
	if err := ApplyPlan(context.Background(), run, p, journal, func(context.Context) error { return nil }); err == nil {
		t.Fatal("invalid native clock accepted")
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("unused journal was created")
	}
}
