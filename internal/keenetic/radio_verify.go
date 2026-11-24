// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strings"
)

// VerifyRadioSettings checks configured state, not measured channel availability.
// Unreported/default width values cannot prove an explicitly requested width.
func VerifyRadioSettings(plan Plan, running string) error {
	blocks := interfaceBlocks(running)
	wanted := map[string]map[string]string{}
	for _, c := range plan.Commands {
		parts := strings.SplitN(c.Text, " ", 3)
		if len(parts) != 3 || parts[0] != "interface" || (parts[1] != "WifiMaster0" && parts[1] != "WifiMaster1") {
			continue
		}
		setting := parts[2]
		family, value := "", ""
		switch {
		case setting == "up" || setting == "down":
			family, value = "state", setting
		case setting == "no channel":
			family = "channel"
		case setting == "no channel width":
			family = "channel width"
		case strings.HasPrefix(setting, "channel width "):
			family, value = "channel width", strings.TrimPrefix(setting, "channel width ")
		case strings.HasPrefix(setting, "channel ") && len(strings.Fields(setting)) == 2:
			family, value = "channel", strings.TrimPrefix(setting, "channel ")
		default:
			continue
		}
		if family == "channel" && value == "auto" {
			value = ""
		}
		if wanted[parts[1]] == nil {
			wanted[parts[1]] = map[string]string{}
		}
		wanted[parts[1]][family] = value
	}
	for target, settings := range wanted {
		lines, ok := blocks[target]
		if !ok {
			return fmt.Errorf("planned radio missing from native readback")
		}
		for family, want := range settings {
			actual := ""
			count := 0
			for _, line := range lines {
				value, match := "", false
				if family == "state" {
					value, match = line, line == "up" || line == "down"
				} else if strings.HasPrefix(line, family+" ") {
					value = strings.TrimPrefix(line, family+" ")
					match = family != "channel" || len(strings.Fields(value)) == 1
				}
				if match {
					actual = value
					count++
				}
			}
			if family == "channel" && actual == "auto" {
				actual = ""
			}
			if count > 1 || actual != want {
				return fmt.Errorf("native radio %s does not match plan", family)
			}
		}
	}
	return nil
}
