// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RouterCivilTime reads NDM's local calendar fields. Its UTC location is only
// a container for civil time: no offset conversion or host timezone is used.
func RouterCivilTime(text string) (time.Time, error) {
	values := Values(text)
	fields := make([]int, 7)
	for i, key := range []string{"year", "month", "day", "hour", "min", "sec", "weekday"} {
		value, err := strconv.Atoi(values[key])
		if err != nil {
			return time.Time{}, fmt.Errorf("native clock lacks a valid %s", key)
		}
		fields[i] = value
	}
	y, m, d, h, min, sec, weekday := fields[0], fields[1], fields[2], fields[3], fields[4], fields[5], fields[6]
	if y < 2020 || y > 9999 || m < 1 || m > 12 || d < 1 || d > 31 || h < 0 || h > 23 || min < 0 || min > 59 || sec < 0 || sec > 59 || weekday < 0 || weekday > 6 {
		return time.Time{}, fmt.Errorf("native clock is invalid or unset")
	}
	civil := time.Date(y, time.Month(m), d, h, min, sec, 0, time.UTC)
	if civil.Day() != d || int(civil.Weekday()) != weekday {
		return time.Time{}, fmt.Errorf("native clock calendar fields disagree")
	}
	return civil, nil
}

// PrepareScheduleTransition prepares the schedule portion of a WLAN
// transaction. Callers must supply validated WLAN settings and separately
// verify operational state against router-local civil time. Administrative
// enablement is independent of scheduled operational
// state: an off-window AP must stay administratively up for a future start.
//
// A paused WLAN is unbound before it is disabled. Disabled/removed schedules
// leave their owned resource dormant, avoiding deletion of a potentially shared
// object and allowing reuse. Foreign bindings or references are never replaced.
func PrepareScheduleTransition(target string, desired *BroadcastSchedule, enabled bool, running string) ([]Change, error) {
	return prepareScheduleTransition(target, desired, enabled, running, false)
}

func prepareScheduleTransition(target string, desired *BroadcastSchedule, enabled bool, running string, force bool) ([]Change, error) {
	if !regexpAP.MatchString(target) {
		return nil, fmt.Errorf("invalid schedule transition target")
	}
	if desired != nil {
		if desired.Target != target {
			return nil, fmt.Errorf("schedule target differs from WLAN")
		}
		if err := validateSchedule(*desired); err != nil {
			return nil, err
		}
	}
	lines, exists := interfaceBlocks(running)[target]
	if !exists {
		return nil, fmt.Errorf("schedule transition interface is absent")
	}
	oldState, oldBinding := "", ""
	for _, line := range lines {
		switch {
		case line == "up" || line == "down":
			if oldState != "" {
				return nil, fmt.Errorf("ambiguous AP administrative state")
			}
			oldState = line
		case strings.HasPrefix(line, "schedule "):
			if oldBinding != "" {
				return nil, fmt.Errorf("ambiguous AP schedule binding")
			}
			oldBinding = strings.TrimPrefix(line, "schedule ")
		}
	}
	if oldState == "" {
		return nil, fmt.Errorf("AP administrative state is missing")
	}
	name := scheduleName(target)
	unbound := removeAPSchedule(running, target)
	if oldBinding != "" {
		if oldBinding != name {
			return nil, fmt.Errorf("AP schedule binding is not owned")
		}
		resource, exists := scheduleBlocks(running)[name]
		if !exists {
			return nil, fmt.Errorf("bound schedule is absent")
		}
		if _, err := ownedScheduleActions(BroadcastSchedule{Target: target}, resource); err != nil {
			return nil, err
		}
		if scheduleReferenced(unbound, name) {
			return nil, fmt.Errorf("owned schedule is referenced by another service")
		}
	}
	newBinding, newState := "", "down"
	var resourceChanges []Change
	if enabled {
		newState = "up"
		if desired != nil {
			newBinding = desired.Name
			var err error
			resourceChanges, err = PrepareUnboundSchedule(*desired, unbound)
			if err != nil {
				return nil, err
			}
		}
	}
	if !force && len(resourceChanges) == 0 && oldBinding == newBinding && oldState == newState {
		return nil, nil
	}
	var changes []Change
	add := func(forward, undo string) {
		changes = append(changes, Change{Forward: Command{Text: "interface " + target + " " + forward}, Undo: Command{Text: "interface " + target + " " + undo}})
	}
	if oldBinding != "" {
		add("no schedule", "schedule "+oldBinding)
	}
	// Keep barriers even when already down: the complete journal can be
	// recovered before its first forward step, or after partial rollback.
	add("down", oldState)
	changes = append(changes, resourceChanges...)
	if newBinding != "" {
		add("schedule "+newBinding, "no schedule")
	}
	add(newState, "down")
	return changes, nil
}

func removeAPSchedule(running, target string) string {
	var out []string
	inside := false
	for _, line := range strings.Split(running, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "interface ") {
			inside = line == "interface "+target
		}
		if inside && strings.HasPrefix(strings.TrimSpace(line), "schedule ") {
			continue
		}
		out = append(out, line)
		if line == "!" {
			inside = false
		}
	}
	return strings.Join(out, "\n")
}
