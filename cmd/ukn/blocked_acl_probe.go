// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"fmt"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

// Called only after the unused, disabled, SSID-free probe guard. Reuse an
// already registered host; never create a router-wide registration for a probe.
func blockedACLProbePlan(target, running string) (keenetic.Plan, error) {
	for _, line := range strings.Split(running, "\n") {
		if !strings.HasPrefix(line, "known host ") {
			continue
		}
		fields := strings.Fields(line)
		base := keenetic.AccessList{Target: target, Policy: "none"}
		acl, err := keenetic.ComposeBlockedACL(base, []string{fields[len(fields)-1]})
		if err != nil {
			continue
		}
		return keenetic.Plan{
			Commands:    []keenetic.Command{{Text: "interface " + target + " ssid ukn-rollback-probe"}},
			AccessLists: []keenetic.AccessList{acl},
		}, nil
	}
	return keenetic.Plan{}, fmt.Errorf("block ACL probe requires an existing valid known-host registration")
}
