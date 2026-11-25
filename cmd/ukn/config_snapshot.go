// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/antoniocosta/unifi-openwrt/internal/device"
)

// Identity is internal only: neither configuration text nor its digest belongs
// in logs/reports. A changed body under the same controller version is new work.
func provisioningConfig(s device.State) (text, identity string, verified bool) {
	text, verified = s.CurrentControllerConfig()
	digest := sha256.Sum256([]byte(s.InformURL + "\x00" + s.Key + "\x00" + text))
	return text, s.CfgVersion + ":" + hex.EncodeToString(digest[:]), verified
}
