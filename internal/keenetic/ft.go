// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

var nativeFTDomain = regexp.MustCompile(`^[A-Za-z0-9]{2}$`)

func validateFTDomains(domains map[string]string) error {
	owners := map[string]string{}
	for ssid, domain := range domains {
		if len(ssid) == 0 || len(ssid) > 32 || !nativeFTDomain.MatchString(domain) {
			return fmt.Errorf("invalid explicit FT domain mapping")
		}
		if _, err := quoted(ssid); err != nil {
			return fmt.Errorf("invalid FT mapping SSID")
		}
		if prior, ok := owners[domain]; ok && prior != ssid {
			return fmt.Errorf("FT domains must be unique per SSID")
		}
		owners[domain] = ssid
	}
	return nil
}

// matchingFTPair deliberately supports only matched dual-band WPA2 personal.
// It does not infer a UniFi MDID or claim roaming with native UniFi APs.
func matchingFTPair(cfg translate.Cfg, slots, bridges map[string]string, ssid string) bool {
	bands := map[string]bool{}
	key, bridge, pmf := "", "", ""
	count := 0
	for name, value := range cfg {
		if !strings.HasPrefix(name, "wireless.") || !strings.HasSuffix(name, ".ssid") || value != ssid {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "wireless."), ".ssid")
		w, a := "wireless."+id+".", "aaa."+id+"."
		band := cfg[w+"parent"]
		if slots[id] == "" || (band != "wifi0" && band != "wifi1") || bands[band] || cfg[a+"ft.status"] != "enabled" || cfg[w+"status"] == "disabled" || cfg[a+"status"] == "disabled" || cfg[w+"schedule_enabled"] == "enabled" {
			return false
		}
		if cfg[a+"wpa"] != "2" || cfg[a+"wpa.psk"] == "" || cfg[a+"wpa3.support"] == "enabled" {
			return false
		}
		if family := cfg[a+"security"]; family != "" && family != "wpapsk" {
			return false
		}
		for field, method := range cfg {
			if strings.HasPrefix(field, a+"wpa.key.") && strings.HasSuffix(field, ".mgmt") {
				for _, token := range strings.Fields(method) {
					if token != "WPA-PSK" {
						return false
					}
				}
			}
		}
		nativePMFMode, err := nativePMF(cfg[a+"pmf.status"], cfg[a+"pmf.mode"], false, false, false)
		if err != nil {
			return false
		}
		nativeBridge := bridges[id]
		if nativeBridge == "" {
			nativeBridge = "Bridge0"
		}
		if count > 0 && (key != cfg[a+"wpa.psk"] || bridge != nativeBridge || pmf != nativePMFMode) {
			return false
		}
		key, bridge, pmf = cfg[a+"wpa.psk"], nativeBridge, nativePMFMode
		bands[band] = true
		count++
	}
	return count == 2 && bands["wifi0"] && bands["wifi1"]
}
