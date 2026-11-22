// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

// probeActiveSchedule briefly enables only an unused, unbridged test AP with a
// random WPA2 password. The native engine must change state at the next minute;
// transaction rollback then restores its exact preimage. It never joins LAN.
func probeActiveSchedule(ctx context.Context, id, stateDir string, start bool) error {
	read := func(command string) (string, error) {
		b, err := keenetic.Local(ctx, command)
		return string(b), err
	}
	clock, err := read("show clock date")
	if err != nil {
		return err
	}
	civil, err := keenetic.RouterCivilTime(clock)
	if err != nil {
		return err
	}
	if civil.Second() < 20 || civil.Second() > 40 {
		return fmt.Errorf("retry active schedule probe when native clock seconds are 20 through 40")
	}
	before, err := read("show running-config")
	if err != nil {
		return err
	}
	// No existing binding or bridge membership may be inherited by this AP.
	block := probeInterfaceBlock(before, id)
	if block == "" || strings.Contains(block, "\n    schedule ") || strings.Contains(block, "\n    rename ") {
		return fmt.Errorf("active schedule probe requires an unaliased, unbound test slot")
	}
	for _, line := range strings.Split(before, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "include" && strings.Trim(fields[1], `"`) == id {
			return fmt.Errorf("active schedule probe slot already belongs to a bridge")
		}
	}
	name := "UKN-" + strings.ReplaceAll(id, "/", "-")
	if strings.Contains("\n"+before, "\nschedule "+name+"\n") {
		return fmt.Errorf("active schedule probe requires a new resource")
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	parent := "wifi0"
	if strings.HasPrefix(id, "WifiMaster1/") {
		parent = "wifi1"
	}
	cfg := "wireless.1.ssid=UKN-Test-Timed-Probe\nwireless.1.parent=" + parent + "\nwireless.1.usage=user\nwireless.1.status=enabled\naaa.1.ssid=UKN-Test-Timed-Probe\naaa.1.wpa=2\naaa.1.wpa.psk=" + hex.EncodeToString(random[:]) + "\n"
	p, err := keenetic.BuildWLANTestPlan(cfg, map[string]string{"1": id})
	if err != nil {
		return err
	}
	var commands []keenetic.Command
	for _, command := range p.Commands {
		if strings.HasPrefix(command.Text, "interface "+id+" ") {
			commands = append(commands, command)
		}
	}
	p.Commands = commands
	action := func(at time.Time, start bool) keenetic.ScheduleAction {
		return keenetic.ScheduleAction{Start: start, Hour: at.Hour(), Minute: at.Minute(), Days: []int{int(at.Weekday())}}
	}
	s := keenetic.BroadcastSchedule{Name: name, Target: id,
		Actions: []keenetic.ScheduleAction{action(civil.Add(-time.Minute), !start), action(civil.Add(time.Minute), start)}}
	p.Schedules = []keenetic.BroadcastSchedule{s}
	verified := false
	err = keenetic.ApplyPlanWatched(ctx, keenetic.Local, p, filepath.Join(stateDir, "rollback-probe.json"), func(ctx context.Context) error {
		running, err := read("show running-config")
		if err != nil {
			return err
		}
		if err := keenetic.VerifyWLANSettings(p, running); err != nil {
			return err
		}
		if err := keenetic.VerifyScheduleBinding(s, running); err != nil {
			return err
		}
		if err := keenetic.VerifyScheduleSettings(p, running); err != nil {
			return err
		}
		if _, err := keenetic.CollectVerifiedWLANs(ctx, keenetic.Local, p); err != nil {
			return err
		}
		initial, final := "up", "down"
		if start {
			initial, final = "down", "up"
		}
		status, err := read("show interface " + id)
		if err != nil || keenetic.Values(status)["state"] != initial {
			return fmt.Errorf("secured timed test AP did not reach initial state %s: %v", initial, err)
		}
		fmt.Printf("Secured isolated test AP %s; waiting for native scheduled %s\n", initial, final)
		deadline := time.NewTimer(45 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return fmt.Errorf("native scheduled transition was not observed")
			case <-tick.C:
				status, err := read("show interface " + id)
				if err != nil {
					return err
				}
				if keenetic.Values(status)["state"] == final {
					clock, err := read("show clock date")
					if err != nil {
						return err
					}
					now, err := keenetic.RouterCivilTime(clock)
					if err != nil || s.ActiveAt(now) != start {
						return fmt.Errorf("AP changed state before native schedule boundary: %v", err)
					}
					if _, err := keenetic.CollectVerifiedWLANs(ctx, keenetic.Local, p); err != nil {
						return err
					}
					verified = true
					return errors.New("intentional active schedule rollback")
				}
			}
		}
	}, armWatchdog)
	if err == nil || !verified || !strings.Contains(err.Error(), "post-change health check failed") {
		return fmt.Errorf("active schedule probe failed: %v", err)
	}
	after, err := read("show running-config")
	if err != nil {
		return err
	}
	if probeInterfaceBlock(after, id) != block || strings.Contains(after, "\nschedule "+s.Name+"\n") {
		return fmt.Errorf("active schedule probe did not restore its preimage")
	}
	return nil
}
