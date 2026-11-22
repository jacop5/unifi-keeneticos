// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouterCivilTime(t *testing.T) {
	good := "weekday: 0\nday: 22\nmonth: 11\nyear: 2026\nhour: 10\nmin: 2\nsec: 48\n"
	civil, err := RouterCivilTime(good + "tz:\n    stdoffset: 3600\n")
	if err != nil || civil.Format("2006-01-02T15:04:05") != "2026-11-22T10:02:48" {
		t.Fatalf("native civil clock was shifted or rejected: %v %v", civil, err)
	}
	for _, invalid := range []string{
		"", strings.Replace(good, "year: 2026", "year: 1970", 1),
		strings.Replace(good, "weekday: 0", "weekday: 1", 1),
		strings.Replace(good, "day: 22", "day: 31", 1),
		strings.Replace(good, "hour: 10", "hour: 24", 1),
		strings.Replace(good, "sec: 48", "sec: invalid", 1),
	} {
		if _, err := RouterCivilTime(invalid); err == nil {
			t.Fatalf("invalid clock accepted: %q", invalid)
		}
	}
}

func TestScheduleTransitionsRecoverEveryForwardBoundary(t *testing.T) {
	s := *draftSchedule(t, "wireless.1.schedule_mon.0=0800-1700\n", true)
	for _, scenario := range []struct {
		name     string
		existing bool
		desired  *BroadcastSchedule
		enabled  bool
		binding  bool
		state    string
	}{
		{"create", false, &s, true, true, "up"},
		{"replace", true, &s, true, true, "up"},
		{"pause", true, &s, false, false, "down"},
		{"remove-schedule", true, nil, true, false, "up"},
		{"remove-wlan", true, nil, false, false, "down"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := func() (*scheduleModel, Runner) {
				m := scheduleFixture(s, scenario.existing)
				state, binding := "up", ""
				if scenario.existing {
					binding = s.Name
				}
				refresh := func() {
					m.refs = "interface " + s.Target + "\n    ssid UKN-Test-Schedule\n    " + state + "\n"
					if binding != "" {
						m.refs += "    schedule " + binding + "\n"
					}
					m.refs += "!\n"
				}
				refresh()
				run := func(ctx context.Context, command string) ([]byte, error) {
					prefix := "interface " + s.Target + " "
					if !strings.HasPrefix(command, prefix) {
						return m.run(ctx, command)
					}
					setting := strings.TrimPrefix(command, prefix)
					switch {
					case setting == "up" || setting == "down":
						state = setting
					case setting == "no schedule":
						binding = ""
					case strings.HasPrefix(setting, "schedule "):
						binding = strings.TrimPrefix(setting, "schedule ")
						if m.objects[binding] == nil {
							return nil, fmt.Errorf("binding absent resource")
						}
					default:
						return nil, fmt.Errorf("unexpected AP mutation")
					}
					refresh()
					return nil, nil
				}
				return m, run
			}
			m, _ := fixture()
			changes, err := PrepareScheduleTransition(s.Target, scenario.desired, scenario.enabled, m.snapshot())
			if err != nil || len(changes) == 0 {
				t.Fatalf("transition was not prepared: %v", err)
			}
			for cut := 0; cut <= len(changes); cut++ {
				t.Run(fmt.Sprint(cut), func(t *testing.T) {
					m, run := fixture()
					before := m.snapshot()
					journal := filepath.Join(t.TempDir(), "undo.json")
					if err := SaveUndo(journal, changes); err != nil {
						t.Fatal(err)
					}
					for _, c := range changes[:cut] {
						if _, err := run(context.Background(), c.Forward.Text); err != nil {
							t.Fatal(err)
						}
					}
					if cut == len(changes) {
						block := interfaceBlocks(m.snapshot())[s.Target]
						text := strings.Join(block, "\n")
						if !strings.Contains(text, "\n"+scenario.state) || strings.Contains(text, "schedule "+s.Name) != scenario.binding {
							t.Fatal("transition did not produce expected binding/state")
						}
						if next, err := PrepareScheduleTransition(s.Target, scenario.desired, scenario.enabled, m.snapshot()); err != nil || len(next) != 0 {
							t.Fatalf("unchanged schedule is disruptive: %v", err)
						}
					}
					if err := RecoverUndo(context.Background(), journal, run); err != nil {
						t.Fatal(err)
					}
					if m.snapshot() != before {
						t.Fatal("schedule transition recovery changed preimage")
					}
				})
			}
		})
	}
}

func TestScheduleTransitionRefusesForeignBindings(t *testing.T) {
	s := *draftSchedule(t, "", true)
	m := scheduleFixture(s, true)
	m.refs = "interface " + s.Target + "\n    up\n    schedule Manual\n!\n"
	if _, err := PrepareScheduleTransition(s.Target, &s, true, m.snapshot()); err == nil {
		t.Fatal("replaced operator binding")
	}
	m.refs = strings.Replace(m.refs, "schedule Manual", "schedule "+s.Name, 1) + "system\n    led power schedule " + s.Name + "\n!\n"
	if _, err := PrepareScheduleTransition(s.Target, nil, false, m.snapshot()); err == nil {
		t.Fatal("modified shared schedule binding")
	}
}
