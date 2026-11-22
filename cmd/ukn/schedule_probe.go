// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

// WLAN commands in the compiler fixture are never applied. Binding probes use
// a constant-off schedule and retain the unused disabled AP prerequisites.
func probeScheduleResource(ctx context.Context, id, stateDir string, crash, bind bool) error {
	parent := "wifi0"
	if strings.HasPrefix(id, "WifiMaster1/") {
		parent = "wifi1"
	}
	cfg := "wireless.1.ssid=UKN-Test-Schedule-Probe\nwireless.1.parent=" + parent + "\nwireless.1.usage=user\naaa.1.ssid=UKN-Test-Schedule-Probe\naaa.1.security=open\nwireless.1.schedule_enabled=enabled\nwireless.1.schedule_invert=enabled\nwireless.1.schedule_mon.0=0800-1700\n"
	p, err := keenetic.BuildWLANTestPlan(cfg, map[string]string{"1": id})
	if err != nil {
		return err
	}
	if len(p.Schedules) != 1 {
		return fmt.Errorf("schedule probe did not compile one resource")
	}
	s := p.Schedules[0]
	if bind {
		s.Actions = []keenetic.ScheduleAction{{Days: []int{0, 1, 2, 3, 4, 5, 6}}}
	}
	read := func() (string, error) {
		b, err := keenetic.Local(ctx, "show running-config")
		return string(b), err
	}
	exists := func(text string) bool {
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimRight(line, "\r") == "schedule "+s.Name {
				return true
			}
		}
		return false
	}
	before, err := read()
	if err != nil {
		return err
	}
	if exists(before) {
		return fmt.Errorf("unbound schedule probe requires an absent resource name")
	}
	changes, err := keenetic.PrepareUnboundSchedule(s, before)
	if bind {
		changes, err = keenetic.PrepareStoppedScheduleBinding(s, before)
	}
	if err != nil {
		return err
	}
	verified := false
	journal := filepath.Join(stateDir, "rollback-probe.json")
	err = keenetic.ApplyChangesWatched(ctx, keenetic.Local, changes, journal, func(context.Context) error {
		after, err := read()
		if err != nil {
			return err
		}
		if err := keenetic.VerifyScheduleResource(s, after); err != nil {
			return err
		}
		if bind {
			if err := keenetic.VerifyScheduleBinding(s, after); err != nil {
				return err
			}
			status, err := keenetic.Local(ctx, "show interface "+id)
			if err != nil || keenetic.Values(string(status))["state"] != "down" {
				return fmt.Errorf("bound schedule probe did not remain disabled: %v", err)
			}
		}
		verified = true
		if crash {
			fmt.Println("Schedule resource watchdog armed; terminating before confirmation")
			os.Exit(77)
		}
		return errors.New("intentional schedule resource rollback")
	}, armWatchdog)
	if err == nil || !verified || !strings.Contains(err.Error(), "post-change health check failed") {
		return fmt.Errorf("unbound schedule resource probe failed: %v", err)
	}
	after, err := read()
	if err != nil {
		return err
	}
	if exists(after) {
		return fmt.Errorf("schedule resource remains after rollback")
	}
	if bind && probeInterfaceBlock(before, id) != probeInterfaceBlock(after, id) {
		return fmt.Errorf("schedule binding probe interface was not restored")
	}
	return nil
}

func probeInterfaceBlock(running, id string) string {
	var block []string
	inside := false
	for _, line := range strings.Split(running, "\n") {
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
	return strings.Join(block, "\n")
}
