// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProbeRollback uses an unused, administratively disabled AP slot. It never
// enables the interface or changes a serving WLAN's credentials.
func probeRollback(ctx context.Context, id, stateDir string, crash bool, mode string) error {
	if mode != "ft" && mode != "ssid" && mode != "owe" && mode != "acl" && mode != "wpa3" && mode != "transition" && mode != "rssi" && mode != "wmm" && mode != "schedule" && mode != "schedule-binding" && mode != "schedule-active" && mode != "schedule-start" {
		return fmt.Errorf("unknown rollback probe mode")
	}
	if !regexp.MustCompile(`^WifiMaster[01]/AccessPoint[1-6]$`).MatchString(id) {
		return fmt.Errorf("probe requires an explicit non-primary AP slot")
	}
	read := func() (map[string]string, error) {
		b, err := keenetic.Local(ctx, "show interface "+id)
		if err != nil {
			return nil, err
		}
		return keenetic.Values(string(b)), nil
	}
	before, err := read()
	if err != nil {
		return err
	}
	if before["id"] != id || before["state"] != "down" || before["ssid"] != "" {
		return fmt.Errorf("probe slot must be down with no configured SSID")
	}
	configuredSSID := func() (bool, error) {
		b, err := keenetic.Local(ctx, "show running-config")
		if err != nil {
			return false, err
		}
		inside := false
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, "interface ") {
				inside = line == "interface "+id
			}
			if line == "!" {
				inside = false
			}
			if inside && strings.HasPrefix(strings.TrimSpace(line), "ssid ") {
				return true, nil
			}
		}
		return false, nil
	}
	if present, err := configuredSSID(); err != nil {
		return err
	} else if present {
		return fmt.Errorf("probe slot already has an SSID in running configuration")
	}
	if mode == "schedule" || mode == "schedule-binding" {
		return probeScheduleResource(ctx, id, stateDir, crash, mode == "schedule-binding")
	}
	if mode == "schedule-active" || mode == "schedule-start" {
		if crash {
			return fmt.Errorf("active schedule probe does not support crash injection")
		}
		return probeActiveSchedule(ctx, id, stateDir, mode == "schedule-start")
	}
	journal := filepath.Join(stateDir, "rollback-probe.json")
	plan := keenetic.Plan{Commands: []keenetic.Command{{Text: "interface " + id + " ssid ukn-rollback-probe"}}}
	readBlock := func() (string, error) {
		b, err := keenetic.Local(ctx, "show running-config")
		if err != nil {
			return "", err
		}
		var block []string
		inside := false
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, "interface ") {
				inside = line == "interface "+id
			}
			if inside {
				block = append(block, line)
			}
			if line == "!" {
				inside = false
			}
		}
		if len(block) == 0 {
			return "", fmt.Errorf("probe interface missing from running configuration")
		}
		return strings.Join(block, "\n"), nil
	}
	beforeBlock, err := readBlock()
	if err != nil {
		return err
	}
	if mode == "ft" {
		plan = disabledFTProbePlan(id)
	}
	if mode == "owe" {
		for _, command := range []string{"encryption enable", "no encryption wpa", "no encryption wpa2", "no encryption wpa3", "encryption owe", "pmf force"} {
			plan.Commands = append(plan.Commands, keenetic.Command{Text: "interface " + id + " " + command})
		}
	}
	if mode == "acl" {
		plan.AccessLists = []keenetic.AccessList{{Target: id, Policy: "deny", MACs: []string{"02:00:00:00:00:fe"}}}
	}
	if mode == "rssi" {
		plan.Commands = append(plan.Commands, keenetic.Command{Text: "interface " + id + " rssi-threshold -75"})
	}
	if mode == "wmm" {
		plan.Commands = append(plan.Commands, keenetic.Command{Text: "interface " + id + " wmm"})
	}
	if mode == "wpa3" || mode == "transition" {
		var key [24]byte
		if _, err := rand.Read(key[:]); err != nil {
			return err
		}
		plan, err = securityProbePlan(id, mode, hex.EncodeToString(key[:]))
		if err != nil {
			return err
		}
	}
	verified := false
	err = keenetic.ApplyPlanWatched(ctx, keenetic.Local, plan, journal, func(context.Context) error {
		if mode == "rssi" || mode == "wmm" || mode == "ft" {
			block, err := readBlock()
			if err != nil {
				return err
			}
			if err := keenetic.VerifyWLANSettings(plan, block); err != nil {
				return err
			}
			status, err := read()
			if err != nil || status["state"] != "down" {
				return fmt.Errorf("WLAN setting probe did not remain disabled: %v", err)
			}
		}
		if mode == "wpa3" || mode == "transition" {
			block, err := readBlock()
			if err != nil {
				return err
			}
			status, err := read()
			if err != nil {
				return err
			}
			if err := verifySecurityProbe(mode, block, status["state"]); err != nil {
				return err
			}
			if err := keenetic.VerifyWLANSettings(plan, block); err != nil {
				return err
			}
		}
		if mode == "acl" {
			block, err := readBlock()
			if err != nil {
				return err
			}
			if err := keenetic.VerifyAccessLists(plan, block); err != nil {
				return err
			}
			status, err := read()
			if err != nil || status["state"] != "down" {
				return fmt.Errorf("ACL probe did not remain disabled: %v", err)
			}
		}
		if mode == "owe" {
			block, err := readBlock()
			if err != nil {
				return err
			}
			status, err := read()
			if err != nil {
				return err
			}
			if !strings.Contains(block, "\n    encryption owe\n") || !strings.Contains(block, "\n    pmf force\n") || status["state"] != "down" {
				return fmt.Errorf("OWE settings not present on disabled probe slot")
			}
		}
		verified = true
		if crash {
			fmt.Println("Watchdog armed; terminating probe before confirmation")
			os.Exit(77)
		}
		return errors.New("intentional rollback probe")
	}, armWatchdog)
	if err == nil || !strings.Contains(err.Error(), "post-change health check failed") {
		return fmt.Errorf("probe did not reach health-triggered rollback: %v", err)
	}
	if !verified {
		return fmt.Errorf("probe did not verify requested configuration")
	}
	after, err := read()
	if err != nil {
		return err
	}
	if after["ssid"] != "" || after["state"] != "down" {
		return fmt.Errorf("probe preimage was not restored")
	}
	if present, err := configuredSSID(); err != nil {
		return err
	} else if present {
		return fmt.Errorf("SSID remains in running configuration after rollback")
	}
	afterBlock, err := readBlock()
	if err != nil {
		return err
	}
	if afterBlock != beforeBlock {
		return fmt.Errorf("probe interface configuration differs after rollback")
	}
	return nil
}

// Use the real WLAN translator, but retain only commands on the disabled probe
// AP. Do not add bridge membership or enable any interface during a probe.
func securityProbePlan(id, mode, key string) (keenetic.Plan, error) {
	if mode != "wpa3" && mode != "transition" {
		return keenetic.Plan{}, fmt.Errorf("invalid personal-security probe mode")
	}
	parent := "wifi0"
	if strings.HasPrefix(id, "WifiMaster1/") {
		parent = "wifi1"
	}
	cfg := "wireless.1.ssid=UKN-Test-Security-Probe\nwireless.1.parent=" + parent + "\nwireless.1.usage=user\nwireless.1.status=disabled\naaa.1.ssid=UKN-Test-Security-Probe\naaa.1.wpa=2\naaa.1.wpa3.support=enabled\naaa.1.wpa.key.1.mgmt=SAE\naaa.1.wpa.psk=" + key + "\n"
	if mode == "transition" {
		cfg += "aaa.1.wpa3.transition=enabled\n"
	}
	p, err := keenetic.BuildWLANTestPlan(cfg, map[string]string{"1": id})
	if err != nil {
		return p, err
	}
	commands := p.Commands[:0]
	for _, c := range p.Commands {
		if strings.HasPrefix(c.Text, "interface "+id+" ") {
			if c.Text == "interface "+id+" up" {
				return keenetic.Plan{}, fmt.Errorf("security probe attempted to enable an AP")
			}
			commands = append(commands, c)
		}
	}
	p.Commands = commands
	return p, nil
}

func verifySecurityProbe(mode, block, state string) error {
	if mode != "wpa3" && mode != "transition" {
		return fmt.Errorf("invalid security verification mode")
	}
	if state != "down" || !strings.Contains(block, "\n    encryption enable\n") || !strings.Contains(block, "\n    encryption wpa3\n") || !strings.Contains(block, "\n    authentication wpa-psk ") || strings.Contains(block, "\n    encryption wpa\n") || strings.Contains(block, "\n    encryption owe\n") {
		return fmt.Errorf("personal-security settings missing or probe AP enabled")
	}
	wpa2 := strings.Contains(block, "\n    encryption wpa2\n")
	if mode == "wpa3" && (wpa2 || !strings.Contains(block, "\n    pmf force\n")) {
		return fmt.Errorf("WPA3-only probe did not require PMF and exclude WPA2")
	}
	if mode == "transition" && (!wpa2 || !strings.Contains(block, "\n    pmf\n")) {
		return fmt.Errorf("transition probe did not enable WPA2/WPA3 with optional PMF")
	}
	return nil
}

// Called only after probeRollback has established a non-primary, SSID-free,
// disabled slot. No up, bridge or shared-radio command belongs in this plan.
func disabledFTProbePlan(id string) keenetic.Plan {
	p := keenetic.Plan{}
	for _, setting := range []string{"ssid ukn-rollback-probe", "ft mdid AB", "no ft otd", "ft enable"} {
		p.Commands = append(p.Commands, keenetic.Command{Text: "interface " + id + " " + setting})
	}
	return p
}
