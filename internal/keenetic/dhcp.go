// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"net"
	"strconv"
	"strings"
	"unicode"
)

// ParseDHCPHostnames uses only unexpired IPv4 leases. Conflicting names for a
// MAC are omitted rather than selected according to native output order.
func ParseDHCPHostnames(text string) map[string]string {
	out := map[string]string{}
	conflicts := map[string]bool{}
	for _, lease := range Records(text, "lease") {
		mac, err := net.ParseMAC(lease["mac"])
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 || net.ParseIP(lease["ip"]).To4() == nil {
			continue
		}
		if lease["expires"] != "infinity" {
			expires, err := strconv.ParseInt(lease["expires"], 10, 64)
			if err != nil || expires <= 0 {
				continue
			}
		}
		name := strings.TrimSpace(lease["hostname"])
		if name == "" || len(name) > 253 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			continue
		}
		id := mac.String()
		if old := out[id]; old != "" && old != name {
			conflicts[id] = true
		}
		out[id] = name
	}
	for id := range conflicts {
		delete(out, id)
	}
	return out
}
