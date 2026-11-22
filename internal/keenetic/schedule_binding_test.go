// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoppedScheduleBindingRecoveryAtEveryBoundary(t *testing.T) {
	s := *draftSchedule(t, "", true)
	s.Actions = []ScheduleAction{{Days: []int{0, 1, 2, 3, 4, 5, 6}}}
	base := "interface " + s.Target + "\n    down\n!\n"
	initial := scheduleFixture(s, false)
	initial.refs = base
	changes, err := PrepareStoppedScheduleBinding(s, initial.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for cut := 0; cut <= len(changes); cut++ {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			m := scheduleFixture(s, false)
			m.refs = base
			before := m.snapshot()
			run := func(ctx context.Context, command string) ([]byte, error) {
				switch command {
				case "interface " + s.Target + " schedule " + s.Name:
					if m.objects[s.Name] == nil {
						return nil, fmt.Errorf("binding missing resource")
					}
					m.refs = strings.Replace(base, "    down", "    schedule "+s.Name+"\n    down", 1)
					return nil, nil
				case "interface " + s.Target + " no schedule":
					m.refs = base
					return nil, nil
				default:
					return m.run(ctx, command)
				}
			}
			journal := filepath.Join(t.TempDir(), "rollback.json")
			if err := SaveUndo(journal, changes); err != nil {
				t.Fatal(err)
			}
			for _, change := range changes[:cut] {
				if _, err := run(context.Background(), change.Forward.Text); err != nil {
					t.Fatal(err)
				}
			}
			if cut == len(changes) {
				if err := VerifyScheduleBinding(s, m.snapshot()); err != nil {
					t.Fatal(err)
				}
			}
			if err := RecoverUndo(context.Background(), journal, run); err != nil {
				t.Fatal(err)
			}
			if m.snapshot() != before {
				t.Fatal("binding recovery did not restore interface and resources")
			}
		})
	}
}

func TestStoppedScheduleBindingRejectsServingOrEnabledTargets(t *testing.T) {
	s := *draftSchedule(t, "", true)
	s.Actions = []ScheduleAction{{Days: []int{0}}}
	for _, body := range []string{"", "up\n", "down\n    ssid Existing\n", "down\n    schedule Manual\n"} {
		if _, err := PrepareStoppedScheduleBinding(s, "interface "+s.Target+"\n    "+body+"!\n"); err == nil {
			t.Fatalf("accepted unsafe slot %q", body)
		}
	}
	running := "interface " + s.Target + "\n    down\n!\n"
	s.Actions[0].Start = true
	if _, err := PrepareStoppedScheduleBinding(s, running); err == nil {
		t.Fatal("accepted a schedule capable of enabling the probe")
	}
	s.Actions[0].Start = false
	for _, target := range []string{"WifiMaster0/AccessPoint0", "WifiMaster1/AccessPoint7"} {
		s.Target, s.Name = target, scheduleName(target)
		if _, err := PrepareStoppedScheduleBinding(s, "interface "+target+"\n    down\n!\n"); err == nil {
			t.Fatal("accepted reserved slot")
		}
	}
}
