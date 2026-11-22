// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"sort"
	"strings"
)

// scheduledPreimage couples schedule lifecycle and WLAN settings. All affected
// APs are detached/quiesced before any resource or security change. Reverse
// execution disables/unbinds them before restoring settings and timer actions,
// and restores the original enablement/bindings last, including full-journal
// recovery when no forward commands have executed yet.
func scheduledPreimage(plan Plan, running string) ([]Change, error) {
	if len(plan.Unsupported) != 0 {
		return nil, fmt.Errorf("plan contains unsupported settings")
	}
	states := map[string]bool{}
	for _, command := range plan.Commands {
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" && regexpAP.MatchString(parts[1]) && (parts[2] == "up" || parts[2] == "down") {
			if _, duplicate := states[parts[1]]; duplicate {
				return nil, fmt.Errorf("duplicate WLAN administrative state")
			}
			states[parts[1]] = parts[2] == "up"
		}
	}
	desired := map[string]*BroadcastSchedule{}
	for i := range plan.Schedules {
		s := &plan.Schedules[i]
		if err := validateSchedule(*s); err != nil {
			return nil, err
		}
		if _, exists := states[s.Target]; !exists || desired[s.Target] != nil {
			return nil, fmt.Errorf("schedule requires one explicitly mapped WLAN state")
		}
		desired[s.Target] = s
	}
	blocks := interfaceBlocks(running)
	// A settings-only caller must not bypass schedule quiescing by omitting
	// the final WLAN state. Controller-built plans always supply that state.
	touched := map[string]bool{}
	for _, command := range plan.Commands {
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" && regexpAP.MatchString(parts[1]) {
			touched[parts[1]] = true
		}
	}
	for _, acl := range plan.AccessLists {
		touched[acl.Target] = true
	}
	for target := range touched {
		if _, explicit := states[target]; explicit {
			continue
		}
		for _, line := range blocks[target] {
			if strings.HasPrefix(line, "schedule ") {
				return nil, fmt.Errorf("settings on a scheduled AP require an explicit final WLAN state")
			}
		}
	}
	affected := map[string]bool{}
	for target := range states {
		if desired[target] != nil {
			affected[target] = true
		}
		for _, line := range blocks[target] {
			if strings.HasPrefix(line, "schedule ") {
				affected[target] = true
			}
		}
	}
	if len(affected) == 0 {
		return preimageSettings(plan, running)
	}
	var targets []string
	for target := range affected {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	settings := plan
	settings.Schedules = nil
	settings.Commands = nil
	for _, command := range plan.Commands {
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) == 3 && affected[parts[1]] && (parts[2] == "up" || parts[2] == "down") {
			continue
		}
		settings.Commands = append(settings.Commands, command)
	}
	quiescent := running
	for _, target := range targets {
		quiescent = quiescentAP(quiescent, target)
	}
	settingChanges, err := preimageSettings(settings, quiescent)
	if err != nil {
		return nil, err
	}
	var leading, resources, trailing []Change
	for _, target := range targets {
		transition, err := prepareScheduleTransition(target, desired[target], states[target], running, len(settingChanges) != 0)
		if err != nil {
			return nil, err
		}
		if len(transition) == 0 {
			continue
		}
		head, tail := 1, 1
		for _, line := range blocks[target] {
			if strings.HasPrefix(line, "schedule ") {
				head++
			}
		}
		if desired[target] != nil && states[target] {
			tail++
		}
		leading = append(leading, transition[:head]...)
		resources = append(resources, transition[head:len(transition)-tail]...)
		trailing = append(trailing, transition[len(transition)-tail:]...)
	}
	changes := append(leading, resources...)
	changes = append(changes, settingChanges...)
	return append(changes, trailing...), nil
}

func quiescentAP(running, target string) string {
	running = removeAPSchedule(running, target)
	var out []string
	inside := false
	for _, line := range strings.Split(running, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "interface ") {
			inside = line == "interface "+target
		}
		if inside && (strings.TrimSpace(line) == "up" || strings.TrimSpace(line) == "down") {
			line = "    down"
		}
		out = append(out, line)
		if line == "!" {
			inside = false
		}
	}
	return strings.Join(out, "\n")
}
