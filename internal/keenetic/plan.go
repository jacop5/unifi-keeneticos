// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/translate"
	"sort"
	"strconv"
	"strings"
)

// Command keeps credential-bearing NDM commands out of diagnostic output.
type Command struct {
	Text   string
	Secret bool
}

func (c Command) String() string {
	if c.Secret {
		return "<WLAN credential command>"
	}
	return c.Text
}

type Plan struct {
	Commands    []Command
	Unsupported []string
}

// quoted uses NDM's documented double-quote syntax. NDM does not document
// escapes inside quoted strings, so reject control characters and quotes.
func quoted(s string) (string, error) {
	for _, r := range s {
		if r < 32 || r == 127 || r == '"' || r == '\\' {
			return "", fmt.Errorf("value contains an unsupported NDM character")
		}
	}
	return `"` + s + `"`, nil
}

// BuildPlan translates AP radio/channel and WPA2 personal WLAN settings.
// Slots are explicit: no existing interface is inferred from SSID or deleted.
// Unsupported settings prevent application of the entire plan.
func BuildPlan(text string, slots map[string]string) (Plan, error) {
	cfg := translate.ParseCfg(text)
	p := Plan{}
	for key, value := range cfg {
		if strings.HasPrefix(key, "vlan.") && strings.HasSuffix(key, ".id") && value != "" && value != "0" {
			p.Unsupported = append(p.Unsupported, key)
		}
	}
	sort.Strings(p.Unsupported)
	add := func(s string) { p.Commands = append(p.Commands, Command{Text: s}) }
	owners := map[string]string{}
	for id, target := range slots {
		if prior, exists := owners[target]; exists {
			return p, fmt.Errorf("WLANs %s and %s share a slot", prior, id)
		}
		owners[target] = id
	}
	for _, idx := range []string{"1", "2"} {
		prefix := "radio." + idx + "."
		phy := cfg[prefix+"phyname"]
		if phy == "" {
			continue
		}
		master := ""
		switch phy {
		case "wifi0":
			master = "WifiMaster0"
		case "wifi1":
			master = "WifiMaster1"
		default:
			return p, fmt.Errorf("unknown radio %q", phy)
		}
		if status := cfg[prefix+"status"]; status != "" {
			action := "up"
			if status == "disabled" {
				action = "down"
			}
			add("interface " + master + " " + action)
		}
		if channel := cfg[prefix+"channel"]; channel != "" {
			if channel == "auto" {
				add("interface " + master + " no channel")
			} else {
				n, err := strconv.Atoi(channel)
				if err != nil || n < 0 || n > 196 {
					return p, fmt.Errorf("invalid channel")
				}
				add("interface " + master + " channel " + channel)
			}
		}
		mode := cfg[prefix+"ieee_mode"]
		if mode != "" {
			matched := false
			for _, width := range []string{"160", "80", "40", "20"} {
				if strings.HasSuffix(mode, "ht"+width) {
					ndmWidth := width
					if width == "40" {
						ndmWidth = "40-above"
						channel, _ := strconv.Atoi(cfg[prefix+"channel"])
						if (master == "WifiMaster0" && channel > 7) || (master == "WifiMaster1" && channel > 0 && channel%8 == 0) {
							ndmWidth = "40-below"
						}
					}
					if width == "80" {
						ndmWidth = "40-above/80"
					}
					add("interface " + master + " channel width " + ndmWidth)
					matched = true
					break
				}
			}
			if !matched {
				p.Unsupported = append(p.Unsupported, prefix+"ieee_mode")
			}
		}
	}
	var ids []string
	for key := range cfg {
		if strings.HasPrefix(key, "wireless.") && strings.HasSuffix(key, ".ssid") {
			ids = append(ids, strings.TrimSuffix(strings.TrimPrefix(key, "wireless."), ".ssid"))
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		prefix := "wireless." + id + "."
		ssid := cfg[prefix+"ssid"]
		if cfg[prefix+"usage"] != "user" {
			p.Unsupported = append(p.Unsupported, prefix+"usage")
			continue
		}
		target := slots[id]
		if target == "" {
			p.Unsupported = append(p.Unsupported, prefix+"slot")
			continue
		}
		parent := "WifiMaster0/"
		if cfg[prefix+"parent"] == "wifi1" {
			parent = "WifiMaster1/"
		} else if cfg[prefix+"parent"] != "wifi0" {
			return p, fmt.Errorf("unknown WLAN radio")
		}
		valid := false
		for n := 0; n < 8; n++ {
			if target == parent+"AccessPoint"+strconv.Itoa(n) {
				valid = true
			}
		}
		if !valid {
			return p, fmt.Errorf("invalid slot for WLAN %s", id)
		}
		aaa := "aaa." + id + "."
		mgmt := ""
		for i := 1; i <= 4; i++ {
			mgmt += " " + cfg[fmt.Sprintf("%swpa.key.%d.mgmt", aaa, i)]
		}
		sae := strings.Contains(mgmt, "SAE")
		if cfg[aaa+"ssid"] != ssid || cfg[aaa+"wpa"] != "2" || strings.Contains(mgmt, "EAP") || (cfg[aaa+"wpa3.support"] == "enabled" && !sae) {
			p.Unsupported = append(p.Unsupported, prefix+"security")
			continue
		}
		key := cfg[aaa+"wpa.psk"]
		if len(key) < 8 || len(key) > 63 {
			return p, fmt.Errorf("invalid WPA2 passphrase length")
		}
		qssid, err := quoted(ssid)
		if err != nil {
			return p, err
		}
		if len(ssid) == 0 || len(ssid) > 32 {
			return p, fmt.Errorf("invalid SSID length")
		}
		qkey, err := quoted(key)
		if err != nil {
			return p, err
		}
		add("interface " + target + " ssid " + qssid)
		p.Commands = append(p.Commands, Command{Text: "interface " + target + " authentication wpa-psk " + qkey, Secret: true})
		add("interface " + target + " encryption enable")
		add("interface " + target + " no encryption wpa")
		if sae {
			add("interface " + target + " encryption wpa3")
		} else {
			add("interface " + target + " no encryption wpa3")
		}
		if !sae || cfg[aaa+"wpa3.transition"] == "enabled" {
			add("interface " + target + " encryption wpa2")
		} else {
			add("interface " + target + " no encryption wpa2")
		}
		hidden := cfg[prefix+"hide_ssid"] == "true" || cfg[aaa+"hide_ssid"] == "true"
		if hidden {
			add("interface " + target + " hide-ssid")
		} else {
			add("interface " + target + " no hide-ssid")
		}
		pmf := "no pmf"
		if cfg[aaa+"pmf.status"] == "enabled" || sae {
			pmf = "pmf"
		}
		if cfg[aaa+"pmf.mode"] == "2" || (sae && cfg[aaa+"wpa3.transition"] != "enabled") {
			pmf = "pmf force"
		}
		add("interface " + target + " " + pmf)
		add("interface Bridge0 include " + target)
		isolation := "no peer-isolation"
		if cfg[prefix+"l2_isolation"] == "enabled" {
			isolation = "peer-isolation"
		}
		add("interface " + target + " " + isolation)
		rrm := "no rrm"
		if cfg[aaa+"11k.status"] == "enabled" {
			rrm = "rrm"
		}
		add("interface " + target + " " + rrm)
		if cfg[aaa+"ft.status"] == "enabled" {
			p.Unsupported = append(p.Unsupported, prefix+"fast-roaming")
		}
		action := "up"
		if cfg[prefix+"status"] == "disabled" || cfg[aaa+"status"] == "disabled" {
			action = "down"
		}
		add("interface " + target + " " + action)
	}
	return p, nil
}
