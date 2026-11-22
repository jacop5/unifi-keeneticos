// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func enabledSchedules(plan Plan) []BroadcastSchedule {
	enabled := map[string]bool{}
	for _, command := range plan.Commands {
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" && parts[2] == "up" {
			enabled[parts[1]] = true
		}
	}
	var schedules []BroadcastSchedule
	for _, schedule := range plan.Schedules {
		if enabled[schedule.Target] {
			schedules = append(schedules, schedule)
		}
	}
	return schedules
}

func VerifyScheduledWLANs(plan Plan, s *Snapshot, civil time.Time) error {
	operational := plan
	operational.Commands = append([]Command(nil), plan.Commands...)
	for _, schedule := range enabledSchedules(plan) {
		if err := validateSchedule(schedule); err != nil {
			return err
		}
		if civil.IsZero() {
			return fmt.Errorf("router-local time is required for schedule verification")
		}
		if !schedule.ActiveAt(civil) {
			for i, command := range operational.Commands {
				if command.Text == "interface "+schedule.Target+" up" {
					operational.Commands[i].Text = "interface " + schedule.Target + " down"
				}
			}
		}
	}
	return VerifyWLANs(operational, s)
}

// CollectVerifiedWLANs brackets collection with native clock reads. A timer
// boundary during collection causes a fresh sample rather than a comparison
// between status from one window and time from another.
func CollectVerifiedWLANs(ctx context.Context, run Runner, plan Plan) (*Snapshot, error) {
	schedules := enabledSchedules(plan)
	clock := func() (time.Time, error) {
		if len(schedules) == 0 {
			return time.Time{}, nil
		}
		b, err := run(ctx, "show clock date")
		if err != nil {
			return time.Time{}, err
		}
		return RouterCivilTime(string(b))
	}
	for attempt := 0; attempt < 3; attempt++ {
		before, err := clock()
		if err != nil {
			return nil, err
		}
		s, err := Collect(ctx, run)
		if err != nil {
			return nil, err
		}
		after, err := clock()
		if err != nil {
			return nil, err
		}
		stable := true
		for _, schedule := range schedules {
			stable = stable && schedule.ActiveAt(before) == schedule.ActiveAt(after)
		}
		if stable {
			return s, VerifyScheduledWLANs(plan, s, after)
		}
	}
	return nil, fmt.Errorf("schedule clock changed across repeated status samples")
}

func VerifyScheduleSettings(plan Plan, running string) error {
	desired := map[string]BroadcastSchedule{}
	for _, schedule := range plan.Schedules {
		if err := validateSchedule(schedule); err != nil {
			return err
		}
		desired[schedule.Target] = schedule
	}
	blocks := interfaceBlocks(running)
	for _, command := range plan.Commands {
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) != 3 || !regexpAP.MatchString(parts[1]) || (parts[2] != "up" && parts[2] != "down") {
			continue
		}
		schedule, hasSchedule := desired[parts[1]]
		if hasSchedule && parts[2] == "up" {
			if err := VerifyScheduleBinding(schedule, running); err != nil {
				return err
			}
		} else {
			for _, line := range blocks[parts[1]] {
				if strings.HasPrefix(line, "schedule ") {
					return fmt.Errorf("paused or unscheduled WLAN retains a native timer binding")
				}
			}
		}
		foundState := false
		for _, line := range blocks[parts[1]] {
			foundState = foundState || line == parts[2]
		}
		if !foundState {
			return fmt.Errorf("native AP administrative state differs from plan")
		}
	}
	return nil
}
