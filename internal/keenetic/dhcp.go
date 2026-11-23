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
	return parseHostnames(Records(text, "lease"), func(r map[string]string) bool {
		if r["expires"] == "infinity" {
			return true
		}
		n, err := strconv.ParseInt(r["expires"], 10, 64)
		return err == nil && n > 0
	})
}

// ParseActiveHostnames excludes offline hosts; it never invents DHCP leases.
func ParseActiveHostnames(text string) map[string]string {
	return parseHostnames(Records(text, "host"), func(r map[string]string) bool { return r["link"] == "up" })
}

func parseHostnames(records []map[string]string, active func(map[string]string) bool) map[string]string {
	out := map[string]string{}
	conflicts := map[string]bool{}
	for _, lease := range records {
		if !active(lease) {
			continue
		}
		mac, err := net.ParseMAC(lease["mac"])
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 || net.ParseIP(lease["ip"]).To4() == nil {
			continue
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
