// SPDX-License-Identifier: GPL-2.0-or-later
// ukn runs on Keenetic Entware and reuses the upstream UniFi inform session.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"github.com/antoniocosta/unifi-openwrt/internal/models"
)

func main() {
	informURL := flag.String("url", "", "UniFi controller inform URL")
	modelCode := flag.String("model", "U7PG2", "UniFi AP model code")
	listModels := flag.Bool("models", false, "list UniFi model catalogue and exit")
	sshListen := flag.String("ssh-listen", ":2222", "controller-only SSH adoption listener; empty disables")
	stateDir := flag.String("state", "/opt/var/lib/unifi-keenetic", "private persistent state directory")
	probe := flag.Bool("probe", false, "collect router status and exit without contacting a controller")
	terminalProbe := flag.Bool("terminal-probe", false, "test read-only NDM terminal execution and exit")
	rollbackProbe := flag.String("rollback-probe", "", "test rollback on an unused disabled AP slot, then exit")
	probeMode := flag.String("rollback-probe-mode", "ssid", "unused-slot rollback probe: ssid, owe, acl, wpa3, transition, rssi, schedule, schedule-binding, schedule-active or schedule-start")
	crashProbe := flag.Bool("crash-probe", false, "with -rollback-probe, exit before confirmation to test detached recovery")
	apply := flag.Bool("apply", false, "apply supported settings with rollback; requires explicit WLAN slots")
	ledControl := flag.Bool("led-control", false, "allow controller LED on/off commands independently of WLAN apply; no reboot or locate")
	reportUpgradeVersion := flag.Bool("report-upgrade-version", false, "acknowledge controller upgrades by reporting their version only; never download firmware or reboot (default: refuse)")
	wlanTestOnly := flag.Bool("wlan-test-only", false, "restrict planning to UKN-Test SSIDs on non-primary slots; exclude all radio changes")
	watchPath := flag.String("rollback-watch", "", "internal rollback watchdog journal")
	watchID := flag.String("rollback-id", "", "internal rollback journal identity")
	planFile := flag.String("plan", "", "print a redacted plan for a saved UniFi system_cfg and exit")
	slotsFile := flag.String("slots", "", "JSON mapping of UniFi WLAN indexes to Keenetic AP interface IDs")
	networksFile := flag.String("networks", "", "JSON mapping of VLAN IDs to existing non-home Keenetic bridges; not allowed in Wi-Fi test mode")
	flag.Parse()
	networks := map[string]string{}
	if *networksFile != "" {
		if *wlanTestOnly {
			log.Fatal("VLAN bridge mappings are not allowed in Wi-Fi test mode")
		}
		b, err := os.ReadFile(*networksFile)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(b, &networks); err != nil {
			log.Fatal(err)
		}
	}
	buildPlan := func(text string, slots map[string]string) (keenetic.Plan, error) {
		if *wlanTestOnly {
			return keenetic.BuildWLANTestPlan(text, slots)
		}
		return keenetic.BuildPlanWithNetworks(text, slots, networks)
	}
	if *listModels {
		for _, m := range models.All() {
			fmt.Println(m)
		}
		return
	}
	model, ok := models.Lookup(*modelCode)
	if !ok {
		log.Fatal("unknown UniFi model; use -models")
	}
	profile := models.Profile(model)
	if *watchPath != "" {
		if err := watchRollback(*watchPath, *watchID); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *planFile != "" {
		data, err := os.ReadFile(*planFile)
		if err != nil {
			log.Fatal(err)
		}
		slots := map[string]string{}
		if *slotsFile != "" {
			b, err := os.ReadFile(*slotsFile)
			if err != nil {
				log.Fatal(err)
			}
			if err := json.Unmarshal(b, &slots); err != nil {
				log.Fatal(err)
			}
		}
		plan, err := buildPlan(string(data), slots)
		if err != nil {
			log.Fatal(err)
		}
		for _, command := range plan.Commands {
			fmt.Println(command)
		}
		for _, unsupported := range plan.Unsupported {
			fmt.Printf("unsupported: %s\n", unsupported)
		}
		for _, acl := range plan.AccessLists {
			fmt.Printf("MAC ACL: %s policy=%s clients=%v (AP pauses during replacement)\n", acl.Target, acl.Policy, acl.MACs)
		}
		for _, schedule := range plan.Schedules {
			fmt.Printf("native schedule: %s for %s\n", schedule.Name, schedule.Target)
			for _, action := range schedule.Actions {
				fmt.Printf("  %s\n", action)
			}
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *rollbackProbe != "" {
		if err := os.MkdirAll(*stateDir, 0700); err != nil {
			log.Fatal(err)
		}
		c, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		if err := probeRollback(c, *rollbackProbe, *stateDir, *crashProbe, *probeMode); err != nil {
			log.Fatal(err)
		}
		fmt.Println("NDM rollback probe passed")
		return
	}
	if *terminalProbe {
		c, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		b, err := keenetic.Terminal(c, "show system")
		if err != nil {
			log.Fatal(err)
		}
		if keenetic.Values(string(b))["hostname"] == "" {
			log.Fatal("terminal did not return system status")
		}
		fmt.Println("NDM terminal probe passed")
		return
	}
	collect := func() (*keenetic.Snapshot, error) {
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return keenetic.Collect(c, keenetic.Local)
	}
	s, err := collect()
	if err != nil {
		log.Fatal(err)
	}
	if *probe {
		if err := json.NewEncoder(os.Stdout).Encode(s); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *informURL == "" {
		log.Fatal("-url is required")
	}
	if err := os.MkdirAll(*stateDir, 0700); err != nil {
		log.Fatal(err)
	}
	stateLock, err := keenetic.LockState(*stateDir)
	if err != nil {
		log.Fatal(err)
	}
	defer stateLock.Close()
	path := filepath.Join(*stateDir, "state.json")
	st, found, err := device.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	if !found {
		st = device.NewState(*informURL)
	}
	if !found || st.Version == "" {
		st.Version = profile.Version
	}
	mac, err := net.ParseMAC(s.Home["mac"])
	if err != nil {
		log.Fatal(err)
	}
	x := &device.Session{S: st, StateDir: *stateDir, Boot: time.Now(), Dev: inform.Device{MAC: mac, IP: s.Home["address"], Netmask: s.Home["mask"], Hostname: s.System["hostname"], Radios: []inform.Radio{}}}
	x.Dev.Model = profile.Model
	x.PhysicalDevice = true
	x.ReportUpgradeVersion = *reportUpgradeVersion
	if *reportUpgradeVersion {
		log.Print("controller firmware policy: report offered version only; no download or reboot")
	} else {
		log.Print("controller firmware policy: refuse upgrades")
	}
	x.Dev.ModelDisplay = profile.Display
	x.RestoreFromMgmtCfg()
	journal := filepath.Join(*stateDir, "rollback.json")
	recoverCtx, recoverCancel := context.WithTimeout(ctx, 30*time.Second)
	if err := keenetic.RecoverUndo(recoverCtx, journal, keenetic.Local); err != nil {
		recoverCancel()
		log.Fatal(err)
	}
	recoverCancel()
	slots := map[string]string{}
	if *slotsFile != "" {
		b, err := os.ReadFile(*slotsFile)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(b, &slots); err != nil {
			log.Fatal(err)
		}
	}
	lastPlanned := ""
	var requests chan device.AdoptRequest
	if *sshListen != "" {
		u, err := url.Parse(x.S.InformURL)
		if err != nil || net.ParseIP(u.Hostname()) == nil {
			log.Fatal("inform URL requires an IP literal")
		}
		_, port, err := net.SplitHostPort(*sshListen)
		if err != nil {
			log.Fatal(err)
		}
		x.SSHPort, err = strconv.Atoi(port)
		if err != nil {
			log.Fatal(err)
		}
		requests = make(chan device.AdoptRequest, 4)
		adopt := &device.SSHAdopt{Listen: *sshListen, AllowFrom: []string{u.Hostname()}, HostKey: filepath.Join(*stateDir, "ssh_host_ed25519.pem"), Requests: requests}
		if err := adopt.Start(); err != nil {
			log.Fatal(err)
		}
		defer adopt.Close()
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var actionHandler func(context.Context, string) error
	if *ledControl {
		actionHandler = func(ctx context.Context, action string) error {
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return keenetic.LEDAction(c, keenetic.Local, action)
		}
	}
	interval := 10 * time.Second
	for ctx.Err() == nil {
		s, err = collect()
		if err != nil {
			log.Printf("collection: %v", err)
		} else {
			x.Dev.IP = s.Home["address"]
			x.Dev.Radios = nil
			for i, r := range s.Radios {
				band := "ng"
				if i == 1 {
					band = "na"
				}
				x.Dev.Radios = append(x.Dev.Radios, inform.Radio{Name: fmt.Sprintf("wifi%d", i), Radio: band, Channel: r["channel"], HT: r["bandwidth"]})
			}
			x.Extra = func() map[string]any {
				cfg, _ := os.ReadFile(filepath.Join(*stateDir, "system_cfg.txt"))
				return s.FieldsForConfig(string(cfg))
			}
			if err := exchange(ctx, client, x, path, &interval, actionHandler); err != nil {
				log.Printf("inform: %v", err)
			}
			if x.S.CfgVersion != lastPlanned {
				plannedVersion := x.S.CfgVersion
				cfg, err := os.ReadFile(filepath.Join(*stateDir, "system_cfg.txt"))
				if err == nil {
					plan, err := buildPlan(string(cfg), slots)
					if err != nil {
						log.Printf("configuration plan: %v", err)
					} else {
						for _, command := range plan.Commands {
							log.Printf("plan: %s", command)
						}
						for _, setting := range plan.Unsupported {
							log.Printf("unsupported: %s", setting)
						}
						for _, acl := range plan.AccessLists {
							log.Printf("MAC ACL: %s policy=%s clients=%d", acl.Target, acl.Policy, len(acl.MACs))
						}
						applyEligible := *apply && x.S.Adopted
						if applyEligible && *wlanTestOnly {
							c, cancel := context.WithTimeout(ctx, 15*time.Second)
							running, err := keenetic.Local(c, "show running-config")
							cancel()
							if err == nil {
								err = keenetic.ValidateTestOwnership(string(running), slots)
							}
							if err != nil {
								log.Printf("test ownership check: %v", err)
								applyEligible = false
							}
						}
						if applyEligible {
							applyCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
							err := keenetic.ApplyPlanWatched(applyCtx, keenetic.Local, plan, journal, func(checkCtx context.Context) error {
								timer := time.NewTimer(5 * time.Second)
								defer timer.Stop()
								select {
								case <-checkCtx.Done():
									return checkCtx.Err()
								case <-timer.C:
								}
								_, err := keenetic.CollectVerifiedWLANs(checkCtx, keenetic.Local, plan)
								if err != nil {
									return err
								}
								running, err := keenetic.Local(checkCtx, "show running-config")
								if err != nil {
									return err
								}
								if err := keenetic.VerifyBridgeMembership(plan, string(running)); err != nil {
									return err
								}
								if err := keenetic.VerifyAccessLists(plan, string(running)); err != nil {
									return err
								}
								if err := keenetic.VerifyWLANSettings(plan, string(running)); err != nil {
									return err
								}
								if err := keenetic.VerifyCountryCodes(plan, string(running)); err != nil {
									return err
								}
								if err := keenetic.VerifyScheduleSettings(plan, string(running)); err != nil {
									return err
								}
								if err := exchange(checkCtx, client, x, path, &interval, actionHandler); err != nil {
									return err
								}
								if !x.S.Adopted {
									return fmt.Errorf("controller removed adoption during health check")
								}
								if x.S.CfgVersion != plannedVersion {
									return fmt.Errorf("controller configuration changed during provisioning")
								}
								return nil
							}, armWatchdog)
							cancel()
							if err != nil {
								log.Printf("apply: %v", err)
							} else {
								log.Print("configuration applied and confirmed")
							}
						}
						lastPlanned = plannedVersion
					}
				}
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		case request := <-requests:
			timer.Stop()
			outcome := x.ApplyAdopt(request)
			for _, event := range outcome.Events {
				log.Print(event)
			}
			if err := x.S.Save(path); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func exchange(ctx context.Context, c *http.Client, x *device.Session, path string, interval *time.Duration, actionHandler func(context.Context, string) error) error {
	plain, err := x.Payload(time.Now())
	if err != nil {
		return err
	}
	packet, err := inform.Encode(x.Dev.MAC, x.S.Key, x.Cipher(), plain, nil)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.S.InformURL, bytes.NewReader(packet))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-binary")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 && !x.S.Adopted {
		return nil
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, inform.MaxBodySize+1))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	if len(body) > inform.MaxBodySize {
		return fmt.Errorf("oversized reply")
	}
	_, plain, err = inform.Decode(body, x.S.Key)
	if err != nil {
		return err
	}
	outcome, err := x.Handle(plain, time.Now())
	if err != nil {
		return err
	}
	for _, e := range outcome.Events {
		log.Print(e)
	}
	for _, action := range outcome.Actions {
		if actionHandler == nil {
			log.Printf("controller action disabled on Keenetic: %s", action)
		} else if err := actionHandler(ctx, action); err != nil {
			log.Printf("controller action %s: %v", action, err)
		} else {
			log.Printf("controller action completed: %s", action)
		}
	}
	if outcome.Interval > 0 {
		*interval = outcome.Interval
	}
	if outcome.Immediate {
		*interval = time.Second
	}
	if outcome.Changed {
		return x.S.Save(path)
	}
	return nil
}
