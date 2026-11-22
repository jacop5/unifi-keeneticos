// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScheduledSecurityRecoveryAtEveryBoundary(t *testing.T) {
	s := *draftSchedule(t, "wireless.1.schedule_mon.0=0800-1700\n", true)
	for _, mode := range []string{"create", "replace", "rekey"} {
		existing := mode != "create"
		fixture := func() (*scheduleModel, Runner) {
			m := scheduleFixture(s, existing)
			if mode == "rekey" {
				m.objects[s.Name].actions = map[string]bool{}
				for _, action := range s.Actions {
					m.objects[s.Name].actions[action.String()] = true
				}
			}
			state, binding, ssid, key := "up", "", `"Old"`, `"old-secret"`
			if existing {
				binding = s.Name
			}
			refresh := func() {
				m.refs = "interface " + s.Target + "\n    ssid " + ssid + "\n    authentication wpa-psk " + key + "\n    " + state + "\n"
				if binding != "" {
					m.refs += "    schedule " + binding + "\n"
				}
				m.refs += "!\ninterface WifiMaster1/AccessPoint0\n    ssid Serving\n    up\n!\n"
			}
			refresh()
			run := func(ctx context.Context, command string) ([]byte, error) {
				if command == "show clock date" {
					return []byte("year: 2026\nmonth: 11\nday: 23\nweekday: 1\nhour: 10\nmin: 0\nsec: 0\n"), nil
				}
				prefix := "interface " + s.Target + " "
				if !strings.HasPrefix(command, prefix) {
					if (strings.HasPrefix(command, "schedule "+s.Name+" ") || command == "no schedule "+s.Name) && scheduleReferenced(m.snapshot(), s.Name) {
						return nil, fmt.Errorf("resource changed while bound")
					}
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
						return nil, fmt.Errorf("bound missing schedule")
					}
				case strings.HasPrefix(setting, "ssid ") || strings.HasPrefix(setting, "authentication wpa-psk "):
					if state != "down" || binding != "" {
						return nil, fmt.Errorf("security changed before quiescing and unbinding")
					}
					if strings.HasPrefix(setting, "ssid ") {
						ssid = strings.TrimPrefix(setting, "ssid ")
					} else {
						key = strings.TrimPrefix(setting, "authentication wpa-psk ")
					}
				default:
					return nil, fmt.Errorf("unexpected settings command")
				}
				refresh()
				return nil, nil
			}
			return m, run
		}
		plan := Plan{Schedules: []BroadcastSchedule{s}, Commands: []Command{
			{Text: "interface " + s.Target + ` ssid "New"`},
			{Text: "interface " + s.Target + ` authentication wpa-psk "new-secret"`, Secret: true},
			{Text: "interface " + s.Target + " up"},
		}}
		m, _ := fixture()
		changes, err := Preimage(plan, m.snapshot())
		if err != nil {
			t.Fatal(err)
		}
		for cut := 0; cut <= len(changes); cut++ {
			t.Run(fmt.Sprintf("%s/cut=%d", mode, cut), func(t *testing.T) {
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
					if err := VerifyScheduleSettings(plan, m.snapshot()); err != nil {
						t.Fatal(err)
					}
					if next, err := Preimage(plan, m.snapshot()); err != nil || len(next) != 0 {
						t.Fatalf("unchanged combined plan is disruptive: %v", err)
					}
				}
				if err := RecoverUndo(context.Background(), journal, run); err != nil {
					t.Fatal(err)
				}
				if m.snapshot() != before {
					t.Fatal("combined security/schedule rollback changed preimage")
				}
			})
		}
	}
}

func TestScheduleVerificationUsesOperationalWindowsAndPause(t *testing.T) {
	s := *draftSchedule(t, "wireless.1.schedule_mon.0=0800-1700\n", true)
	p := Plan{Schedules: []BroadcastSchedule{s}, Commands: []Command{{Text: "interface " + s.Target + " up"}}}
	on := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	off := on.Add(10 * time.Hour)
	up := &Snapshot{VAPs: []map[string]string{{"id": s.Target, "ssid": "Managed"}}}
	down := &Snapshot{}
	if VerifyScheduledWLANs(p, up, on) != nil || VerifyScheduledWLANs(p, down, off) != nil {
		t.Fatal("valid scheduled operational states rejected")
	}
	if VerifyScheduledWLANs(p, down, on) == nil || VerifyScheduledWLANs(p, up, off) == nil || VerifyScheduledWLANs(p, up, time.Time{}) == nil {
		t.Fatal("invalid scheduled operational state accepted")
	}
	p.Commands[0].Text = "interface " + s.Target + " down"
	if VerifyScheduledWLANs(p, up, on) == nil || VerifyScheduledWLANs(p, down, on) != nil || VerifyScheduledWLANs(p, down, time.Time{}) != nil {
		t.Fatal("pause was overridden by a schedule window")
	}
	m := scheduleFixture(s, true)
	m.refs = "interface " + s.Target + "\n    down\n    schedule " + s.Name + "\n!\n"
	if VerifyScheduleSettings(p, m.snapshot()) == nil {
		t.Fatal("paused AP retained timer binding")
	}
	m.refs = removeAPSchedule(m.refs, s.Target)
	if err := VerifyScheduleSettings(p, m.snapshot()); err != nil {
		t.Fatal(err)
	}
}

func TestFullPlanDisablesDeletedMappedWLAN(t *testing.T) {
	p, err := BuildPlan("", map[string]string{"1": "WifiMaster0/AccessPoint5"})
	if err != nil || len(p.Commands) != 1 || p.Commands[0].Text != "interface WifiMaster0/AccessPoint5 down" {
		t.Fatalf("deleted mapped WLAN remains enabled: %v", err)
	}
}

func TestSettingsOnlyPlanCannotBypassScheduleLifecycle(t *testing.T) {
	running := "interface WifiMaster0/AccessPoint5\n    up\n    schedule Manual\n!\n"
	for _, p := range []Plan{
		{Commands: []Command{{Text: `interface WifiMaster0/AccessPoint5 ssid "Changed"`}}},
		{AccessLists: []AccessList{{Target: "WifiMaster0/AccessPoint5", Policy: "none"}}},
	} {
		if _, err := Preimage(p, running); err == nil {
			t.Fatal("settings-only plan bypassed bound schedule protection")
		}
	}
}
