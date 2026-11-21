// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"sort"
	"strings"
)

func interfaceBlocks(running string) map[string][]string {
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
	return blocks
}

// Preimage attaches inverses using only existing interface blocks. It never
// restores an entire router configuration over unrelated WAN or VPN settings.
func Preimage(plan Plan, running string) ([]Change, error) {
	if len(plan.Unsupported) > 0 {
		return nil, fmt.Errorf("plan contains unsupported settings")
	}
	blocks := interfaceBlocks(running)
	known := map[string]bool{}
	for _, line := range strings.Split(running, "\n") {
		if !strings.HasPrefix(line, "known host ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			if mac, err := clientMAC(fields[len(fields)-1]); err == nil {
				known[mac] = true
			}
		}
	}
	var changes []Change
	countryResume := map[string]string{}
	seenACL := map[string]bool{}
	for _, acl := range plan.AccessLists {
		lines, exists := blocks[acl.Target]
		if !exists || seenACL[acl.Target] {
			return nil, fmt.Errorf("missing or duplicate ACL target")
		}
		seenACL[acl.Target] = true
		delta, err := aclChanges(acl, lines)
		if err != nil {
			return nil, err
		}
		// Native NDM only accepts registered hosts. Check both directions
		// before quiescing an AP; rollback must be able to restore old entries.
		for _, change := range delta {
			for _, command := range []Command{change.Forward, change.Undo} {
				if _, mac, ok := strings.Cut(command.Text, " mac access-list address "); ok && !known[mac] {
					return nil, fmt.Errorf("MAC ACL requires existing Keenetic known-host registrations")
				}
			}
		}
		changes = append(changes, delta...)
	}
	aliases := map[string]string{}
	for id, lines := range blocks {
		aliases[id] = id
		for _, line := range lines {
			if strings.HasPrefix(line, "rename ") {
				aliases[strings.Trim(strings.TrimPrefix(line, "rename "), `"`)] = id
			}
		}
	}
	for commandIndex, c := range plan.Commands {
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
		if strings.HasPrefix(setting, "country-code") {
			if fields[2] != setting || !validCountrySetting(setting) || (fields[1] != "WifiMaster0" && fields[1] != "WifiMaster1") {
				return nil, fmt.Errorf("invalid radio country command")
			}
			previous := ""
			for _, line := range lines {
				if strings.HasPrefix(line, "country-code ") {
					if !validCountrySetting(line) {
						return nil, fmt.Errorf("invalid native country preimage")
					}
					previous = line
				}
			}
			// NDM has no 'no country-code'. Never guess a firmware default
			// as an inverse for a shared-radio regulatory change.
			if previous == "" {
				return nil, fmt.Errorf("country change requires an explicit native country preimage")
			}
			if previous != setting {
				if _, exists := countryResume[fields[1]]; exists {
					return nil, fmt.Errorf("duplicate country change")
				}
				channel, width := false, false
				prefix := "interface " + fields[1] + " "
				for i, candidate := range plan.Commands {
					if !strings.HasPrefix(candidate.Text, prefix) {
						continue
					}
					s := strings.TrimPrefix(candidate.Text, prefix)
					isWidth := strings.HasPrefix(s, "channel width ")
					isChannel := s == "no channel" || (strings.HasPrefix(s, "channel ") && !isWidth)
					if i < commandIndex && (isWidth || isChannel || strings.HasPrefix(s, "no channel")) {
						return nil, fmt.Errorf("country must precede channel and width")
					}
					channel, width = channel || isChannel, width || isWidth
				}
				if !channel || !width {
					return nil, fmt.Errorf("country changes require explicit channel and width requests")
				}
				state, oldChannel, oldWidth := "", "no channel", "no channel width"
				for _, line := range lines {
					switch {
					case line == "up" || line == "down":
						state = line
					case strings.HasPrefix(line, "channel width "):
						oldWidth = line
					case strings.HasPrefix(line, "channel "):
						oldChannel = line
					}
				}
				if state == "" {
					return nil, fmt.Errorf("country change requires native radio administrative state")
				}
				add := func(forward, undo string) {
					changes = append(changes, Change{Forward: Command{Text: prefix + forward}, Undo: Command{Text: prefix + undo}})
				}
				if state == "up" {
					add("down", "up")
				}
				if oldWidth != "channel width 20" {
					add("channel width 20", oldWidth)
				}
				if oldChannel != "no channel" {
					add("no channel", oldChannel)
				}
				changes = append(changes, Change{Forward: c, Undo: Command{Text: "interface " + fields[1] + " " + previous}})
				countryResume[fields[1]] = state
				// Later channel/width inverses return to the neutral state,
				// then the country inverse runs, then old channel/width/state.
				var neutral []string
				for _, line := range lines {
					if line == "up" || line == "down" || strings.HasPrefix(line, "channel ") || strings.HasPrefix(line, "country-code ") {
						continue
					}
					neutral = append(neutral, line)
				}
				blocks[fields[1]] = append(neutral, "down", "channel width 20", setting)
			}
			continue
		}
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
			if _, pending := countryResume[fields[1]]; pending {
				countryResume[fields[1]] = setting
				continue
			}
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
		for _, f := range []string{"channel width", "channel", "ssid", "authentication wpa-psk", "encryption enable", "encryption wpa2", "encryption wpa3", "encryption wpa", "encryption owe", "hide-ssid", "pmf", "peer-isolation", "rrm"} {
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
	var masters []string
	for master := range countryResume {
		masters = append(masters, master)
	}
	sort.Strings(masters)
	for _, master := range masters {
		// Keep the inverse down barrier even for a desired down state:
		// startup recovery replays the complete journal, including when a
		// crash happened before the first forward command.
		changes = append(changes, Change{Forward: Command{Text: "interface " + master + " " + countryResume[master]}, Undo: Command{Text: "interface " + master + " down"}})
	}
	return changes, nil
}
