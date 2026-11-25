// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
)

func TestConfigIdentityChangesWithBodyAndRejectsUnboundData(t *testing.T) {
	s := device.State{CfgVersion: "version", Key: "synthetic-key", InformURL: "http://127.0.0.1:9/inform"}
	s.BindControllerConfig("first-body")
	_, first, verified := provisioningConfig(s)
	if !verified {
		t.Fatal("rejected bound configuration")
	}
	p := provisionRetry{}
	p.complete(first)
	s.ControllerConfig.Text = "second-body"
	_, second, verified := provisioningConfig(s)
	if !verified || first == second || !p.ready(second, time.Now()) {
		t.Fatal("same-version body change was skipped")
	}
	s.CfgVersion = "other-version"
	if text, _, verified := provisioningConfig(s); verified || text != "" {
		t.Fatal("unbound configuration reached planning")
	}
	s.ControllerConfig = nil
	if text, _, verified := provisioningConfig(s); verified || text != "" {
		t.Fatal("legacy configuration reached planning")
	}
}

func TestConfigSnapshotDaemonRefreshesLegacyAndIgnoresDiagnosticFailure(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	// This blocks the diagnostic mirror, while the private atomic state record
	// remains writable. The actual daemon must still use the received snapshot.
	if err := os.Mkdir(filepath.Join(state, "system_cfg.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	const cfg = `wireless.1.ssid=UKN-Test-Snapshot
wireless.1.parent=wifi0
wireless.1.usage=user
aaa.1.ssid=UKN-Test-Snapshot
aaa.1.wpa=2
aaa.1.wpa.psk=private-snapshot-password
`
	const key = "00112233445566778899aabbccddeeff"
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		_, plain, err := inform.Decode(body, key)
		var payload map[string]any
		if err != nil || json.Unmarshal(plain, &payload) != nil {
			t.Error("invalid encrypted inform")
			w.WriteHeader(400)
			return
		}
		if strings.Contains(string(plain), "private-snapshot-password") {
			t.Error("configuration secret leaked")
		}
		reply := snapshotTestReply(n == 1, cfg)
		if n == 1 && payload["cfgversion"] != "0" {
			t.Error("legacy daemon did not request full config")
		}
		if n > 1 && payload["cfgversion"] != "snapshot-version" {
			t.Error("did not acknowledge verified version")
		}
		js, _ := json.Marshal(reply)
		packet, err := inform.Encode(mac, key, inform.GCM, js, nil)
		if err != nil {
			t.Error(err)
			return
		}
		w.Write(packet)
	}))
	defer server.Close()
	s := device.NewState(server.URL)
	s.Key, s.Adopted, s.GCM, s.CfgVersion = key, true, true, "legacy-version"
	if err := s.Save(filepath.Join(state, "state.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "slots.json"), []byte(`{"1":"WifiMaster0/AccessPoint6"}`), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$2" in
 'show version') printf 'hw_id: KN-1810\n';;
 'show system') printf 'hostname: Snapshot-Lab\nuptime: 123\n';;
 'show interface Bridge0') printf 'id: Bridge0\naddress: 192.0.2.1\nmac: 02:00:00:00:00:01\nmask: 255.255.255.0\n';;
 'show interface WifiMaster0') printf 'id: WifiMaster0\nchannel: 6\n';;
 'show interface WifiMaster1') printf 'id: WifiMaster1\nchannel: 36\n';;
 'show associations'|'show ip arp'|'show interface'|'show ip dhcp bindings'|'show ip hotspot') :;;
 *) exit 99;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartupCLIHelper$", "--", "-state", state,
		"-url", server.URL, "-ssh-listen", "", "-slots", filepath.Join(dir, "slots.json"), "-wlan-test-only")
	cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "PATH="+dir+":"+os.Getenv("PATH"))
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		r, err := readProvisionStatus(filepath.Join(state, "provision-status.json"))
		if err == nil && r.Phase == "log-only" && r.ReceivedVersion == "snapshot-version" && requests.Load() > 1 {
			saved, found, err := device.Load(filepath.Join(state, "state.json"))
			if err != nil || !found {
				t.Fatal("missing committed state")
			}
			if text, verified := saved.CurrentControllerConfig(); !verified || text != cfg {
				t.Fatal("wrong authoritative snapshot")
			}
			if saved.Key != key || !saved.Adopted {
				t.Fatal("migration lost adoption")
			}
			b, _ := os.ReadFile(filepath.Join(dir, "agent.log"))
			if !strings.Contains(string(b), `ssid "UKN-Test-Snapshot"`) || strings.Contains(string(b), "private-snapshot-password") {
				t.Fatal("wrong plan or leaked credential")
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "agent.log"))
	t.Fatalf("daemon did not plan refreshed snapshot: %s", b)
}

func snapshotTestReply(first bool, cfg string) map[string]any {
	if first {
		return map[string]any{"_type": "setparam", "cfgversion": "snapshot-version", "system_cfg": cfg, "interval": 1}
	}
	return map[string]any{"_type": "noop", "interval": 1}
}
