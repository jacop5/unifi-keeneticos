// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

var regexpAP = regexp.MustCompile(`^WifiMaster[01]/AccessPoint[0-7]$`)

type AccessList struct {
	Target string
	Policy string // none, permit, deny (native NDM spelling)
	MACs   []string
}

func clientMAC(value string) (string, error) {
	m, err := net.ParseMAC(value)
	if err != nil || len(m) != 6 || m[0]&1 != 0 || m.String() == "00:00:00:00:00:00" {
		return "", fmt.Errorf("invalid unicast client MAC")
	}
	return m.String(), nil
}

func controllerACL(cfg translate.Cfg, dev, target string) (*AccessList, error) {
	if cfg["macacl.status"] == "" {
		return nil, nil // no controller request: retain an operator's native ACL
	}
	a := &AccessList{Target: target, Policy: "none"}
	if cfg["macacl.status"] == "disabled" {
		return a, nil
	}
	if cfg["macacl.status"] != "enabled" || dev == "" {
		return nil, fmt.Errorf("invalid MAC ACL status or missing WLAN device name")
	}
	owner := ""
	for key, value := range cfg {
		if strings.HasPrefix(key, "macacl.") && strings.HasSuffix(key, ".devname") && value == dev {
			if owner != "" {
				return nil, fmt.Errorf("duplicate ACL for WLAN device")
			}
			owner = strings.TrimSuffix(key, "devname")
		}
	}
	if owner == "" || cfg[owner+"status"] == "disabled" || cfg[owner+"acl.status"] == "disabled" {
		return a, nil
	}
	if cfg[owner+"status"] != "enabled" || cfg[owner+"acl.status"] != "enabled" {
		return nil, fmt.Errorf("incomplete MAC ACL status")
	}
	switch cfg[owner+"acl.policy"] {
	case "allow":
		a.Policy = "permit"
	case "deny":
		a.Policy = "deny"
	default:
		return nil, fmt.Errorf("unsupported MAC ACL policy")
	}
	seen := map[string]bool{}
	for key, value := range cfg {
		if !strings.HasPrefix(key, owner+"acl.") || !strings.HasSuffix(key, ".mac") {
			continue
		}
		status := cfg[strings.TrimSuffix(key, "mac")+"status"]
		if status == "disabled" {
			continue
		}
		if status != "" && status != "enabled" {
			return nil, fmt.Errorf("invalid MAC ACL entry status")
		}
		mac, err := clientMAC(value)
		if err != nil {
			return nil, err
		}
		seen[mac] = true
	}
	for mac := range seen {
		a.MACs = append(a.MACs, mac)
	}
	sort.Strings(a.MACs)
	if a.Policy == "deny" && len(a.MACs) == 0 {
		a.Policy = "none"
	}
	return a, nil
}

func nativeACL(lines []string) (string, []string, string, error) {
	policy, state := "none", ""
	seen := map[string]bool{}
	for _, line := range lines {
		if line == "up" || line == "down" {
			state = line
		}
		if strings.HasPrefix(line, "mac access-list type ") {
			policy = strings.TrimPrefix(line, "mac access-list type ")
		}
		if strings.HasPrefix(line, "mac access-list address ") {
			mac, err := clientMAC(strings.TrimPrefix(line, "mac access-list address "))
			if err != nil {
				return "", nil, "", err
			}
			seen[mac] = true
		}
	}
	if policy != "none" && policy != "permit" && policy != "deny" {
		return "", nil, "", fmt.Errorf("unknown native MAC ACL policy")
	}
	var macs []string
	for mac := range seen {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	return policy, macs, state, nil
}

// aclChanges quiesces an active AP while replacing its policy and list, so
// intermediate rules cannot expose an otherwise filtered WLAN. Every delta
// has an inverse; unrelated interface settings are never restored wholesale.
func aclChanges(a AccessList, lines []string) ([]Change, error) {
	if !regexpAP.MatchString(a.Target) || (a.Policy != "none" && a.Policy != "permit" && a.Policy != "deny") {
		return nil, fmt.Errorf("invalid native ACL target or policy")
	}
	oldPolicy, oldMACs, state, err := nativeACL(lines)
	if err != nil {
		return nil, err
	}
	old, want := map[string]bool{}, map[string]bool{}
	for _, mac := range oldMACs {
		old[mac] = true
	}
	for _, value := range a.MACs {
		mac, err := clientMAC(value)
		if err != nil {
			return nil, err
		}
		want[mac] = true
	}
	var changes []Change
	add := func(forward, undo string) {
		prefix := "interface " + a.Target + " "
		changes = append(changes, Change{Forward: Command{Text: prefix + forward}, Undo: Command{Text: prefix + undo}})
	}
	for _, mac := range oldMACs {
		if !want[mac] {
			add("no mac access-list address "+mac, "mac access-list address "+mac)
		}
	}
	var additions []string
	for mac := range want {
		if !old[mac] {
			additions = append(additions, mac)
		}
	}
	sort.Strings(additions)
	for _, mac := range additions {
		add("mac access-list address "+mac, "no mac access-list address "+mac)
	}
	if oldPolicy != a.Policy {
		add("mac access-list type "+a.Policy, "mac access-list type "+oldPolicy)
	}
	if len(changes) == 0 {
		return nil, nil
	}
	if state != "up" && state != "down" {
		return nil, fmt.Errorf("ACL target administrative state missing")
	}
	if state == "up" {
		middle := changes
		changes = nil
		add("down", "up")
		changes = append(changes, middle...)
		add("up", "down")
	}
	return changes, nil
}

func VerifyAccessLists(plan Plan, running string) error {
	blocks := interfaceBlocks(running)
	for _, a := range plan.AccessLists {
		lines, exists := blocks[a.Target]
		if !exists {
			return fmt.Errorf("ACL target missing after application")
		}
		changes, err := aclChanges(a, lines)
		if err != nil {
			return err
		}
		if len(changes) != 0 {
			return fmt.Errorf("applied MAC ACL does not match controller request")
		}
	}
	return nil
}
