// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"sort"
)

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
