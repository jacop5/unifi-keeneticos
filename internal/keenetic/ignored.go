// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"sort"
	"strconv"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

// Per-client shaping has not been ported. Report only recognized setting
// names for this WLAN, never their values or unrelated VAP policy.
func ignoredQoSLimits(cfg translate.Cfg, devname string) []string {
	if devname == "" || cfg["qos.status"] != "enabled" {
		return nil
	}
	var ignored []string
	for key, value := range cfg {
		parts := strings.Split(key, ".")
		if value == "" || len(parts) < 5 || parts[0] != "qos" || parts[1] != "vap" {
			continue
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil || n < 0 || strconv.Itoa(n) != parts[2] || cfg["qos.vap."+parts[2]+".devname"] != devname {
			continue
		}
		if len(parts) == 5 && parts[3] == "dwnlink" && parts[4] == "maxspeed" {
			ignored = append(ignored, key)
		}
		if len(parts) == 6 && parts[3] == "uplink" && parts[5] == "maxspeed" {
			u, err := strconv.Atoi(parts[4])
			if err == nil && u >= 0 && strconv.Itoa(u) == parts[4] {
				ignored = append(ignored, key)
			}
		}
	}
	sort.Strings(ignored)
	return ignored
}
