// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestBlockedApplyConfirmsOrRollsBackAcrossEncryptedControllerExchange(t *testing.T) {
	const target, client = "WifiMaster0/AccessPoint6", "02:00:00:00:00:0a"
	const key = "00112233445566778899aabbccddeeff"
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	for _, scenario := range []string{"confirm", "unblock-during-confirmation", "controller-outage", "removed-during-confirmation"} {
		t.Run(scenario, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				body, _ := io.ReadAll(r.Body)
				_, plain, err := inform.Decode(body, key)
				var payload map[string]any
				if err != nil || json.Unmarshal(plain, &payload) != nil || payload["cfgversion"] != "same-version" {
					t.Error("invalid encrypted confirmation request")
					w.WriteHeader(400)
					return
				}
				if scenario == "controller-outage" {
					w.WriteHeader(503)
					return
				}
				reply := map[string]any{"_type": "noop", "interval": 1}
				if scenario == "unblock-during-confirmation" && n == 1 {
					reply = map[string]any{"_type": "setparam", "blocked_sta": "", "interval": 1}
				}
				if scenario == "removed-during-confirmation" {
					reply = map[string]any{"_type": "setdefault"}
				}
				js, _ := json.Marshal(reply)
				packet, err := inform.Encode(mac, key, inform.GCM, js, nil)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				_, _ = w.Write(packet)
			}))
			defer server.Close()
			dir := t.TempDir()
			statePath, journal := filepath.Join(dir, "state.json"), filepath.Join(dir, "rollback.json")
			s := device.NewState(server.URL)
			s.Key, s.Adopted, s.GCM, s.CfgVersion = key, true, true, "same-version"
			s.BindControllerConfig("explicit-controller-acl-baseline")
			s.Blocked = []string{client}
			if err := s.Save(statePath); err != nil {
				t.Fatal(err)
			}
			x := &device.Session{S: s, StateDir: dir, Boot: time.Now(), PhysicalDevice: true,
				RequireConfigSnapshot: true, Dev: inform.Device{MAC: mac, Radios: []inform.Radio{}}}
			base := keenetic.Plan{AccessLists: []keenetic.AccessList{{Target: target, Policy: "none"}}}
			policy, up, member, mutations, armed := "none", true, false, 0, false
			running := func() string {
				state := "down"
				if up {
					state = "up"
				}
				text := "known host client " + client + "\ninterface " + target + "\n    " + state + "\n    mac access-list type " + policy + "\n"
				if member {
					text += "    mac access-list address " + client + "\n"
				}
				return text + "!\n"
			}
			run := func(_ context.Context, command string) ([]byte, error) {
				if command == "show running-config" {
					return []byte(running()), nil
				}
				if !armed {
					t.Fatal("mutation before watchdog was armed")
				}
				info, err := os.Stat(journal)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("mutation without private persisted rollback")
				}
				setting, ok := strings.CutPrefix(command, "interface "+target+" ")
				if !ok {
					return nil, fmt.Errorf("command escaped owned AP")
				}
				switch setting {
				case "up":
					up = true
				case "down":
					up = false
				case "mac access-list type none":
					policy = "none"
				case "mac access-list type deny":
					policy = "deny"
				case "mac access-list address " + client:
					member = true
				case "no mac access-list address " + client:
					member = false
				default:
					return nil, fmt.Errorf("unexpected native setting")
				}
				mutations++
				return nil, nil
			}
			interval := time.Second
			apply := func() error {
				plan, err := keenetic.ComposeBlockedPlan(base, x.S.Blocked)
				if err != nil {
					return err
				}
				_, identity, _ := provisioningConfigWithBlocks(x.S, true)
				return keenetic.ApplyPlanWatched(context.Background(), run, plan, journal, func(ctx context.Context) error {
					if err := keenetic.VerifyAccessLists(plan, running()); err != nil {
						return err
					}
					if err := exchange(ctx, server.Client(), x, statePath, &interval, nil); err != nil {
						return err
					}
					return requireProvisioningIdentity(x.S, identity, true)
				}, func(path string) error {
					if _, err := os.Stat(path); err != nil {
						return err
					}
					armed = true
					return nil
				})
			}
			err := apply()
			if (err == nil) != (scenario == "confirm") || mutations == 0 || requests.Load() != 1 || !up {
				t.Fatalf("unexpected outcome: scenario=%s err=%v mutations=%d", scenario, err, mutations)
			}
			if (policy == "deny" && member) != (scenario == "confirm") {
				t.Fatal("confirmation or rollback did not preserve intended ACL")
			}
			if scenario != "confirm" && (policy != "none" || member) {
				t.Fatal("failed confirmation lost exact baseline")
			}
			if _, err := os.Stat(journal); !os.IsNotExist(err) {
				t.Fatal("journal retained after complete confirmation/recovery")
			}
			if scenario == "removed-during-confirmation" && x.S.Adopted {
				t.Fatal("controller removal did not invalidate adoption")
			}
			if scenario == "confirm" || scenario == "unblock-during-confirmation" {
				before := mutations
				if err := apply(); err != nil || mutations != before || requests.Load() != 2 {
					t.Fatalf("fresh confirmation of reconciled no-op failed: %v", err)
				}
				stored, found, err := device.Load(statePath)
				if err != nil || !found || len(stored.Blocked) != len(x.S.Blocked) {
					t.Fatal("controller block intent was not durably retained")
				}
			}
		})
	}
}
