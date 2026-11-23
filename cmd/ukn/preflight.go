// SPDX-License-Identifier: GPL-2.0-or-later
package main

import "github.com/antoniocosta/unifi-openwrt/internal/keenetic"

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
