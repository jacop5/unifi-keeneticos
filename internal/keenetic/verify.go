// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strconv"
	"strings"
)

// VerifyWLANs checks every explicitly enabled mapped AP, not merely whether
// an unrelated pre-existing WLAN is still operational.
func VerifyWLANs(plan Plan, s *Snapshot) error {
	ssids := map[string]string{}
	states := map[string]string{}
	for _, c := range plan.Commands {
		fields := strings.SplitN(c.Text, " ", 3)
		if len(fields) != 3 || !strings.Contains(fields[1], "/AccessPoint") {
			continue
		}
		if strings.HasPrefix(fields[2], "ssid ") {
			ssid, err := strconv.Unquote(strings.TrimPrefix(fields[2], "ssid "))
			if err != nil {
				return fmt.Errorf("invalid planned SSID")
			}
			ssids[fields[1]] = ssid
		}
		if fields[2] == "up" || fields[2] == "down" {
			states[fields[1]] = fields[2]
		}
	}
	for id, state := range states {
		found := false
		for _, v := range s.VAPs {
			if v["id"] == id {
				found = true
				if state == "down" {
					return fmt.Errorf("disabled mapped AP remains active")
				}
				if ssid, ok := ssids[id]; ok && ssid != v["ssid"] {
					return fmt.Errorf("mapped AP SSID does not match plan")
				}
			}
		}
		if state == "up" && !found {
			return fmt.Errorf("enabled mapped AP is not operational")
		}
	}
	return nil
}
