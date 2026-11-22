// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"regexp"
	"sort"
	"strconv"
)

var nativeEthernetPort = regexp.MustCompile(`^(?:GigabitEthernet|FastEthernet)[0-9]+/[0-9]+$`)

// PortFields publishes measured physical links, not inferred WAN/uplink roles.
// Native port IDs disambiguate ports sharing a switch-local index or label.
func (s *Snapshot) PortFields() []map[string]any {
	ports := map[string]map[string]string{}
	for _, p := range s.Ports {
		if nativeEthernetPort.MatchString(p["id"]) {
			ports[p["id"]] = p
		}
	}
	ids := make([]string, 0, len(ports))
	for id := range ports {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []map[string]any{}
	for i, id := range ids {
		p := ports[id]
		if p["link"] != "up" && p["link"] != "down" {
			continue
		}
		up := p["link"] == "up"
		name := p["label"]
		if name == "" {
			name = p["interface-name"]
		}
		if name == "" {
			name = id
		}
		row := map[string]any{"port_idx": i + 1, "name": name, "ifname": id, "up": up}
		if !up {
			row["speed"] = int64(0)
		} else if speed, err := strconv.ParseInt(p["speed"], 10, 64); err == nil && speed > 0 {
			row["speed"] = speed
		}
		if !up {
			row["full_duplex"] = false
		} else if p["duplex"] == "full" || p["duplex"] == "half" {
			row["full_duplex"] = p["duplex"] == "full"
		}
		if p["auto-negotiation"] == "on" || p["auto-negotiation"] == "off" {
			row["autoneg"] = p["auto-negotiation"] == "on"
		}
		out = append(out, row)
	}
	return out
}
