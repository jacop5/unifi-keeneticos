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

// VerifyBridgeMembership checks the applied network binding, including native
// aliases. An operational SSID alone does not prove VLAN separation.
func VerifyBridgeMembership(plan Plan, running string) error {
	aliases := map[string]string{}
	members := map[string][]string{}
	current := ""
	for _, line := range strings.Split(running, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "interface ") {
			current = strings.TrimPrefix(line, "interface ")
			aliases[current] = current
			continue
		}
		if line == "!" {
			current = ""
		}
		if current == "" {
			continue
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "rename ") {
			aliases[strings.Trim(strings.TrimPrefix(line, "rename "), `"`)] = current
		}
		if strings.HasPrefix(line, "include ") {
			members[current] = append(members[current], strings.TrimPrefix(line, "include "))
		}
	}
	for _, command := range plan.Commands {
		fields := strings.Fields(command.Text)
		if len(fields) != 4 || fields[0] != "interface" || fields[2] != "include" {
			continue
		}
		found := false
		for bridge, ports := range members {
			for _, port := range ports {
				if aliases[port] != fields[3] {
					continue
				}
				if bridge != fields[1] {
					return fmt.Errorf("mapped AP belongs to an unexpected bridge")
				}
				found = true
			}
		}
		if !found {
			return fmt.Errorf("mapped AP missing from planned bridge")
		}
	}
	return nil
}

// VerifyWLANSettings checks native security flags before a transaction is
// confirmed. Being up on the correct bridge does not prove the requested
// encryption was applied. Password contents remain private; successful client
// authentication is a separate end-to-end check.
func VerifyWLANSettings(plan Plan, running string) error {
	blocks := interfaceBlocks(running)
	for _, c := range plan.Commands {
		fields := strings.SplitN(c.Text, " ", 3)
		if len(fields) != 3 || fields[0] != "interface" || !regexpAP.MatchString(fields[1]) {
			continue
		}
		lines, exists := blocks[fields[1]]
		if !exists {
			return fmt.Errorf("mapped WLAN missing from native readback")
		}
		setting := strings.TrimPrefix(fields[2], "no ")
		enabled := setting == fields[2]
		checked := false
		for _, flag := range []string{"encryption enable", "encryption wpa", "encryption wpa2", "encryption wpa3", "encryption owe", "hide-ssid", "peer-isolation", "rrm", "wmm", "ft enable"} {
			if setting != flag {
				continue
			}
			present := false
			for _, line := range lines {
				present = present || line == flag
			}
			if present != enabled {
				return fmt.Errorf("native WLAN flag does not match plan: %s", flag)
			}
			checked = true
			break
		}
		if checked {
			continue
		}
		if setting == "pmf" || setting == "pmf force" {
			actual := ""
			for _, line := range lines {
				if line == "pmf" || line == "pmf force" {
					actual = line
				}
			}
			want := ""
			if enabled {
				want = setting
			}
			if actual != want {
				return fmt.Errorf("native WLAN PMF mode does not match plan")
			}
		}
		if strings.HasPrefix(setting, "authentication wpa-psk") {
			present := false
			for _, line := range lines {
				present = present || strings.HasPrefix(line, "authentication wpa-psk ")
			}
			if present != enabled {
				return fmt.Errorf("native WLAN key presence does not match plan")
			}
		}
		if strings.HasPrefix(setting, "rssi-threshold") {
			actual := "no rssi-threshold"
			for _, line := range lines {
				if strings.HasPrefix(line, "rssi-threshold ") && line != "rssi-threshold 0" {
					actual = line
				}
			}
			if actual != fields[2] {
				return fmt.Errorf("native minimum RSSI does not match plan")
			}
		}
	}
	return nil
}
