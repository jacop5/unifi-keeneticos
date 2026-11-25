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
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
)

func TestFailedAdoptionSaveBlocksAcknowledgementAndDefersActions(t *testing.T) {
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	key := "00112233445566778899aabbccddeeff"
	const config = "wireless.1.ssid=UKN-Test-Pending\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// A directory blocks atomic replacement even under root in the Linux lab.
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		decryptKey, cipher := inform.DefaultKeyHex, inform.CBC
		if n > 1 {
			decryptKey, cipher = key, inform.GCM
		}
		body, _ := io.ReadAll(r.Body)
		if _, _, err := inform.Decode(body, decryptKey); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		reply := map[string]any{"_type": "noop"}
		if n == 1 {
			reply = map[string]any{"_type": "setparam", "interval": 1, "system_cfg": config,
				"mgmt_cfg": "authkey=" + key + "\nuse_aes_gcm=true\nled_enabled=true\ncfgversion=persisted-version\n"}
		} else {
			saved, found, err := device.Load(path)
			if err != nil || !found || saved.Key != key || !saved.Adopted || !saved.GCM {
				t.Error("controller received acknowledgement before key persistence")
				w.WriteHeader(500)
				return
			}
			if text, verified := saved.CurrentControllerConfig(); !verified || text != config {
				t.Error("controller received acknowledgement before configuration persistence")
				w.WriteHeader(500)
				return
			}
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
	x := &device.Session{S: device.NewState(server.URL), StateDir: dir,
		PhysicalDevice: true, RequireConfigSnapshot: true, Dev: inform.Device{MAC: mac, IP: "192.0.2.1"}, Boot: time.Now()}
	interval := 10 * time.Second
	actions := 0
	handler := func(context.Context, string) error { actions++; return nil }
	for i := 0; i < 2; i++ {
		if err := exchange(context.Background(), server.Client(), x, path, &interval, handler); err == nil {
			t.Fatal("blocked persistence succeeded")
		}
		if requests.Load() != 1 || actions != 0 || interval != 10*time.Second || x.PendingPersistence == nil {
			t.Fatal("failed save acknowledged state, dispatched actions or lost retry")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := exchange(context.Background(), server.Client(), x, path, &interval, handler); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 3 || actions != 1 || interval != time.Second || x.PendingPersistence != nil {
		t.Fatal("successful local persistence did not resume once with the new key")
	}
}
