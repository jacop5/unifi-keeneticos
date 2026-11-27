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
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
	"github.com/antoniocosta/unifi-openwrt/internal/installation"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"github.com/antoniocosta/unifi-openwrt/internal/models"
)

func main() {
	informURL := flag.String("url", "", "UniFi controller inform URL")
	modelCode := flag.String("model", "U7PG2", "UniFi AP model code")
	listModels := flag.Bool("models", false, "list UniFi model catalogue and exit")
	sshListen := flag.String("ssh-listen", ":2222", "controller-only SSH adoption listener; empty disables")
	debugAdoption := flag.Bool("debug-adoption", false, "log only the numeric HTTP status of inform replies when it changes (never URLs, headers, bodies, errors or keys)")
	stateDir := flag.String("state", "/opt/var/lib/unifi-keenetic", "private persistent state directory")
	probe := flag.Bool("probe", false, "collect router status and exit without contacting a controller")
	wifiSystemStatus := flag.Bool("wifi-system-status", false, "read native Wi-Fi-system WLAN/AP membership; no controller or configuration changes")
	installationStatus := flag.String("installation-status", "", "inspect retained installer data under this Entware root (normally /opt); read-only, no recovery")
	terminalProbe := flag.Bool("terminal-probe", false, "test read-only NDM terminal execution and exit")
	rollbackProbe := flag.String("rollback-probe", "", "test rollback on an unused disabled AP slot, then exit")
	probeMode := flag.String("rollback-probe-mode", "ssid", "unused-slot rollback probe: ssid, owe, acl, blocked-acl, wpa3, transition, rssi, wmm, ft, schedule, schedule-binding, schedule-active or schedule-start")
	crashProbe := flag.Bool("crash-probe", false, "with -rollback-probe, exit before confirmation to test detached recovery")
	apply := flag.Bool("apply", false, "apply supported settings with rollback; requires explicit WLAN slots")
	rebootControl := flag.Bool("reboot-control", false, "allow controller-requested physical reboot; unavailable in WLAN test mode")
	ledControl := flag.Bool("led-control", false, "allow controller LED on/off commands independently of WLAN apply; no reboot or locate")
	clientBlockControl := flag.Bool("client-block-control", false, "compose controller block list into explicitly controller-owned WLAN ACLs; requires slots; native enforcement requires -apply")
	reportUpgradeVersion := flag.Bool("report-upgrade-version", false, "acknowledge controller upgrades by reporting their version only; never download firmware or reboot (default: refuse)")
	wlanTestOnly := flag.Bool("wlan-test-only", false, "restrict planning to UKN-Test SSIDs on non-primary slots; exclude all radio changes")
	wlanOnly := flag.Bool("wlan-only", false, "restrict provisioning to explicitly mapped WLAN slots with arbitrary SSIDs; exclude all radio-wide changes; requires -slots")
	watchPath := flag.String("rollback-watch", "", "internal rollback watchdog journal")
	watchID := flag.String("rollback-id", "", "internal rollback journal identity")
	planFile := flag.String("plan", "", "print a redacted plan for a saved UniFi system_cfg and exit")
	planBlocked := flag.String("plan-blocked", "", "with -plan, compose a JSON array of blocked client MACs; read-only, never loads adoption state")
	slotsFile := flag.String("slots", "", "JSON mapping of UniFi WLAN indexes to Keenetic AP interface IDs")
	networksFile := flag.String("networks", "", "JSON mapping of VLAN IDs to existing non-home Keenetic bridges; not allowed in Wi-Fi test mode")
	ftDomainsFile := flag.String("experimental-ft-domains", "", "JSON SSID-to-native-MDID map for experimental matched dual-band WPA2 FT; no UniFi interoperability claim")
	preflightRunning := flag.String("preflight-running", "", "with -plan, validate rollback preparation against a saved native running-config; no router access")
	showProvisionStatus := flag.Bool("provision-status", false, "read local historical provisioning status and exit; no router or controller access")
	showBuild := flag.Bool("build-info", false, "print local executable build metadata as JSON and exit; no router or controller access")
	experimentalSAE := flag.Bool("experimental-sae", false, "advertise only the controller SAE radio capability for WPA3 testing; client interoperability unverified")
	jsonPlan := flag.Bool("plan-json", false, "with -plan, print a redacted structured plan without router access")
	flag.Parse()
	if *clientBlockControl && (*slotsFile == "" || *planFile != "") {
		log.Fatal("-client-block-control requires -slots and cannot be used with offline -plan")
	}
	if *rebootControl && *wlanTestOnly {
		log.Fatal("-reboot-control cannot be used in WLAN test mode")
	}
	if *wlanOnly && *wlanTestOnly {
		log.Fatal("-wlan-only cannot be used with -wlan-test-only")
	}
	if *wlanOnly && *slotsFile == "" {
		log.Fatal("-wlan-only requires -slots")
	}
	offlineModes := map[string]bool{
		"rollback-watch": *watchPath != "", "rollback-probe": *rollbackProbe != "",
		"crash-probe": *crashProbe, "apply": *apply, "led-control": *ledControl, "reboot-control": *rebootControl,
		"probe": *probe, "terminal-probe": *terminalProbe, "models": *listModels, "build-info": *showBuild, "provision-status": *showProvisionStatus,
		"wifi-system-status":   *wifiSystemStatus,
		"client-block-control": *clientBlockControl,
		"installation-status":  *installationStatus != "",
	}
	if err := validateOfflineMode(*preflightRunning, *planFile, offlineModes); err != nil {
		log.Fatal(err)
	}
	if *planBlocked != "" {
		if err := validateOfflineMode("requested", *planFile, offlineModes, "-plan-blocked"); err != nil {
			log.Fatal(err)
		}
	}
	if *jsonPlan {
		if err := validateOfflineMode("requested", *planFile, offlineModes, "-plan-json"); err != nil {
			log.Fatal(err)
		}
	}
	if *installationStatus != "" {
		if *planFile != "" || *jsonPlan || *preflightRunning != "" {
			log.Fatal("-installation-status cannot be combined with planning")
		}
		for name, enabled := range offlineModes {
			if name != "installation-status" && enabled {
				log.Fatalf("-installation-status conflicts with -%s", name)
			}
		}
		report, err := installation.Inspect(*installationStatus)
		if err != nil {
			log.Fatal("cannot inspect installation root")
		}
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *showProvisionStatus {
		if *planFile != "" || *jsonPlan || *preflightRunning != "" {
			log.Fatal("-provision-status cannot be combined with planning")
		}
		for name, enabled := range offlineModes {
			if name != "provision-status" && enabled {
				log.Fatalf("-provision-status conflicts with -%s", name)
			}
		}
		report, err := readProvisionStatus(filepath.Join(*stateDir, "provision-status.json"))
		if err != nil {
			log.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *wifiSystemStatus {
		if *planFile != "" {
			log.Fatal("-wifi-system-status cannot be combined with planning")
		}
		for name, enabled := range offlineModes {
			if name != "wifi-system-status" && enabled {
				log.Fatalf("-wifi-system-status conflicts with -%s", name)
			}
		}
		c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		inventory, err := keenetic.CollectWiFiSystem(c, keenetic.Local)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(inventory); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *showBuild {
		info, _ := debug.ReadBuildInfo()
		if err := json.NewEncoder(os.Stdout).Encode(buildInformation(info)); err != nil {
			log.Fatal(err)
		}
		return
	}
	networks := map[string]string{}
	if *networksFile != "" {
		if *wlanTestOnly {
			log.Fatal("VLAN bridge mappings are not allowed in Wi-Fi test mode")
		}
		m, err := loadMappingFile(*networksFile, "-networks")
		if err != nil {
			log.Fatal(err)
		}
		networks = m
	}
	ftDomains := map[string]string{}
	if *ftDomainsFile != "" {
		m, err := loadMappingFile(*ftDomainsFile, "-experimental-ft-domains")
		if err != nil {
			log.Fatal(err)
		}
		ftDomains = m
	}
	buildPlan := func(text string, slots map[string]string) (keenetic.Plan, error) {
		if *wlanTestOnly {
			return keenetic.BuildWLANTestPlanWithFT(text, slots, ftDomains)
		}
		if *wlanOnly {
			return keenetic.BuildWLANPlanWithOptions(text, slots, keenetic.PlannerOptions{Networks: networks, FTDomains: ftDomains})
		}
		return keenetic.BuildPlanWithOptions(text, slots, keenetic.PlannerOptions{Networks: networks, FTDomains: ftDomains})
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
			m, err := loadMappingFile(*slotsFile, "-slots")
			if err != nil {
				log.Fatal(err)
			}
			slots = m
		}
		plan, err := buildPlan(string(data), slots)
		if err != nil {
			log.Fatal(err)
		}
		if *planBlocked != "" {
			f, err := os.Open(*planBlocked)
			if err != nil {
				log.Fatal("cannot open offline block-list input")
			}
			b, err := io.ReadAll(io.LimitReader(f, 65537))
			_ = f.Close()
			var blocked []string
			trimmed := bytes.TrimSpace(b)
			if err != nil || len(b) > 65536 || len(trimmed) == 0 || trimmed[0] != '[' || json.Unmarshal(b, &blocked) != nil {
				log.Fatal("offline block-list input must be a JSON array of MAC strings under 64 KiB")
			}
			plan, err = keenetic.ComposeBlockedPlan(plan, blocked)
			if err != nil {
				log.Fatal(err)
			}
		}
		var preflightChanges *int
		if *preflightRunning != "" {
			running, err := os.ReadFile(*preflightRunning)
			if err != nil {
				log.Fatal(err)
			}
			changes, err := preflightPlan(plan, string(running), slots, *wlanTestOnly)
			if err != nil {
				log.Fatal(err)
			}
			preflightChanges = &changes
			if !*jsonPlan {
				fmt.Printf("offline preflight passed: %d reversible changes; no router commands executed\n", changes)
			}
		}
		if *jsonPlan {
			if err := json.NewEncoder(os.Stdout).Encode(redactedPlan(plan, preflightChanges)); err != nil {
				log.Fatal(err)
			}
			return
		}
		for _, command := range plan.Commands {
			fmt.Println(command)
		}
		for _, unsupported := range plan.Unsupported {
			fmt.Printf("unsupported: %s\n", unsupported)
		}
		for _, setting := range plan.Ignored {
			fmt.Printf("ignored (no native translation): %s\n", setting)
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
	journal := filepath.Join(*stateDir, "rollback.json")
	if !*probe {
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
		// Pending recovery must precede telemetry and adoption-state loading:
		// failures there must not strand the router in an unconfirmed state.
		recoverCtx, recoverCancel := context.WithTimeout(ctx, 30*time.Second)
		err = keenetic.RecoverUndo(recoverCtx, journal, keenetic.Local)
		recoverCancel()
		if err != nil {
			log.Fatal(err)
		}
	}
	collect := func() (*keenetic.Snapshot, error) {
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		snapshot, err := keenetic.Collect(c, keenetic.Local)
		if snapshot != nil {
			snapshot.ExperimentalSAE = *experimentalSAE
		}
		return snapshot, err
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
	statusPath := filepath.Join(*stateDir, "provision-status.json")
	previousStatus, statusErr := readProvisionStatus(statusPath)
	if statusErr != nil {
		log.Printf("provision status: %v", statusErr)
	}
	reporter := provisionReporter{path: statusPath, report: previousStatus, apply: *apply, testOnly: *wlanTestOnly, wlanOnly: *wlanOnly, clientBlockControl: *clientBlockControl}
	recordProvision := func(version, phase string, plan *keenetic.Plan, retry time.Time) {
		if err := reporter.record(version, phase, plan, retry); err != nil {
			log.Printf("provision status: %v", err)
		}
	}
	recordProvision(st.CfgVersion, "starting", nil, time.Time{})
	mac, err := net.ParseMAC(s.Home["mac"])
	if err != nil {
		log.Fatal(err)
	}
	x := &device.Session{S: st, StateDir: *stateDir, Boot: time.Now(), Dev: inform.Device{MAC: mac, IP: s.Home["address"], Netmask: s.Home["mask"], Hostname: s.System["hostname"], Radios: []inform.Radio{}}}
	x.Dev.Model = profile.Model
	x.PhysicalDevice = true
	x.RequireConfigSnapshot = true
	x.ReportUpgradeVersion = *reportUpgradeVersion
	if *reportUpgradeVersion {
		log.Print("controller firmware policy: report offered version only; no download or reboot")
	} else {
		log.Print("controller firmware policy: refuse upgrades")
	}
	x.Dev.ModelDisplay = profile.Display
	x.RestoreFromMgmtCfg()
	slots := map[string]string{}
	if *slotsFile != "" {
		m, err := loadMappingFile(*slotsFile, "-slots")
		if err != nil {
			log.Fatal(err)
		}
		slots = m
	}
	var provisioning provisionRetry
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
		log.Printf("ssh-adopt: listening on %s (allowed controller %v)", *sshListen, adopt.AllowFrom)
		defer adopt.Close()
	}
	client := &http.Client{Timeout: 15 * time.Second}
	if *debugAdoption {
		client.Transport = newStatusLoggingTransport(http.DefaultTransport)
	}
	var actionHandler func(context.Context, string) error
	if *ledControl || *rebootControl {
		actionHandler = nativeActionHandler(keenetic.Local, keenetic.ActionPolicy{LED: *ledControl, Reboot: *rebootControl, WLANTestOnly: *wlanTestOnly}, journal, stop)
	}
	if *rebootControl {
		log.Print("controller native reboot control enabled")
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
				cfg, _ := x.S.CurrentControllerConfig()
				return s.FieldsForMappedConfig(cfg, slots)
			}
			informErr := exchange(ctx, client, x, path, &interval, actionHandler)
			if informErr != nil {
				log.Printf("inform: %v", informErr)
			}
			cfg, plannedIdentity, configVerified := provisioningConfigWithBlocks(x.S, *clientBlockControl)
			if ctx.Err() == nil && informErr == nil && provisioning.ready(plannedIdentity, time.Now()) {
				plannedVersion := x.S.CfgVersion
				if configVerified {
					plan, err := buildPlan(cfg, slots)
					if err == nil && *clientBlockControl {
						plan, err = keenetic.ComposeBlockedPlan(plan, x.S.Blocked)
					}
					if err != nil {
						log.Printf("configuration plan: %v", err)
						recordProvision(plannedVersion, "plan-rejected", nil, time.Time{})
						provisioning.complete(plannedIdentity)
					} else {
						for _, command := range plan.Commands {
							log.Printf("plan: %s", command)
						}
						for _, setting := range plan.Unsupported {
							log.Printf("unsupported: %s", setting)
						}
						for _, setting := range plan.Ignored {
							log.Printf("ignored (no native translation): %s", setting)
						}
						for _, acl := range plan.AccessLists {
							log.Printf("MAC ACL: %s policy=%s clients=%d", acl.Target, acl.Policy, len(acl.MACs))
						}
						phase := "log-only"
						if *apply {
							phase = "not-adopted"
						}
						applyEligible := *apply && x.S.Adopted
						if len(plan.Unsupported) != 0 {
							applyEligible = false
							phase = "unsupported"
						}
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
								phase = "ownership-rejected"
							}
						}
						if applyEligible {
							receipts, receiptErr := keenetic.NewCredentialReceipts(filepath.Join(*stateDir, "credential-receipts.json"), x.S.Key, x.Dev.MAC.String())
							if receiptErr != nil {
								log.Print("credential receipts unavailable; credential writes remain enabled")
							}
							recordProvision(plannedVersion, "applying", &plan, time.Time{})
							applyCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
							err := keenetic.ApplyPlanWatchedWithReceipts(applyCtx, keenetic.Local, plan, journal, func(checkCtx context.Context) error {
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
								if err := keenetic.VerifyRadioSettings(plan, string(running)); err != nil {
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
								return requireProvisioningIdentity(x.S, plannedIdentity, *clientBlockControl)
							}, armWatchdog, receipts)
							cancel()
							if err != nil {
								log.Printf("apply: %v", err)
								if provisioning.failed(plannedIdentity, err, time.Now()) {
									log.Printf("transient apply failure; retry in %s", provisioning.delay)
								}
								recordProvision(plannedVersion, "apply-failed", &plan, provisioning.at)
							} else {
								provisioning.complete(plannedIdentity)
								log.Print("configuration applied and confirmed")
								recordProvision(plannedVersion, "confirmed", &plan, time.Time{})
							}
						} else {
							recordProvision(plannedVersion, phase, &plan, time.Time{})
							provisioning.complete(plannedIdentity)
						}
					}
				} else {
					recordProvision(plannedVersion, "config-unverified", nil, time.Time{})
					provisioning.complete(plannedIdentity)
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
			if err := finishExchangeOutcome(ctx, x, path, &interval, actionHandler); err != nil {
				log.Fatal(err)
			}
			outcome := x.ApplyAdopt(request)
			x.PendingPersistence = &outcome
			if err := finishExchangeOutcome(ctx, x, path, &interval, actionHandler); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func exchange(ctx context.Context, c *http.Client, x *device.Session, path string, interval *time.Duration, actionHandler func(context.Context, string) error) error {
	// A controller can already have switched to the new key. Retain it in memory
	// but do not acknowledge it in another inform until its save succeeds.
	if err := finishExchangeOutcome(ctx, x, path, interval, actionHandler); err != nil {
		return err
	}
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
		return controllerHTTPError{status: resp.StatusCode}
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
	x.PendingPersistence = &outcome
	return finishExchangeOutcome(ctx, x, path, interval, actionHandler)
}

func finishExchangeOutcome(ctx context.Context, x *device.Session, path string, interval *time.Duration, actionHandler func(context.Context, string) error) error {
	outcome := x.PendingPersistence
	if outcome == nil {
		return nil
	}
	if outcome.Changed {
		if err := x.S.Save(path); err != nil {
			return err
		}
	}
	// Clear before dispatch so a later network failure cannot replay actions.
	x.PendingPersistence = nil
	for _, e := range outcome.Events {
		log.Print(e)
	}
	for _, action := range outcome.Actions {
		if actionHandler == nil {
			log.Printf("controller action disabled on Keenetic: %s", action)
		} else if err := actionHandler(ctx, action); err != nil {
			log.Printf("controller action %s: %v", action, err)
		} else {
			log.Printf("controller action dispatched: %s", action)
		}
	}
	if outcome.Interval > 0 {
		*interval = outcome.Interval
	}
	if outcome.Immediate {
		*interval = time.Second
	}
	return nil
}
