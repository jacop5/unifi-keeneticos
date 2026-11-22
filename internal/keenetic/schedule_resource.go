// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func scheduleName(target string) string { return "UKN-" + strings.ReplaceAll(target, "/", "-") }
func scheduleOwner(target string) string {
	return "unifi-keeneticos-managed-" + strings.ReplaceAll(target, "/", "-")
}

func scheduleBlocks(running string) map[string][]string {
	blocks := map[string][]string{}
	current := ""
	for _, line := range strings.Split(ansi.ReplaceAllString(running, ""), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "schedule ") {
			current = strings.TrimPrefix(line, "schedule ")
			blocks[current] = nil
			continue
		}
		if line == "!" || (line != "" && !strings.HasPrefix(line, " ")) {
			current = ""
		}
		if current != "" && strings.HasPrefix(line, " ") {
			blocks[current] = append(blocks[current], strings.TrimSpace(line))
		}
	}
	return blocks
}

func scheduleReferenced(running, name string) bool {
	for _, line := range strings.Split(ansi.ReplaceAllString(running, ""), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "schedule "+name {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-2] == "schedule" && strings.Trim(fields[len(fields)-1], `"`) == name {
			return true
		}
	}
	return false
}

func parseScheduleAction(line string) (ScheduleAction, error) {
	parts := strings.Fields(line)
	a := ScheduleAction{}
	if len(parts) != 5 || parts[0] != "action" || (parts[1] != "start" && parts[1] != "stop") {
		return a, fmt.Errorf("unknown native schedule action")
	}
	a.Start = parts[1] == "start"
	var err error
	a.Minute, err = strconv.Atoi(parts[2])
	if err != nil || a.Minute < 0 || a.Minute > 59 {
		return a, fmt.Errorf("invalid schedule minute")
	}
	a.Hour, err = strconv.Atoi(parts[3])
	if err != nil || a.Hour < 0 || a.Hour > 23 {
		return a, fmt.Errorf("invalid schedule hour")
	}
	if parts[4] == "*" {
		a.Days = []int{0, 1, 2, 3, 4, 5, 6}
		return a, nil
	}
	seen := map[int]bool{}
	for _, value := range strings.Split(parts[4], ",") {
		day, err := strconv.Atoi(value)
		if err != nil || day < 0 || day > 6 || seen[day] {
			return a, fmt.Errorf("invalid or duplicate schedule weekday")
		}
		seen[day] = true
		a.Days = append(a.Days, day)
	}
	sort.Ints(a.Days)
	return a, nil
}

func validateSchedule(s BroadcastSchedule) error {
	if !regexpAP.MatchString(s.Target) || s.Name != scheduleName(s.Target) || len(s.Actions) == 0 || len(s.Actions) > 448 {
		return fmt.Errorf("invalid owned schedule identity or size")
	}
	seen := map[int]bool{}
	for _, a := range s.Actions {
		if a.Hour < 0 || a.Hour > 23 || a.Minute < 0 || a.Minute > 59 || len(a.Days) == 0 || len(a.Days) > 7 {
			return fmt.Errorf("invalid schedule action")
		}
		lastDay := -1
		for _, day := range a.Days {
			at := day*1440 + a.Hour*60 + a.Minute
			if day < 0 || day > 6 || day <= lastDay || seen[at] {
				return fmt.Errorf("duplicate or invalid schedule event")
			}
			lastDay = day
			seen[at] = true
		}
	}
	return nil
}

func ownedScheduleActions(s BroadcastSchedule, lines []string) (map[string]bool, error) {
	owned := false
	actions := map[string]bool{}
	for _, line := range lines {
		if strings.HasPrefix(line, "description ") {
			value := strings.TrimPrefix(line, "description ")
			if decoded, err := strconv.Unquote(value); err == nil {
				value = decoded
			}
			if value != scheduleOwner(s.Target) || owned {
				return nil, fmt.Errorf("native schedule is not owned by this backend")
			}
			owned = true
			continue
		}
		a, err := parseScheduleAction(line)
		if err != nil {
			return nil, err
		}
		actions[a.String()] = true
	}
	if !owned {
		return nil, fmt.Errorf("native schedule ownership marker is missing")
	}
	return actions, nil
}

// PrepareUnboundSchedule manages only an owned, unreferenced native resource.
// AP binding is a separate phase of the coupled WLAN transaction.
func PrepareUnboundSchedule(s BroadcastSchedule, running string) ([]Change, error) {
	if err := validateSchedule(s); err != nil {
		return nil, err
	}
	if scheduleReferenced(running, s.Name) {
		return nil, fmt.Errorf("native schedule is already referenced; refusing resource-only update")
	}
	lines, exists := scheduleBlocks(running)[s.Name]
	have := map[string]bool{}
	var err error
	if exists {
		have, err = ownedScheduleActions(s, lines)
		if err != nil {
			return nil, err
		}
	}
	var changes []Change
	add := func(forward, undo string) {
		c := Change{Forward: Command{Text: forward}, Undo: Command{Text: undo}}
		if !exists {
			c.Undo.IfScheduleExists = s.Name
		}
		changes = append(changes, c)
	}
	root := "schedule " + s.Name
	if !exists {
		add(root, "no "+root)
		// Keep/reassert ownership until final deletion. Clearing the marker
		// before a failed delete would make a later recovery ambiguous.
		add(root+" description "+scheduleOwner(s.Target), root+" description "+scheduleOwner(s.Target))
	}
	want := map[string]bool{}
	for _, a := range s.Actions {
		want[a.String()] = true
	}
	var old []string
	for action := range have {
		old = append(old, action)
	}
	sort.Strings(old)
	for _, action := range old {
		if !want[action] {
			add(root+" no "+action, root+" "+action)
		}
	}
	for _, a := range s.Actions {
		if !have[a.String()] {
			add(root+" "+a.String(), root+" no "+a.String())
		}
	}
	return changes, nil
}

func VerifyScheduleResource(s BroadcastSchedule, running string) error {
	if err := validateSchedule(s); err != nil {
		return err
	}
	lines, exists := scheduleBlocks(running)[s.Name]
	if !exists {
		return fmt.Errorf("owned schedule is absent")
	}
	actions, err := ownedScheduleActions(s, lines)
	if err != nil {
		return err
	}
	if len(actions) != len(s.Actions) {
		return fmt.Errorf("native schedule action count differs")
	}
	for _, a := range s.Actions {
		if !actions[a.String()] {
			return fmt.Errorf("native schedule action differs")
		}
	}
	return nil
}

// runUndo guards only inverse operations on newly created schedules. A durable
// journal contains every inverse even when a crash precedes forward creation.
func runUndo(ctx context.Context, run Runner, c Command, cache map[string]bool) error {
	name := c.IfScheduleExists
	if name == "" {
		_, err := run(ctx, c.Text)
		return err
	}
	target := strings.ReplaceAll(strings.TrimPrefix(name, "UKN-"), "-AccessPoint", "/AccessPoint")
	if !regexpAP.MatchString(target) || name != scheduleName(target) {
		return fmt.Errorf("invalid schedule recovery guard")
	}
	root := "schedule " + name
	if c.Text != "no "+root && c.Text != root+" description "+scheduleOwner(target) {
		action := strings.TrimPrefix(c.Text, root+" no ")
		if action == c.Text {
			return fmt.Errorf("schedule recovery guard has an unrelated command")
		}
		if _, err := parseScheduleAction(action); err != nil {
			return err
		}
	}
	exists, cached := cache[name]
	if !cached {
		b, err := run(ctx, "show running-config")
		if err != nil {
			return err
		}
		lines, present := scheduleBlocks(string(b))[name]
		exists = present
		if exists && len(lines) > 0 {
			if _, err := ownedScheduleActions(BroadcastSchedule{Name: name, Target: target}, lines); err != nil {
				return fmt.Errorf("schedule recovery ownership changed")
			}
		}
		if exists && scheduleReferenced(string(b), name) {
			return fmt.Errorf("new schedule is still referenced; refusing resource recovery")
		}
		cache[name] = exists
	}
	if !exists {
		return nil
	}
	_, err := run(ctx, c.Text)
	return err
}
