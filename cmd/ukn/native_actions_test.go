// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/device"
	"github.com/antoniocosta/unifi-openwrt/internal/inform"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestRebootDispatchRejectsPendingRecoveryAndNativeFailure(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "rollback.json")
	if err := os.WriteFile(journal, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped := false
	handler := nativeActionHandler(func(context.Context, string) ([]byte, error) { t.Fatal("reboot interrupted recovery"); return nil, nil }, keenetic.ActionPolicy{Reboot: true}, journal, func() { stopped = true })
	if handler(context.Background(), "reboot") == nil || stopped {
		t.Fatal("pending recovery did not block reboot")
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	want := errors.New("native command rejected")
	handler = nativeActionHandler(func(context.Context, string) ([]byte, error) { return nil, want }, keenetic.ActionPolicy{Reboot: true}, journal, func() { stopped = true })
	if err := handler(context.Background(), "reboot"); !errors.Is(err, want) || stopped {
		t.Fatal("failed reboot stopped the daemon")
	}
}

func TestEncryptedControllerRebootIsOptInAndUpgradeIndependent(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, reply := range []string{`{"_type":"reboot"}`, `{"_type":"cmd","cmd":"restart"}`, `{"_type":"upgrade","version":"9.9.9","url":"http://invalid/firmware.bin"}`} {
			t.Run(reply+map[bool]string{false: " disabled", true: " enabled"}[enabled], func(t *testing.T) {
				mac, _ := net.ParseMAC("02:00:00:00:00:01")
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/inform" {
						t.Error("unexpected firmware request")
						w.WriteHeader(500)
						return
					}
					b, err := inform.Encode(mac, inform.DefaultKeyHex, inform.CBC, []byte(reply), nil)
					if err != nil {
						t.Error(err)
						return
					}
					w.Write(b)
				}))
				defer server.Close()
				dir := t.TempDir()
				boot := time.Now().Add(-time.Hour)
				x := &device.Session{S: device.NewState(server.URL + "/inform"), StateDir: dir, Dev: inform.Device{MAC: mac}, Boot: boot, PhysicalDevice: true, ReportUpgradeVersion: true}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				handler := nativeActionHandler(func(_ context.Context, cmd string) ([]byte, error) {
					calls++
					if cmd != "system reboot 5" {
						t.Fatal(cmd)
					}
					return nil, nil
				}, keenetic.ActionPolicy{Reboot: enabled}, filepath.Join(dir, "rollback.json"), cancel)
				interval := time.Second
				if err := exchange(ctx, server.Client(), x, filepath.Join(dir, "state.json"), &interval, handler); err != nil {
					t.Fatal(err)
				}
				expected := 0
				if enabled && reply != `{"_type":"upgrade","version":"9.9.9","url":"http://invalid/firmware.bin"}` {
					expected = 1
				}
				if calls != expected || (ctx.Err() != nil) != (expected == 1) {
					t.Fatalf("calls=%d canceled=%v", calls, ctx.Err())
				}
				if !x.Boot.Equal(boot) {
					t.Fatal("physical reboot request faked uptime")
				}
			})
		}
	}
}
