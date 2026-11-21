// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptedControllerLEDDispatch(t *testing.T) {
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	request := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request++
		value := "false"
		if request == 2 {
			value = "true"
		}
		reply, _ := json.Marshal(map[string]string{"_type": "setparam", "mgmt_cfg": "led_enabled=" + value + "\n"})
		packet, err := inform.Encode(mac, inform.DefaultKeyHex, inform.CBC, reply, nil)
		if err != nil {
			t.Error(err)
			return
		}
		w.Write(packet)
	}))
	defer server.Close()
	dir := t.TempDir()
	x := &device.Session{S: device.NewState(server.URL), StateDir: dir, Dev: inform.Device{MAC: mac}, Boot: time.Now()}
	var commands []string
	handler := func(ctx context.Context, action string) error {
		return keenetic.LEDAction(ctx, func(_ context.Context, command string) ([]byte, error) {
			commands = append(commands, command)
			return nil, nil
		}, action)
	}
	interval := time.Second
	for i := 0; i < 2; i++ {
		if err := exchange(context.Background(), server.Client(), x, filepath.Join(dir, "state.json"), &interval, handler); err != nil {
			t.Fatal(err)
		}
	}
	if len(commands) != 2 || commands[0] != "system led power shutdown all" || commands[1] != "system no led power shutdown" {
		t.Fatal(commands)
	}
}

func TestControllerUpgradePolicy(t *testing.T) {
	for _, report := range []bool{false, true} {
		t.Run(fmt.Sprintf("report=%t", report), func(t *testing.T) {
			mac, _ := net.ParseMAC("02:00:00:00:00:01")
			requests := 0
			firmwareURL := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/inform" {
					t.Error("firmware URL was fetched")
					w.WriteHeader(500)
					return
				}
				requests++
				body, _ := io.ReadAll(r.Body)
				_, plain, err := inform.Decode(body, inform.DefaultKeyHex)
				if err != nil {
					t.Error(err)
					return
				}
				var payload map[string]any
				if err := json.Unmarshal(plain, &payload); err != nil {
					t.Error(err)
				}
				if requests == 2 {
					want := inform.U7PG2Version
					if report {
						want = "99.99.99"
					}
					if payload["version"] != want {
						t.Errorf("reported version=%v want=%s", payload["version"], want)
					}
				}
				reply := map[string]string{"_type": "noop"}
				if requests == 1 {
					reply = map[string]string{"_type": "upgrade", "version": "99.99.99", "url": firmwareURL}
				}
				encoded, _ := json.Marshal(reply)
				packet, err := inform.Encode(mac, inform.DefaultKeyHex, inform.CBC, encoded, nil)
				if err != nil {
					t.Error(err)
					return
				}
				w.Write(packet)
			}))
			defer server.Close()
			firmwareURL = server.URL + "/firmware.bin"
			dir := t.TempDir()
			x := &device.Session{S: device.NewState(server.URL + "/inform"), StateDir: dir, Dev: inform.Device{MAC: mac}, Boot: time.Now().Add(-time.Hour), PhysicalDevice: true, ReportUpgradeVersion: report}
			boot := x.Boot
			path := filepath.Join(dir, "state.json")
			interval := time.Second
			for i := 0; i < 2; i++ {
				if err := exchange(context.Background(), server.Client(), x, path, &interval, func(context.Context, string) error {
					t.Error("upgrade invoked a native action")
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if !x.Boot.Equal(boot) || requests != 2 {
				t.Fatal("upgrade moved boot clock or fetched firmware")
			}
			if report {
				saved, found, err := device.Load(path)
				if err != nil || !found || saved.Version != "99.99.99" {
					t.Fatalf("reported version not persisted: found=%v err=%v version=%s", found, err, saved.Version)
				}
			}
		})
	}
}

func TestControllerProvisioningPersistsAndSwitchesCipher(t *testing.T) {
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	const key = "a066a066a066a066a066a066a066a066"
	const cfg = "radio.1.phyname=wifi0\nradio.1.channel=6\n"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		decryptKey := inform.DefaultKeyHex
		cipher := inform.CBC
		if requests > 1 {
			decryptKey = key
			cipher = inform.GCM
		}
		header, _, err := inform.Decode(body, decryptKey)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if requests > 1 && header.Flags&inform.FlagGCM == 0 {
			t.Error("second inform did not use GCM")
		}
		reply := map[string]any{"_type": "noop"}
		if requests == 1 {
			reply = map[string]any{"_type": "setparam", "mgmt_cfg": "authkey=" + key + "\nuse_aes_gcm=true\ncfgversion=test-version\n", "system_cfg": cfg}
		}
		plain, _ := json.Marshal(reply)
		packet, err := inform.Encode(mac, decryptKey, cipher, plain, nil)
		if err != nil {
			t.Error(err)
			return
		}
		w.Write(packet)
	}))
	defer server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	x := &device.Session{S: device.NewState(server.URL + "/inform"), StateDir: dir, Dev: inform.Device{MAC: mac, IP: "192.0.2.1"}, Boot: time.Now()}
	interval := 10 * time.Second
	if err := exchange(context.Background(), server.Client(), x, path, &interval, nil); err != nil {
		t.Fatal(err)
	}
	saved, found, err := device.Load(path)
	if err != nil || !found || !saved.Adopted || !saved.GCM || saved.Key != key {
		t.Fatalf("adoption state not persisted: found=%v err=%v", found, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "system_cfg.txt"))
	if err != nil || string(b) != cfg {
		t.Fatal("configuration not persisted", err)
	}
	x.S = saved
	if err := exchange(context.Background(), server.Client(), x, path, &interval, nil); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal(requests)
	}
}

func TestEncryptedControllerExchange(t *testing.T) {
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, plain, err := inform.Decode(body, inform.DefaultKeyHex)
		if err != nil || len(plain) == 0 {
			t.Errorf("invalid inform: %v", err)
			w.WriteHeader(400)
			return
		}
		reply, err := inform.Encode(mac, inform.DefaultKeyHex, inform.CBC, []byte(`{"_type":"noop","interval":5}`), nil)
		if err != nil {
			t.Error(err)
			return
		}
		w.Write(reply)
	}))
	defer server.Close()
	x := &device.Session{S: device.NewState(server.URL + "/inform"), Dev: inform.Device{MAC: mac, IP: "192.0.2.1"}, Boot: time.Now()}
	interval := 10 * time.Second
	if err := exchange(context.Background(), server.Client(), x, filepath.Join(t.TempDir(), "state.json"), &interval, nil); err != nil {
		t.Fatal(err)
	}
	if interval != 5*time.Second {
		t.Fatalf("interval=%s", interval)
	}
}
