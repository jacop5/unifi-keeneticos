// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strings"
)

// PrepareStoppedScheduleBinding is a development probe, not normal WLAN
// provisioning. It binds only an always-off schedule to an unused disabled
// non-primary AP. Resource inverses follow detachment during both immediate
// rollback and recovery from a journal containing unapplied forward steps.
func PrepareStoppedScheduleBinding(s BroadcastSchedule, running string) ([]Change, error) {
	if err := validateSchedule(s); err != nil {
		return nil, err
	}
	if strings.HasSuffix(s.Target, "AccessPoint0") || strings.HasSuffix(s.Target, "AccessPoint7") {
		return nil, fmt.Errorf("schedule binding probe requires a non-primary test slot")
	}
	for _, action := range s.Actions {
		if action.Start {
			return nil, fmt.Errorf("schedule binding probe must remain off at all times")
		}
	}
	lines, exists := interfaceBlocks(running)[s.Target]
	if !exists {
		return nil, fmt.Errorf("schedule binding probe interface is absent")
	}
	down := false
	for _, line := range lines {
		if line == "down" {
			down = true
		}
		if line == "up" || line == "ssid" || strings.HasPrefix(line, "ssid ") || line == "schedule" || strings.HasPrefix(line, "schedule ") {
			return nil, fmt.Errorf("schedule binding probe interface is in use")
		}
	}
	if !down {
		return nil, fmt.Errorf("schedule binding probe requires explicit disabled state")
	}
	if _, exists := scheduleBlocks(running)[s.Name]; exists {
		return nil, fmt.Errorf("schedule binding probe requires a new resource")
	}
	changes, err := PrepareUnboundSchedule(s, running)
	if err != nil {
		return nil, err
	}
	changes = append(changes, Change{
		Forward: Command{Text: "interface " + s.Target + " schedule " + s.Name},
		Undo:    Command{Text: "interface " + s.Target + " no schedule"},
	})
	return changes, nil
}

func VerifyScheduleBinding(s BroadcastSchedule, running string) error {
	if err := VerifyScheduleResource(s, running); err != nil {
		return err
	}
	count := 0
	for _, line := range interfaceBlocks(running)[s.Target] {
		if line == "schedule "+s.Name {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("native AP schedule binding differs")
	}
	return nil
}
