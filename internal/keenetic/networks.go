// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

var nativeNetworkBridge = regexp.MustCompile(`^Bridge[1-9][0-9]*$`)

func vlanID(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n >= 1 && n <= 4094 && strconv.Itoa(n) == value
}

// resolveNetworks requires an explicit native mapping AND a complete UniFi
// VLAN/bridge membership description. An unknown or partial tagged network
// remains unsupported; it must never degrade to an untagged Home WLAN.
func resolveNetworks(cfg translate.Cfg, networks map[string]string) (map[string]string, []string, error) {
	owners := map[string]string{}
	for id, bridge := range networks {
		if !vlanID(id) || !nativeNetworkBridge.MatchString(bridge) {
			return nil, nil, fmt.Errorf("invalid VLAN bridge mapping")
		}
		if prior := owners[bridge]; prior != "" {
			return nil, nil, fmt.Errorf("VLANs %s and %s share a native bridge", prior, id)
		}
		owners[bridge] = id
	}
	declared := map[string]bool{}
	uplinks := map[string]map[string]bool{}
	var unsupported []string
	for key, value := range cfg {
		if strings.HasPrefix(key, "vlan.") && strings.HasSuffix(key, ".id") && value != "" && value != "0" {
			if !vlanID(value) || networks[value] == "" {
				unsupported = append(unsupported, key)
			}
			declared[value] = true
			dev := cfg[strings.TrimSuffix(key, "id")+"devname"]
			if dev != "" {
				if uplinks[value] == nil {
					uplinks[value] = map[string]bool{}
				}
				uplinks[value][dev+"."+value] = true
			}
		}
	}
	devs := map[string]string{}
	for key, value := range cfg {
		if strings.HasPrefix(key, "wireless.") && strings.HasSuffix(key, ".devname") {
			id := strings.TrimSuffix(strings.TrimPrefix(key, "wireless."), ".devname")
			if prior := devs[value]; value != "" && prior != "" && prior != id {
				return nil, nil, fmt.Errorf("duplicate WLAN device name")
			}
			devs[value] = id
		}
	}
	bridges := map[string]string{}
	seen := map[string]string{}
	used := map[string]bool{}
	for key, value := range cfg {
		if !strings.HasPrefix(key, "bridge.") || !strings.HasSuffix(key, ".devname") || strings.Contains(key, ".port.") {
			continue
		}
		prefix := strings.TrimSuffix(key, "devname") + "port."
		native := "Bridge0"
		tag := ""
		if dot := strings.LastIndexByte(value, '.'); dot >= 0 {
			id := value[dot+1:]
			if !vlanID(id) || !declared[id] || networks[id] == "" {
				unsupported = append(unsupported, key)
				continue
			}
			native = networks[id]
			tag = id
			used[id] = true
		}
		matchedUplink := tag == ""
		for port, dev := range cfg {
			if strings.HasPrefix(port, prefix) && strings.HasSuffix(port, ".devname") && uplinks[tag][dev] {
				matchedUplink = true
			}
		}
		if !matchedUplink {
			unsupported = append(unsupported, key+".uplink")
		}
		for port, dev := range cfg {
			if !strings.HasPrefix(port, prefix) || !strings.HasSuffix(port, ".devname") || devs[dev] == "" {
				continue
			}
			id := devs[dev]
			if prior := seen[id]; prior != "" && prior != key {
				return nil, nil, fmt.Errorf("WLAN %s belongs to multiple bridges", id)
			}
			seen[id], bridges[id] = key, native
		}
	}
	for id := range declared {
		if networks[id] != "" && !used[id] {
			unsupported = append(unsupported, "vlan."+id+".bridge")
		}
	}
	// Explicit network ownership must not depend on the controller including
	// a VLAN declaration: an incomplete VLAN-less snapshot is ambiguous too.
	if len(networks) > 0 {
		for key := range cfg {
			if strings.HasPrefix(key, "wireless.") && strings.HasSuffix(key, ".ssid") {
				id := strings.TrimSuffix(strings.TrimPrefix(key, "wireless."), ".ssid")
				if bridges[id] == "" {
					unsupported = append(unsupported, "wireless."+id+".network")
				}
			}
		}
	}
	return bridges, unsupported, nil
}
