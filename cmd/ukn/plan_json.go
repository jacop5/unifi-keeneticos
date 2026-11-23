// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"strings"
)

// Never marshal Plan directly: Command.Text can contain WLAN credentials.
func redactedPlan(plan keenetic.Plan, changes *int) map[string]any {
	commands := []string{}
	targets := []string{}
	for _, command := range plan.Commands {
		commands = append(commands, command.String())
		target := ""
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" {
			target = parts[1]
		}
		targets = append(targets, target)
	}
	acls := []map[string]any{}
	for _, acl := range plan.AccessLists {
		acls = append(acls, map[string]any{"target": acl.Target, "policy": acl.Policy, "client_count": len(acl.MACs)})
	}
	schedules := []map[string]any{}
	for _, schedule := range plan.Schedules {
		actions := []string{}
		for _, action := range schedule.Actions {
			actions = append(actions, action.String())
		}
		schedules = append(schedules, map[string]any{"name": schedule.Name, "target": schedule.Target, "actions": actions})
	}
	unsupported := append([]string{}, plan.Unsupported...)
	return map[string]any{"schema_version": 1, "commands": commands, "command_targets": targets, "unsupported": unsupported, "planner_supported": len(unsupported) == 0, "access_lists": acls, "schedules": schedules, "preflight_passed": changes != nil, "preflight_changes": changes}
}
