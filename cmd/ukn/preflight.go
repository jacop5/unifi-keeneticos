// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"sort"
)

// preflightPlan has no Runner: neither forward nor recovery commands can run.
// A saved snapshot proves preparation only, never current state or health.
func preflightPlan(plan keenetic.Plan, running string, slots map[string]string, testOnly bool) (int, error) {
	if testOnly {
		if err := keenetic.ValidateTestOwnership(running, slots); err != nil {
			return 0, err
		}
	}
	changes, err := keenetic.Preimage(plan, running)
	if err != nil {
		return 0, err
	}
	return len(changes), nil
}

// Reject conflicting modes before any mode dispatch or native command runs.
func validateOfflineMode(snapshot, plan string, modes map[string]bool) error {
	if snapshot == "" {
		return nil
	}
	if plan == "" {
		return fmt.Errorf("-preflight-running requires -plan")
	}
	var conflicts []string
	for name, enabled := range modes {
		if enabled {
			conflicts = append(conflicts, name)
		}
	}
	sort.Strings(conflicts)
	if len(conflicts) > 0 {
		return fmt.Errorf("-preflight-running cannot be combined with -%s", conflicts[0])
	}
	return nil
}
