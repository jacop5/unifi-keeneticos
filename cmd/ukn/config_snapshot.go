// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"sort"
)

// Identity is internal only: neither configuration text nor its digest belongs
// in logs/reports. A changed body under the same controller version is new work.
func provisioningConfig(s device.State) (text, identity string, verified bool) {
	text, verified = s.CurrentControllerConfig()
	digest := sha256.Sum256([]byte(s.InformURL + "\x00" + s.Key + "\x00" + text))
	return text, s.CfgVersion + ":" + hex.EncodeToString(digest[:]), verified
}

func requireProvisioningIdentity(s device.State, expected string, blockControl bool) error {
	if !s.Adopted {
		return fmt.Errorf("controller removed adoption during health check")
	}
	_, current, verified := provisioningConfigWithBlocks(s, blockControl)
	if !verified || current != expected {
		return fmt.Errorf("controller configuration or block policy changed during provisioning")
	}
	return nil
}

func provisioningConfigWithBlocks(s device.State, enabled bool) (text, identity string, verified bool) {
	text, identity, verified = provisioningConfig(s)
	if enabled {
		blocked := append([]string(nil), s.Blocked...)
		sort.Strings(blocked)
		encoded, _ := json.Marshal(blocked)
		digest := sha256.Sum256(append([]byte(identity+"\x00client-block\x00"), encoded...))
		identity = hex.EncodeToString(digest[:])
	}
	return
}
