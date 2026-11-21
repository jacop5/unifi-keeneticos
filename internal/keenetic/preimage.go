// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strings"
)

// Preimage attaches inverses using only existing interface blocks. It never
// restores an entire router configuration over unrelated WAN or VPN settings.
func Preimage(plan Plan, running string) ([]Change, error) {
	if len(plan.Unsupported) > 0 {
		return nil, fmt.Errorf("plan contains unsupported settings")
	}
	blocks := map[string][]string{}
	current := ""
	for _, line := range strings.Split(running, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "interface ") {
			current = strings.TrimSpace(strings.TrimPrefix(line, "interface "))
			blocks[current] = []string{}
			continue
		}
		if line == "!" {
			current = ""
			continue
		}
		if current != "" && strings.HasPrefix(line, " ") {
			blocks[current] = append(blocks[current], strings.TrimSpace(line))
		}
	}
	var changes []Change
	aliases := map[string]string{}
	for id, lines := range blocks {
		aliases[id] = id
		for _, line := range lines {
			if strings.HasPrefix(line, "rename ") {
				aliases[strings.Trim(strings.TrimPrefix(line, "rename "), `"`)] = id
			}
		}
	}
	for _, c := range plan.Commands {
		fields := strings.SplitN(c.Text, " ", 3)
		if len(fields) != 3 || fields[0] != "interface" {
			return nil, fmt.Errorf("unexpected planned command")
		}
		lines, exists := blocks[fields[1]]
		if !exists {
			return nil, fmt.Errorf("target interface does not exist")
		}
		setting := fields[2]
		setting = strings.TrimPrefix(setting, "no ")
		if strings.HasPrefix(setting, "include ") {
			target := strings.TrimPrefix(setting, "include ")
			if _, ok := blocks[target]; !ok {
				return nil, fmt.Errorf("bridge target does not exist")
			}
			already := false
			for bridge, items := range blocks {
				for _, line := range items {
					if strings.HasPrefix(line, "include ") && aliases[strings.TrimPrefix(line, "include ")] == target {
						if bridge != fields[1] {
							return nil, fmt.Errorf("mapped AP belongs to another bridge")
						}
						already = true
					}
				}
			}
			if !already {
				changes = append(changes, Change{Forward: c, Undo: Command{Text: "interface " + fields[1] + " no include " + target}})
			}
			continue
		}
		if setting == "up" || setting == "down" {
			previous := ""
			for _, line := range lines {
				if line == "up" || line == "down" {
					previous = line
					break
				}
			}
			if previous == "" {
				return nil, fmt.Errorf("interface administrative state missing from preimage")
			}
			if previous != setting {
				changes = append(changes, Change{Forward: c, Undo: Command{Text: "interface " + fields[1] + " " + previous}})
			}
			continue
		}
		family := ""
		for _, f := range []string{"channel width", "channel", "ssid", "authentication wpa-psk", "encryption enable", "encryption wpa2", "encryption wpa3", "encryption wpa", "hide-ssid", "pmf", "peer-isolation", "rrm"} {
			if setting == f || strings.HasPrefix(setting, f+" ") {
				family = f
				break
			}
		}
		if family == "" {
			return nil, fmt.Errorf("no inverse rule for command")
		}
		undo := "no " + family
		for _, line := range lines {
			if line == family || strings.HasPrefix(line, family+" ") {
				if family == "channel" && strings.HasPrefix(line, "channel width") {
					continue
				}
				undo = line
				break
			}
		}
		inverse := Command{Text: "interface " + fields[1] + " " + undo, Secret: c.Secret}
		if c.Text == inverse.Text {
			continue
		}
		changes = append(changes, Change{Forward: c, Undo: inverse})
	}
	return changes, nil
}
