// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"encoding/hex"
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
	// Only for inverse operations on a newly created native schedule.
	// Recovery may run even when its forward creation never happened.
	IfScheduleExists string `json:",omitempty"`
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
	// Known settings without native translation; values are never retained.
	Ignored     []string
	AccessLists []AccessList
	Schedules   []BroadcastSchedule
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

// BuildPlan translates AP radio/channel and supported personal/open WLAN settings.
// Slots are explicit: no existing interface is inferred from SSID or deleted.
// Unsupported settings prevent application of the entire plan.
func BuildPlan(text string, slots map[string]string) (Plan, error) {
	return BuildPlanWithNetworks(text, slots, nil)
}

// BuildPlanWithNetworks maps VLAN IDs to operator-provisioned native bridges.
// It never creates an uplink, DHCP service, bridge, or management network.
func BuildPlanWithNetworks(text string, slots, networks map[string]string) (Plan, error) {
	return BuildPlanWithOptions(text, slots, PlannerOptions{Networks: networks})
}

// PlannerOptions requires operator-owned native resources; none are inferred.
type PlannerOptions struct {
	Networks  map[string]string
	FTDomains map[string]string // SSID -> explicit two-alphanumeric-symbol native MDID.
}

func BuildPlanWithOptions(text string, slots map[string]string, options PlannerOptions) (Plan, error) {
	if err := validateFTDomains(options.FTDomains); err != nil {
		return Plan{}, err
	}
	text, err := normalizeScheduleWindows(text)
	if err != nil {
		return Plan{}, err
	}
	cfg := translate.ParseCfg(text)
	p := Plan{}
	bridges, unsupported, err := resolveNetworks(cfg, options.Networks)
	if err != nil {
		return p, err
	}
	p.Unsupported = append(p.Unsupported, unsupported...)
	thresholds, unsupported, err := minimumRSSI(cfg)
	if err != nil {
		return p, err
	}
	p.Unsupported = append(p.Unsupported, unsupported...)
	sort.Strings(p.Unsupported)
	add := func(s string) { p.Commands = append(p.Commands, Command{Text: s}) }
	owners := map[string]string{}
	for id, target := range slots {
		if !regexpAP.MatchString(target) {
			return p, fmt.Errorf("invalid mapped AP slot")
		}
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
		if err := validateEnableStatus(cfg, prefix+"status"); err != nil {
			return p, err
		}
		if country, raw := translate.CountryCode(cfg[prefix+"countrycode"], cfg["radio.countrycode"]); raw != "" {
			if country == "" {
				p.Unsupported = append(p.Unsupported, prefix+"countrycode")
			} else {
				// Set the regulatory domain before channel/width changes.
				add("interface " + master + " country-code " + country)
			}
		}
		if cfg[prefix+"txpower"] != "" {
			p.Ignored = append(p.Ignored, prefix+"txpower")
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
	var ftCommands []Command
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
		if err := validateEnableStatus(cfg, prefix+"status", "aaa."+id+".status"); err != nil {
			return p, err
		}
		for _, name := range []string{"dtim_period", "uapsd"} {
			if cfg[prefix+name] != "" {
				p.Ignored = append(p.Ignored, prefix+name)
			}
		}
		schedule, err := compileSchedule(cfg, prefix, target)
		if err != nil {
			return p, err
		}
		if schedule != nil {
			p.Schedules = append(p.Schedules, *schedule)
		}
		switch cfg[prefix+"wmm"] {
		case "": // Preserve native configuration when not requested.
		case "enabled":
			add("interface " + target + " wmm")
		case "disabled":
			add("interface " + target + " no wmm")
		default:
			return p, fmt.Errorf("invalid WLAN WMM status")
		}
		band := "ng"
		if parent == "WifiMaster1/" {
			band = "na"
		}
		if threshold := thresholds[band]; threshold != "" {
			add("interface " + target + " " + threshold)
		}
		acl, err := controllerACL(cfg, cfg[prefix+"devname"], target)
		if err != nil {
			return p, err
		}
		if acl != nil {
			p.AccessLists = append(p.AccessLists, *acl)
		}
		aaa := "aaa." + id + "."
		if err := validateEnableStatus(cfg, aaa+"wpa3.support", aaa+"wpa3.transition", prefix+"l2_isolation", aaa+"11k.status"); err != nil {
			return p, err
		}
		if err := validateBooleanStatus(cfg, prefix+"hide_ssid", aaa+"hide_ssid"); err != nil {
			return p, err
		}
		unsupportedCipher := false
		for key, value := range cfg {
			if strings.HasPrefix(key, aaa+"wpa.") && strings.HasSuffix(key, ".pairwise") && value != "" && value != "CCMP" {
				unsupportedCipher = true
			}
		}
		if cipher := cfg[aaa+"pmf.cipher"]; cipher != "" && cipher != "AES-128-CMAC" {
			unsupportedCipher = true
		}
		if unsupportedCipher {
			p.Unsupported = append(p.Unsupported, prefix+"security-cipher")
			continue
		}
		pskFile := cfg[aaa+"wpa.psk_file.status"]
		if err := validateEnableStatus(cfg, aaa+"wpa.psk_file.status"); err != nil {
			return p, err
		}
		multipleKeys := pskFile == "enabled"
		if pskFile == "" {
			for key, value := range cfg {
				if strings.HasPrefix(key, aaa+"wpa.psk_file.") && value != "" {
					multipleKeys = true
				}
			}
		}
		if multipleKeys || cfg[aaa+"wpa.psk_radius"] != "" || !singleSAEKey(cfg, aaa) {
			p.Unsupported = append(p.Unsupported, prefix+"private-psk")
			continue
		}
		mgmt := ""
		sae, unknownMgmt := false, false
		for name, value := range cfg {
			if !strings.HasPrefix(name, aaa+"wpa.key.") || !strings.HasSuffix(name, ".mgmt") {
				continue
			}
			mgmt += " " + value
			for _, method := range strings.Fields(value) {
				switch method {
				case "WPA-PSK":
				case "SAE":
					sae = true
				default:
					unknownMgmt = true
				}
			}
		}
		// An explicit security family cannot be overridden by stale WPA keys.
		switch cfg[aaa+"security"] {
		case "", "wpapsk":
		case "open", "none":
			if cfg[aaa+"wpa"] != "" || cfg[aaa+"wpa.psk"] != "" {
				p.Unsupported = append(p.Unsupported, prefix+"security")
				continue
			}
		default:
			p.Unsupported = append(p.Unsupported, prefix+"security")
			continue
		}
		open := cfg[aaa+"wpa"] == "" && cfg[aaa+"wpa.psk"] == "" && (cfg[aaa+"security"] == "open" || cfg[aaa+"security"] == "none")
		owe := open && cfg[aaa+"wpa3.support"] == "enabled"
		if unknownMgmt || cfg[aaa+"ssid"] != ssid || (!open && (cfg[aaa+"wpa"] != "2" || (cfg[aaa+"wpa3.support"] == "enabled" && !sae))) || (open && strings.TrimSpace(mgmt) != "") {
			p.Unsupported = append(p.Unsupported, prefix+"security")
			continue
		}
		key := cfg[aaa+"wpa.psk"]
		if !open {
			_, hexErr := hex.DecodeString(key)
			rawPSK := len(key) == 64 && hexErr == nil && !sae
			if !rawPSK && (len(key) < 8 || len(key) > 63) {
				return p, fmt.Errorf("invalid personal WLAN key length or format")
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
			add("interface " + target + " no encryption owe")
		} else {
			qssid, err := quoted(ssid)
			if err != nil {
				return p, err
			}
			if len(ssid) == 0 || len(ssid) > 32 {
				return p, fmt.Errorf("invalid SSID length")
			}
			add("interface " + target + " ssid " + qssid)
			p.Commands = append(p.Commands, Command{Text: "interface " + target + " no authentication wpa-psk", Secret: true})
			for _, mode := range []string{"wpa", "wpa2", "wpa3"} {
				add("interface " + target + " no encryption " + mode)
			}
			if owe {
				add("interface " + target + " encryption enable")
				add("interface " + target + " encryption owe")
			} else {
				add("interface " + target + " no encryption owe")
				add("interface " + target + " no encryption enable")
			}
		}
		hidden := cfg[prefix+"hide_ssid"] == "true" || cfg[aaa+"hide_ssid"] == "true"
		if hidden {
			add("interface " + target + " hide-ssid")
		} else {
			add("interface " + target + " no hide-ssid")
		}
		pmf, err := nativePMF(cfg[aaa+"pmf.status"], cfg[aaa+"pmf.mode"], sae, cfg[aaa+"wpa3.transition"] == "enabled", owe)
		if err != nil {
			return p, err
		}
		add("interface " + target + " " + pmf)
		bridge := bridges[id]
		if bridge == "" {
			bridge = "Bridge0"
		}
		add("interface " + bridge + " include " + target)
		isolation := "no peer-isolation"
		if cfg[prefix+"l2_isolation"] == "enabled" {
			isolation = "peer-isolation"
		}
		add("interface " + target + " " + isolation)
		// The documented per-AP rrm command supplies 802.11k; the separate
		// Wi-Fi-system BSS transition control is not an equivalent AP command.
		switch cfg[aaa+"bss_transition"] {
		case "", "disabled":
		case "enabled":
			p.Unsupported = append(p.Unsupported, prefix+"bss-transition")
		default:
			return p, fmt.Errorf("invalid BSS transition status")
		}
		rrm := "no rrm"
		if cfg[aaa+"11k.status"] == "enabled" {
			rrm = "rrm"
		}
		add("interface " + target + " " + rrm)
		switch cfg[aaa+"ft.status"] {
		case "":
		case "disabled":
			add("interface " + target + " no ft enable")
		case "enabled":
			domain := options.FTDomains[ssid]
			if domain == "" || !matchingFTPair(cfg, slots, bridges, ssid) {
				p.Unsupported = append(p.Unsupported, prefix+"fast-roaming")
			} else {
				for _, setting := range []string{"ft mdid " + domain, "no ft otd", "ft enable"} {
					ftCommands = append(ftCommands, Command{Text: "interface " + target + " " + setting})
				}
			}
		default:
			return p, fmt.Errorf("invalid fast-roaming status")
		}
		switch cfg[aaa+"wpa3.ft.status"] {
		case "", "disabled":
		case "enabled":
			p.Unsupported = append(p.Unsupported, prefix+"wpa3-fast-roaming")
		default:
			return p, fmt.Errorf("invalid WPA3 fast-roaming status")
		}
		action := "up"
		if cfg[prefix+"status"] == "disabled" || cfg[aaa+"status"] == "disabled" {
			action = "down"
		}
		add("interface " + target + " " + action)
	}
	// Configure both members before enabling their shared FT domain.
	p.Commands = append(p.Commands, ftCommands...)
	var missing []string
	for id, target := range slots {
		if cfg["wireless."+id+".ssid"] == "" {
			missing = append(missing, target)
		}
	}
	sort.Strings(missing)
	for _, target := range missing {
		add("interface " + target + " down")
	}
	return p, nil
}

// Missing status retains the controller format's existing defaults. Explicit
// values must be recognised so a typo cannot turn a disable request into up.
func validateEnableStatus(cfg translate.Cfg, keys ...string) error {
	for _, key := range keys {
		switch cfg[key] {
		case "", "enabled", "disabled":
		default:
			return fmt.Errorf("invalid enable status for %s", key)
		}
	}
	return nil
}

// Explicit disable wins over stale mode values for WPA2. SAE/OWE still retain
// the PMF required by their authentication modes.
func nativePMF(status, mode string, sae, transition, owe bool) (string, error) {
	switch status {
	case "", "enabled", "disabled":
	default:
		return "", fmt.Errorf("invalid PMF status")
	}
	switch mode {
	case "", "0", "1", "2":
	default:
		return "", fmt.Errorf("invalid PMF mode")
	}
	if owe || (sae && !transition) || (status == "enabled" && mode == "2") {
		return "pmf force", nil
	}
	if status == "enabled" || sae {
		return "pmf", nil
	}
	return "no pmf", nil
}

// Accept only the controller's redundant wildcard entry for the same global
// password. Per-station keys, additional passwords and VLAN/ID bindings need
// native mechanisms this backend does not implement.
func singleSAEKey(cfg translate.Cfg, aaa string) bool {
	prefix := aaa + "sae.psk."
	for key, value := range cfg {
		if !strings.HasPrefix(key, prefix) || value == "" {
			continue
		}
		switch strings.TrimPrefix(key, prefix) {
		case "1.psk":
			if cfg[aaa+"wpa.psk"] == "" || value != cfg[aaa+"wpa.psk"] {
				return false
			}
		case "1.mac":
			if !strings.EqualFold(value, "ff:ff:ff:ff:ff:ff") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validateBooleanStatus(cfg translate.Cfg, keys ...string) error {
	for _, key := range keys {
		switch cfg[key] {
		case "", "true", "false":
		default:
			return fmt.Errorf("invalid boolean status for %s", key)
		}
	}
	return nil
}
