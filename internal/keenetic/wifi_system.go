// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type WiFiSystemBand struct {
	Band         int      `json:"band"`
	Enabled      *bool    `json:"enabled"`
	AccessPoints []string `json:"access_points"`
}

type WiFiSystemWLAN struct {
	ID                 string           `json:"id"`
	DisabledBySchedule *bool            `json:"disabled_by_schedule"`
	Bands              []WiFiSystemBand `json:"bands"`
}

// Inventory is evidence of membership, not authorization to mutate an AP or a
// claim that BSS transition is configured. Unknown aliases are counted only;
// no WPS, credentials or unrecognized status fields are retained.
type WiFiSystemInventory struct {
	SchemaVersion          int              `json:"schema_version"`
	WLANs                  []WiFiSystemWLAN `json:"wlans"`
	UnresolvedAccessPoints int              `json:"unresolved_access_points"`
}

var wifiSystemAP = regexp.MustCompile(`^WifiMaster[0-9]+/AccessPoint[0-9]+$`)
var wifiSystemHeader = regexp.MustCompile(`^(wlan, id|band, id|access-point, name) = (.*):$`)

func nativeLabel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		x, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid Wi-Fi-system identifier")
		}
		value = x
	}
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("invalid Wi-Fi-system identifier")
	}
	return value, nil
}

func wiFiSystemAliases(interfaces string) map[string][]string {
	aliases := map[string][]string{}
	add := func(name, id string) {
		for _, existing := range aliases[name] {
			if existing == id {
				return
			}
		}
		aliases[name] = append(aliases[name], id)
	}
	for _, block := range strings.Split(ansi.ReplaceAllString(interfaces, ""), "Interface, name = ")[1:] {
		id := Values(block)["id"]
		if !wifiSystemAP.MatchString(id) {
			continue
		}
		add(id, id)
		header := strings.TrimSpace(strings.SplitN(block, "\n", 2)[0])
		name, err := nativeLabel(strings.TrimSuffix(header, ":"))
		if err == nil {
			add(name, id)
		}
	}
	return aliases
}

func ParseWiFiSystem(wlans, interfaces string) (WiFiSystemInventory, error) {
	result := WiFiSystemInventory{SchemaVersion: 1, WLANs: []WiFiSystemWLAN{}}
	aliases := wiFiSystemAliases(interfaces)
	wlan, band := -1, -1
	inAP := false
	seen := map[string]bool{}
	for _, line := range strings.Split(ansi.ReplaceAllString(wlans, ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if h := wifiSystemHeader.FindStringSubmatch(line); h != nil {
			value, err := nativeLabel(h[2])
			if err != nil {
				return result, err
			}
			switch h[1] {
			case "wlan, id":
				if seen[value] {
					return result, fmt.Errorf("duplicate Wi-Fi-system WLAN")
				}
				seen[value] = true
				result.WLANs = append(result.WLANs, WiFiSystemWLAN{ID: value, Bands: []WiFiSystemBand{}})
				wlan = len(result.WLANs) - 1
				band = -1
				inAP = false
			case "band, id":
				if wlan < 0 {
					return result, fmt.Errorf("Wi-Fi-system band has no WLAN")
				}
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 || n > 2 {
					return result, fmt.Errorf("unknown Wi-Fi-system band")
				}
				for _, old := range result.WLANs[wlan].Bands {
					if old.Band == n {
						return result, fmt.Errorf("duplicate Wi-Fi-system band")
					}
				}
				result.WLANs[wlan].Bands = append(result.WLANs[wlan].Bands, WiFiSystemBand{Band: n, AccessPoints: []string{}})
				band = len(result.WLANs[wlan].Bands) - 1
				inAP = false
			case "access-point, name":
				if wlan < 0 || band < 0 {
					return result, fmt.Errorf("Wi-Fi-system AP has no band")
				}
				inAP = true
				ids := aliases[value]
				if len(ids) != 1 {
					result.UnresolvedAccessPoints++
					continue
				}
				row := &result.WLANs[wlan].Bands[band]
				for _, old := range row.AccessPoints {
					if old == ids[0] {
						return result, fmt.Errorf("duplicate Wi-Fi-system AP")
					}
				}
				row.AccessPoints = append(row.AccessPoints, ids[0])
			}
			continue
		}
		if strings.HasPrefix(line, "wlan,") || strings.HasPrefix(line, "band,") || strings.HasPrefix(line, "access-point,") {
			return result, fmt.Errorf("unrecognized Wi-Fi-system header")
		}
		if wlan < 0 {
			return result, fmt.Errorf("unrecognized Wi-Fi-system status")
		}
		if strings.HasPrefix(line, "disabled-by-schedule:") && band < 0 {
			v := strings.TrimSpace(strings.TrimPrefix(line, "disabled-by-schedule:"))
			if v != "yes" && v != "no" {
				return result, fmt.Errorf("invalid Wi-Fi-system schedule status")
			}
			if result.WLANs[wlan].DisabledBySchedule != nil {
				return result, fmt.Errorf("duplicate Wi-Fi-system schedule status")
			}
			x := v == "yes"
			result.WLANs[wlan].DisabledBySchedule = &x
		}
		// Band scalar precedes AP/WPS objects. Never flatten their enabled fields.
		if strings.HasPrefix(line, "enabled:") && band >= 0 && !inAP {
			v := strings.TrimSpace(strings.TrimPrefix(line, "enabled:"))
			if v != "yes" && v != "no" {
				return result, fmt.Errorf("invalid Wi-Fi-system band status")
			}
			row := &result.WLANs[wlan].Bands[band]
			if row.Enabled != nil {
				return result, fmt.Errorf("duplicate Wi-Fi-system band status")
			}
			x := v == "yes"
			row.Enabled = &x
		}
		if !strings.Contains(line, ":") {
			return result, fmt.Errorf("unrecognized Wi-Fi-system status line")
		}
	}
	for i := range result.WLANs {
		for j := range result.WLANs[i].Bands {
			sort.Strings(result.WLANs[i].Bands[j].AccessPoints)
		}
		sort.Slice(result.WLANs[i].Bands, func(a, b int) bool { return result.WLANs[i].Bands[a].Band < result.WLANs[i].Bands[b].Band })
	}
	sort.Slice(result.WLANs, func(a, b int) bool { return result.WLANs[a].ID < result.WLANs[b].ID })
	return result, nil
}

func CollectWiFiSystem(ctx context.Context, run Runner) (WiFiSystemInventory, error) {
	wlans, err := run(ctx, "show mws wlan")
	if err != nil {
		return WiFiSystemInventory{}, err
	}
	interfaces, err := run(ctx, "show interface")
	if err != nil {
		return WiFiSystemInventory{}, err
	}
	return ParseWiFiSystem(string(wlans), string(interfaces))
}
