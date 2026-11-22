// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type scheduleModelObject struct {
	description string
	actions     map[string]bool
}

type scheduleModel struct {
	objects map[string]*scheduleModelObject
	refs    string
}

func (m *scheduleModel) snapshot() string {
	var names []string
	for name := range m.objects {
		names = append(names, name)
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		object := m.objects[name]
		fmt.Fprintf(&out, "schedule %s\n", name)
		if object.description != "" {
			fmt.Fprintf(&out, "    description %s\n", object.description)
		}
		var actions []string
		for action := range object.actions {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		for _, action := range actions {
			fmt.Fprintf(&out, "    %s\n", action)
		}
		out.WriteString("!\n")
	}
	out.WriteString(m.refs)
	return out.String()
}

func (m *scheduleModel) run(_ context.Context, command string) ([]byte, error) {
	if command == "show running-config" {
		return []byte(m.snapshot()), nil
	}
	if strings.HasPrefix(command, "no schedule ") {
		name := strings.TrimPrefix(command, "no schedule ")
		if m.objects[name] == nil {
			return nil, fmt.Errorf("native schedule missing")
		}
		delete(m.objects, name)
		return nil, nil
	}
	parts := strings.SplitN(command, " ", 3)
	if len(parts) < 2 || parts[0] != "schedule" {
		return nil, fmt.Errorf("unexpected model command")
	}
	name := parts[1]
	if len(parts) == 2 {
		if m.objects[name] == nil {
			m.objects[name] = &scheduleModelObject{actions: map[string]bool{}}
		}
		return nil, nil
	}
	o := m.objects[name]
	if o == nil {
		return nil, fmt.Errorf("native schedule missing")
	}
	switch setting := parts[2]; {
	case strings.HasPrefix(setting, "description "):
		o.description = strings.TrimPrefix(setting, "description ")
	case setting == "no description":
		o.description = ""
	case strings.HasPrefix(setting, "no action "):
		delete(o.actions, strings.TrimPrefix(setting, "no "))
	case strings.HasPrefix(setting, "action "):
		o.actions[setting] = true
	default:
		return nil, fmt.Errorf("unsupported model schedule operation")
	}
	return nil, nil
}

func scheduleFixture(s BroadcastSchedule, existing bool) *scheduleModel {
	m := &scheduleModel{objects: map[string]*scheduleModelObject{
		"Manual": {description: "operator", actions: map[string]bool{"action start 0 0 *": true}},
	}}
	if existing {
		m.objects[s.Name] = &scheduleModelObject{description: scheduleOwner(s.Target), actions: map[string]bool{"action stop 0 0 0": true}}
	}
	return m
}

func TestScheduleResourceCrashRecoveryAtEveryBoundary(t *testing.T) {
	s := *draftSchedule(t, "wireless.1.schedule_mon.0=0800-1700\n", true)
	for _, existing := range []bool{false, true} {
		initial := scheduleFixture(s, existing)
		changes, err := PrepareUnboundSchedule(s, initial.snapshot())
		if err != nil || len(changes) == 0 {
			t.Fatalf("prepare failed: %v", err)
		}
		for cut := 0; cut <= len(changes); cut++ {
			t.Run(fmt.Sprintf("existing=%t/cut=%d", existing, cut), func(t *testing.T) {
				model := scheduleFixture(s, existing)
				before := model.snapshot()
				journal := filepath.Join(t.TempDir(), "rollback.json")
				if err := SaveUndo(journal, changes); err != nil {
					t.Fatal(err)
				}
				for _, change := range changes[:cut] {
					if _, err := model.run(context.Background(), change.Forward.Text); err != nil {
						t.Fatal(err)
					}
				}
				if cut == len(changes) {
					if err := VerifyScheduleResource(s, model.snapshot()); err != nil {
						t.Fatal(err)
					}
				}
				if err := RecoverUndo(context.Background(), journal, model.run); err != nil {
					t.Fatal(err)
				}
				if model.snapshot() != before {
					t.Fatal("resource recovery changed the preimage or unrelated schedule")
				}
				if err := RecoverUndo(context.Background(), journal, model.run); err != nil {
					t.Fatal("completed resource recovery was not idempotent")
				}
			})
		}
	}
}

func TestScheduleResourceTransactionAndOwnership(t *testing.T) {
	s := *draftSchedule(t, "wireless.1.schedule_mon.0=0800-1700\n", true)
	m := scheduleFixture(s, false)
	before := m.snapshot()
	changes, err := PrepareUnboundSchedule(s, before)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := Begin(context.Background(), m.run, changes, time.Second)
	if err != nil || VerifyScheduleResource(s, m.snapshot()) != nil {
		t.Fatalf("resource transaction failed: %v", err)
	}
	if delta, err := PrepareUnboundSchedule(s, m.snapshot()); err != nil || len(delta) != 0 {
		t.Fatalf("unchanged resource was rewritten: %v %v", delta, err)
	}
	if err := tx.Rollback(); err != nil || m.snapshot() != before {
		t.Fatalf("immediate resource rollback failed: %v", err)
	}
	m = scheduleFixture(s, true)
	m.objects[s.Name].description = "operator-owned"
	if _, err := PrepareUnboundSchedule(s, m.snapshot()); err == nil {
		t.Fatal("colliding operator schedule was taken over")
	}
	guardedDelete := Command{Text: "no schedule " + s.Name, IfScheduleExists: s.Name}
	if err := runUndo(context.Background(), m.run, guardedDelete, map[string]bool{}); err == nil || m.objects[s.Name] == nil {
		t.Fatal("recovery deleted a resource whose ownership had changed")
	}
	m.objects[s.Name].description = scheduleOwner(s.Target)
	for _, ref := range []string{
		"interface WifiMaster0/AccessPoint5\n    schedule " + s.Name + "\n!\n",
		"system\n    led power schedule " + s.Name + "\n!\n",
	} {
		m.refs = ref
		if _, err := PrepareUnboundSchedule(s, m.snapshot()); err == nil {
			t.Fatal("referenced schedule allowed a resource-only update")
		}
		c := Command{Text: "no schedule " + s.Name, IfScheduleExists: s.Name}
		if err := runUndo(context.Background(), m.run, c, map[string]bool{}); err == nil {
			t.Fatal("resource recovery deleted a referenced schedule")
		}
	}
}

func TestScheduleRecoveryGuardRejectsUnrelatedCommands(t *testing.T) {
	s := *draftSchedule(t, "", true)
	for _, c := range []Command{
		{Text: "system reboot", IfScheduleExists: s.Name},
		{Text: "no schedule Manual", IfScheduleExists: "Manual"},
		{Text: "schedule " + s.Name + " no action start 0 99 0", IfScheduleExists: s.Name},
	} {
		err := runUndo(context.Background(), func(context.Context, string) ([]byte, error) {
			t.Fatal("invalid recovery guard reached the router")
			return nil, nil
		}, c, map[string]bool{})
		if err == nil {
			t.Fatal("invalid recovery guard accepted")
		}
	}
}
