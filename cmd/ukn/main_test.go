// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	if err := exchange(context.Background(), server.Client(), x, path, &interval); err != nil {
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
	if err := exchange(context.Background(), server.Client(), x, path, &interval); err != nil {
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
	if err := exchange(context.Background(), server.Client(), x, filepath.Join(t.TempDir(), "state.json"), &interval); err != nil {
		t.Fatal(err)
	}
	if interval != 5*time.Second {
		t.Fatalf("interval=%s", interval)
	}
}
