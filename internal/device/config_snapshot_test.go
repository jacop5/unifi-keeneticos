// SPDX-License-Identifier: GPL-2.0-or-later
package device

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/inform"
)

func TestStoredConfigMatchesVersionAndStaysPrivate(t *testing.T) {
	dir := t.TempDir()
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	x := &Session{S: NewState("http://127.0.0.1:9/inform"), StateDir: dir,
		RequireConfigSnapshot: true, Dev: inform.Device{MAC: mac}, Boot: time.Now()}
	x.S.CfgVersion = "legacy-version"
	assertVersion := func(want string) {
		t.Helper()
		b, err := x.Payload(time.Now())
		var payload map[string]any
		if err != nil || json.Unmarshal(b, &payload) != nil || payload["cfgversion"] != want {
			t.Fatal("wrong advertised config version")
		}
		if strings.Contains(string(b), "private-config-password") {
			t.Fatal("configuration secret leaked into inform")
		}
	}
	assertVersion("0")
	if x.S.CfgVersion != "legacy-version" {
		t.Fatal("fresh-config request changed received version")
	}
	// An unreadable diagnostic copy must not prevent the authoritative snapshot
	// from being stored together with its received version and adoption state.
	if err := os.Mkdir(filepath.Join(dir, "system_cfg.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := "aaa.1.wpa.psk=private-config-password\n"
	reply, _ := json.Marshal(Reply{Type: "setparam", CfgVersion: "new-version", SystemCfg: cfg})
	outcome, err := x.Handle(reply, time.Now())
	if err != nil || !outcome.Changed {
		t.Fatal("configuration not staged")
	}
	path := filepath.Join(dir, "state.json")
	if err := x.S.Save(path); err != nil {
		t.Fatal(err)
	}
	saved, found, err := Load(path)
	if err != nil || !found {
		t.Fatal("state unavailable")
	}
	if text, verified := saved.CurrentControllerConfig(); !verified || text != cfg {
		t.Fatal("lost version-bound snapshot")
	}
	x.S = saved
	assertVersion("new-version")
	for _, authority := range []string{"key", "controller"} {
		changed := saved
		if authority == "key" {
			changed.Key = "another-synthetic-key"
		} else {
			changed.InformURL = "http://127.0.0.2:9/inform"
		}
		if _, verified := changed.CurrentControllerConfig(); verified {
			t.Fatal("accepted another authority's configuration")
		}
	}
	x.S.CfgVersion = "different-version"
	if _, verified := x.S.CurrentControllerConfig(); verified {
		t.Fatal("accepted a different version's body")
	}
	assertVersion("0")
}
