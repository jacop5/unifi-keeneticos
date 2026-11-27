// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/translate"
	"sort"
	"strings"
)

// BuildWLANPlanWithOptions restricts production provisioning to explicitly
// mapped WLAN slots. Unlike BuildWLANTestPlanWithFT it accepts arbitrary SSIDs
// and primary AP slots, but like the test planner it never lets radio-wide or
// unrelated WLAN settings enter the plan: only mapped wireless/aaa keys plus
// the bridge/vlan/macacl/stamgr/qos info the ordinary planner needs to resolve
// those WLANs are selected. Bridge mappings and FT domains are honored through
// the same PlannerOptions guards. Existing bridge membership can change; no
// bridge/uplink is created and no radio-wide setting changes. Slots must be explicit, nonempty, valid and unique.
func BuildWLANPlanWithOptions(text string, slots map[string]string, options PlannerOptions) (Plan, error) {
	text, err := normalizeScheduleWindows(text)
	if err != nil {
		return Plan{}, err
	}
	if len(slots) == 0 {
		return Plan{}, fmt.Errorf("wlan-only mode requires explicit nonempty slots")
	}
	owners := map[string]string{}
	for id, target := range slots {
		if target == "" {
			return Plan{}, fmt.Errorf("wlan-only mapping for WLAN %s has an empty AP slot", id)
		}
		if !regexpAP.MatchString(target) {
			return Plan{}, fmt.Errorf("wlan-only mode refuses invalid AP slot for WLAN %s", id)
		}
		if prior, exists := owners[target]; exists {
			return Plan{}, fmt.Errorf("WLANs %s and %s share a slot", prior, id)
		}
		owners[target] = id
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
		if strings.HasPrefix(key, "vlan.") || strings.HasPrefix(key, "bridge.") || strings.HasPrefix(key, "macacl.") || strings.HasPrefix(key, "stamgr.") || strings.HasPrefix(key, "qos.") {
			keys = append(keys, key)
		}
	}
	activeSlots := map[string]string{}
	var missing []string
	for id, target := range slots {
		if cfg["wireless."+id+".ssid"] == "" {
			missing = append(missing, target)
			continue
		}
		activeSlots[id] = target
	}
	sort.Strings(keys)
	var selected strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&selected, "%s=%s\n", key, cfg[key])
	}
	p, err := BuildPlanWithOptions(selected.String(), activeSlots, options)
	if err != nil {
		return p, err
	}
	sort.Strings(missing)
	for _, target := range missing {
		p.Commands = append(p.Commands, Command{Text: "interface " + target + " down"})
	}
	return p, nil
}
