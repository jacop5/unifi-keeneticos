// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/translate"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// BuildWLANTestPlan restricts connected testing to explicitly mapped,
// non-primary AP slots. Radio-wide and unrelated WLAN settings never enter
// the planner. Only separately named UKN-Test SSIDs can be provisioned.
func BuildWLANTestPlan(text string, slots map[string]string) (Plan, error) {
	return BuildWLANTestPlanWithFT(text, slots, nil)
}

// BuildWLANTestPlanWithFT retains all test scoping with explicit FT domains.
func BuildWLANTestPlanWithFT(text string, slots, domains map[string]string) (Plan, error) {
	text, err := normalizeScheduleWindows(text)
	if err != nil {
		return Plan{}, err
	}
	if len(slots) == 0 {
		return Plan{}, fmt.Errorf("test mode requires explicit non-primary slots")
	}
	for _, target := range slots {
		if !regexp.MustCompile(`^WifiMaster[01]/AccessPoint[1-6]$`).MatchString(target) {
			return Plan{}, fmt.Errorf("test mode refuses primary or invalid AP slots")
		}
	}
	cfg := translate.ParseCfg(text)
	var keys []string
	for key := range cfg {
		parts := strings.Split(key, ".")
		if len(parts) >= 3 && (parts[0] == "wireless" || parts[0] == "aaa") {
			if _, ok := slots[parts[1]]; ok {
				keys = append(keys, key)
			}
		}
		if strings.HasPrefix(key, "vlan.") || strings.HasPrefix(key, "bridge.") || strings.HasPrefix(key, "macacl.") || strings.HasPrefix(key, "stamgr.") {
			keys = append(keys, key)
		}
	}
	activeSlots := map[string]string{}
	var missing []string
	for id, target := range slots {
		ssid := cfg["wireless."+id+".ssid"]
		if ssid == "" {
			missing = append(missing, target)
			continue
		}
		if !strings.HasPrefix(ssid, "UKN-Test") {
			return Plan{}, fmt.Errorf("test WLAN must have a UKN-Test SSID")
		}
		activeSlots[id] = target
	}
	sort.Strings(keys)
	var selected strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&selected, "%s=%s\n", key, cfg[key])
	}
	p, err := BuildPlanWithOptions(selected.String(), activeSlots, PlannerOptions{FTDomains: domains})
	if err != nil {
		return p, err
	}
	sort.Strings(missing)
	for _, target := range missing {
		p.Commands = append(p.Commands, Command{Text: "interface " + target + " down"})
	}
	return p, nil
}

// ValidateTestOwnership prevents a stale mapping from modifying a slot that
// has since been assigned to a serving/non-test SSID, including paused plans.
func ValidateTestOwnership(running string, slots map[string]string) error {
	selected := map[string]bool{}
	for _, target := range slots {
		selected[target] = true
	}
	current := ""
	for _, line := range strings.Split(running, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "interface ") {
			current = strings.TrimPrefix(line, "interface ")
		}
		if line == "!" {
			current = ""
		}
		if selected[current] && strings.HasPrefix(strings.TrimSpace(line), "ssid ") {
			ssid := strings.TrimPrefix(strings.TrimSpace(line), "ssid ")
			if strings.HasPrefix(ssid, `"`) {
				unquoted, err := strconv.Unquote(ssid)
				if err != nil {
					return fmt.Errorf("cannot parse test-slot SSID")
				}
				ssid = unquoted
			}
			if !strings.HasPrefix(ssid, "UKN-Test") {
				return fmt.Errorf("mapped slot contains a non-test SSID")
			}
		}
	}
	return nil
}
