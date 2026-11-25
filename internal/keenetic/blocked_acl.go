// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"sort"
	"strings"
)

// ComposeBlockedPlan requires an explicit controller ACL for each configured
// WLAN. Its durable controller snapshot remains the baseline, so no native
// operator ACL is inferred, captured or claimed by this operation.
func ComposeBlockedPlan(plan Plan, blocked []string) (Plan, error) {
	result := plan
	result.AccessLists = nil
	owned := map[string]bool{}
	for _, base := range plan.AccessLists {
		if owned[base.Target] {
			return Plan{}, fmt.Errorf("duplicate block baseline")
		}
		owned[base.Target] = true
		acl, err := ComposeBlockedACL(base, blocked)
		if err != nil {
			return Plan{}, err
		}
		result.AccessLists = append(result.AccessLists, acl)
	}
	missing := map[string]bool{}
	for _, command := range plan.Commands {
		parts := strings.SplitN(command.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" && regexpAP.MatchString(parts[1]) &&
			strings.HasPrefix(parts[2], "ssid ") && !owned[parts[1]] {
			missing[parts[1]] = true
		}
	}
	result.Unsupported = append([]string(nil), plan.Unsupported...)
	for target := range missing {
		result.Unsupported = append(result.Unsupported, "client-block."+target+".baseline-missing")
	}
	sort.Strings(result.Unsupported)
	return result, nil
}

// ComposeBlockedACL intersects a controller-owned baseline ACL with a block
// list. The caller must retain that baseline separately: feeding the result
// back as the next baseline would make unblock lose the original policy.
// This function does not establish ownership or perform native operations.
func ComposeBlockedACL(base AccessList, blocked []string) (AccessList, error) {
	if !regexpAP.MatchString(base.Target) || (base.Policy != "none" && base.Policy != "permit" && base.Policy != "deny") {
		return AccessList{}, fmt.Errorf("invalid block overlay baseline")
	}
	entries, denied := map[string]bool{}, map[string]bool{}
	for _, value := range base.MACs {
		mac, err := clientMAC(value)
		if err != nil {
			return AccessList{}, err
		}
		entries[mac] = true
	}
	for _, value := range blocked {
		mac, err := clientMAC(value)
		if err != nil {
			return AccessList{}, err
		}
		denied[mac] = true
	}
	result := AccessList{Target: base.Target, Policy: base.Policy}
	switch base.Policy {
	case "permit":
		for mac := range denied {
			delete(entries, mac)
		}
	case "deny":
		for mac := range denied {
			entries[mac] = true
		}
	case "none":
		// Dormant baseline entries must not become active deny rules. The
		// untouched baseline is what a later unblock must restore instead.
		if len(denied) != 0 {
			result.Policy = "deny"
			entries = denied
		}
	}
	for mac := range entries {
		result.MACs = append(result.MACs, mac)
	}
	sort.Strings(result.MACs)
	return result, nil
}
