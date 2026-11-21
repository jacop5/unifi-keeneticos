// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

// minimumRSSI translates only explicit requests. Missing station-manager
// settings leave an operator's native per-AP threshold alone.
func minimumRSSI(cfg translate.Cfg) (map[string]string, []string, error) {
	switch cfg["stamgr.status"] {
	case "disabled":
		return map[string]string{"ng": "no rssi-threshold", "na": "no rssi-threshold"}, nil, nil
	case "", "enabled":
	default:
		return nil, nil, fmt.Errorf("invalid station-manager status")
	}
	entries := map[string]bool{}
	for key := range cfg {
		parts := strings.Split(key, ".")
		if len(parts) >= 3 && parts[0] == "stamgr" {
			entries[parts[1]] = true
		}
	}
	var ids []string
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	thresholds := map[string]string{}
	owners := map[string]bool{}
	var unsupported []string
	for _, id := range ids {
		prefix := "stamgr." + id + "."
		band := cfg[prefix+"radio"]
		if band != "ng" && band != "na" {
			unsupported = append(unsupported, prefix+"radio")
			continue
		}
		if owners[band] {
			return nil, nil, fmt.Errorf("multiple station-manager entries for one native band")
		}
		owners[band] = true
		if cfg[prefix+"loadbalance.status"] == "true" {
			unsupported = append(unsupported, prefix+"loadbalance")
		}
		switch cfg[prefix+"minrssi.status"] {
		case "":
			continue
		case "false":
			thresholds[band] = "no rssi-threshold"
		case "true":
			value, err := strconv.Atoi(cfg[prefix+"minrssi.rssi"])
			if err != nil {
				return nil, nil, fmt.Errorf("invalid minimum RSSI")
			}
			if value == 0 {
				thresholds[band] = "no rssi-threshold"
				continue
			}
			// The upstream controller format also uses RSSI above its
			// reference noise floor; retain the upstream conversion.
			if value > 0 {
				value -= 95
			}
			if value < -100 || value >= 0 {
				return nil, nil, fmt.Errorf("minimum RSSI is outside native range")
			}
			thresholds[band] = "rssi-threshold " + strconv.Itoa(value)
		default:
			return nil, nil, fmt.Errorf("invalid minimum RSSI status")
		}
	}
	return thresholds, unsupported, nil
}
