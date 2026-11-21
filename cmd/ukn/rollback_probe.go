// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
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
	if mode != "ssid" && mode != "owe" {
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
	if mode == "owe" {
		for _, command := range []string{"encryption enable", "no encryption wpa", "no encryption wpa2", "no encryption wpa3", "encryption owe", "pmf force"} {
			plan.Commands = append(plan.Commands, keenetic.Command{Text: "interface " + id + " " + command})
		}
	}
	verified := false
	err = keenetic.ApplyPlanWatched(ctx, keenetic.Local, plan, journal, func(context.Context) error {
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
