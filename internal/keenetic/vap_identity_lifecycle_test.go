// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"testing"
)

// These tests pin the client-side identity lifecycle of an administratively
// down (paused) WLAN through the real Collect + FieldsForMappedConfig path with
// synthetic NDM output. They are a synthetic contract, not controller API or
// GUI evidence: the live AP6 run asserted matching ESSID/state, not merely
// alias names. Existing vap_state_test.go already covers mapped down rows;
// this file focuses on config-omission and pause/resume transitions.

const (
	lifecycleMainSSID  = "UKN-Main"
	lifecycleTestSSID0 = "UKN-Test-24"
	lifecycleTestSSID1 = "UKN-Test-5"

	lifecycleCurrentCfg = "wireless.0.ssid=" + lifecycleMainSSID + "\n" +
		"wireless.0.parent=wifi1\n" +
		"wireless.1.ssid=" + lifecycleTestSSID0 + "\n" +
		"wireless.1.parent=wifi0\n" +
		"wireless.1.id=101\n" +
		"wireless.1.devname=ath2\n" +
		"wireless.2.ssid=" + lifecycleTestSSID1 + "\n" +
		"wireless.2.parent=wifi1\n" +
		"wireless.2.id=102\n" +
		"wireless.2.devname=ath3\n"

	// The paused controller request omitted the test WLAN identity fields;
	// only the serving WLAN remains.
	lifecycleOmittedCfg = "wireless.0.ssid=" + lifecycleMainSSID + "\n" +
		"wireless.0.parent=wifi1\n"

	// Same aliases declared twice: collision must refuse alias reuse.
	lifecycleCollisionCfg = "wireless.0.ssid=" + lifecycleMainSSID + "\n" +
		"wireless.0.parent=wifi1\n" +
		"wireless.1.ssid=" + lifecycleTestSSID0 + "\n" +
		"wireless.1.parent=wifi0\n" +
		"wireless.1.id=101\n" +
		"wireless.1.devname=ath2\n" +
		"wireless.2.ssid=" + lifecycleTestSSID1 + "\n" +
		"wireless.2.parent=wifi1\n" +
		"wireless.2.id=102\n" +
		"wireless.2.devname=ath2\n"
)

var lifecycleSlots = map[string]string{
	"1": "WifiMaster0/AccessPoint6",
	"2": "WifiMaster1/AccessPoint6",
}

func lifecycleAPBlock(name, id, state, link, ssid, mac string) string {
	b := fmt.Sprintf("Interface, name = %q\n id: %s\n type: AccessPoint\n state: %s\n link: %s\n mac: %s\n", name, id, state, link, mac)
	if ssid != "" {
		b += " ssid: " + ssid + "\n"
	}
	return b
}

// lifecycleRunner returns a synthetic Runner whose AP6 test WLANs are
// administratively down (paused) or up (resumed); the unused AP4 is always
// down and SSID-free.
func lifecycleRunner(paused bool) Runner {
	state, link := "up", "up"
	if paused {
		state, link = "down", "down"
	}
	showInterface := lifecycleAPBlock("main", "WifiMaster1/AccessPoint0", "up", "up", lifecycleMainSSID, "02:00:00:00:00:a0") +
		lifecycleAPBlock("test24", "WifiMaster0/AccessPoint6", state, link, lifecycleTestSSID0, "02:00:00:00:00:06") +
		lifecycleAPBlock("test5", "WifiMaster1/AccessPoint6", state, link, lifecycleTestSSID1, "02:00:00:00:00:16") +
		lifecycleAPBlock("unused", "WifiMaster0/AccessPoint4", "down", "down", "", "02:00:00:00:00:04")
	responses := map[string]string{
		"show version":               "hw_id: KN-1810\n",
		"show system":                "hostname: Lifecycle-Lab\n",
		"show interface Bridge0":     "mac: 02:00:00:00:00:01\naddress: 192.0.2.1\n",
		"show interface WifiMaster0": "id: WifiMaster0\nchannel: 6\nbandwidth: 40\n",
		"show interface WifiMaster1": "id: WifiMaster1\nchannel: 36\nbandwidth: 80\n",
		"show associations": "station:\n mac: 02:00:00:00:00:02\n ap: WifiMaster1/AccessPoint0\n authenticated: yes\n txbytes: 100\n rxbytes: 200\n" +
			"station:\n mac: 02:00:00:00:00:03\n ap: WifiMaster0/AccessPoint6\n authenticated: yes\n txbytes: 300\n rxbytes: 400\n" +
			"station:\n mac: 02:00:00:00:00:04\n ap: WifiMaster0/AccessPoint4\n authenticated: yes\n txbytes: 500\n rxbytes: 600\n",
		"show ip arp":           "",
		"show interface":        showInterface,
		"show ip dhcp bindings": "",
		"show ip hotspot":       "",
	}
	return func(_ context.Context, query string) ([]byte, error) {
		response, found := responses[query]
		if !found {
			return nil, fmt.Errorf("unexpected query %q", query)
		}
		return []byte(response), nil
	}
}

func lifecycleCollect(t *testing.T, paused bool) *Snapshot {
	t.Helper()
	s, err := Collect(context.Background(), lifecycleRunner(paused))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func vapByESSID(t *testing.T, f map[string]any, essid string) map[string]any {
	t.Helper()
	for _, vap := range f["vap_table"].([]map[string]any) {
		if vap["essid"] == essid {
			return vap
		}
	}
	t.Fatalf("no VAP row for ESSID %q", essid)
	return nil
}

func assertDownRow(t *testing.T, vap map[string]any, name, id string) {
	t.Helper()
	if vap["name"] != name {
		t.Fatalf("paused WLAN name = %v, want %q", vap["name"], name)
	}
	if id != "" {
		if vap["id"] != id {
			t.Fatalf("paused WLAN id = %v, want %q", vap["id"], id)
		}
	} else if _, exists := vap["id"]; exists {
		t.Fatalf("paused WLAN got controller id without a matching config entry: %#v", vap)
	}
	if vap["up"] != false || vap["state"] != "INIT" {
		t.Fatalf("paused WLAN not up=false/INIT: %#v", vap)
	}
	if vap["num_sta"] != 0 || len(vap["sta_table"].([]map[string]any)) != 0 || vap["tx_bytes"] != int64(0) || vap["rx_bytes"] != int64(0) {
		t.Fatalf("paused WLAN retained clients or traffic: %#v", vap)
	}
}

// Case 1: a configured native down AP whose SSID is still present, with the
// current controller config and explicit slot map matching it, emits the
// controller alias with up=false/INIT and zero clients/traffic.
func TestPausedMappedDownAPKeepsAliasAndDropsStaleClients(t *testing.T) {
	s := lifecycleCollect(t, true)
	if len(s.InactiveAPs) != 3 {
		t.Fatalf("inactive APs = %v", s.InactiveAPs)
	}
	f := s.FieldsForMappedConfig(lifecycleCurrentCfg, lifecycleSlots)
	assertDownRow(t, vapByESSID(t, f, lifecycleTestSSID0), "ath2", "101")
	assertDownRow(t, vapByESSID(t, f, lifecycleTestSSID1), "ath3", "102")
	// The serving WLAN has no slot and keeps its native name, up/RUN.
	main := vapByESSID(t, f, lifecycleMainSSID)
	if main["name"] != "WifiMaster1/AccessPoint0" || main["up"] != true || main["state"] != "RUN" || main["num_sta"] != 1 {
		t.Fatalf("serving WLAN row wrong: %#v", main)
	}
	// Only the serving AP's client survives; both down APs' stale records are
	// excluded from the common station source.
	if f["num_sta"] != 1 || len(f["sta_table"].([]map[string]any)) != 1 {
		t.Fatalf("stale associations survived: num_sta=%v", f["num_sta"])
	}
	for _, sta := range f["sta_table"].([]map[string]any) {
		if sta["vap"] != "WifiMaster1/AccessPoint0" {
			t.Fatalf("unexpected station identity: %#v", sta)
		}
	}
}

// Case 2: the same native down AP retains its native name when the current
// controller config omits the test WLAN identity fields — absence of an alias
// in the emitted payload is client-side; this does not prove API behavior.
func TestPausedOmittedConfigRetainsNativeName(t *testing.T) {
	s := lifecycleCollect(t, true)
	f := s.FieldsForMappedConfig(lifecycleOmittedCfg, lifecycleSlots)
	assertDownRow(t, vapByESSID(t, f, lifecycleTestSSID0), "WifiMaster0/AccessPoint6", "")
	assertDownRow(t, vapByESSID(t, f, lifecycleTestSSID1), "WifiMaster1/AccessPoint6", "")
}

// Case 3: a native down AP without an SSID emits no VAP row at all, yet its
// stale authenticated association is still excluded.
func TestSSIDFreeDownAPEmitsNoVAPRowButExcludesStaleStation(t *testing.T) {
	s := lifecycleCollect(t, true)
	for _, vap := range s.VAPs {
		if vap["id"] == "WifiMaster0/AccessPoint4" {
			t.Fatalf("SSID-free down AP became a VAP row: %#v", vap)
		}
	}
	found := false
	for _, id := range s.InactiveAPs {
		if id == "WifiMaster0/AccessPoint4" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SSID-free down AP missing from InactiveAPs: %v", s.InactiveAPs)
	}
	f := s.FieldsForMappedConfig(lifecycleCurrentCfg, lifecycleSlots)
	for _, vap := range f["vap_table"].([]map[string]any) {
		if vap["bssid"] == "02:00:00:00:00:04" {
			t.Fatalf("SSID-free down AP emitted a VAP row: %#v", vap)
		}
	}
	for _, sta := range f["sta_table"].([]map[string]any) {
		if sta["mac"] == "02:00:00:00:00:04" || sta["vap"] == "WifiMaster0/AccessPoint4" {
			t.Fatalf("stale station of SSID-free AP survived: %#v", sta)
		}
	}
	if f["num_sta"] != 1 {
		t.Fatalf("num_sta = %v, want only the serving client", f["num_sta"])
	}
}

// Case 4: resume restores the current matching alias (up/RUN, client rows
// relabeled), while a slot mismatch or duplicate alias refuses alias reuse.
func TestResumeRestoresAliasAndCollisionRefusesReuse(t *testing.T) {
	s := lifecycleCollect(t, false)
	f := s.FieldsForMappedConfig(lifecycleCurrentCfg, lifecycleSlots)
	resumed := vapByESSID(t, f, lifecycleTestSSID0)
	if resumed["name"] != "ath2" || resumed["id"] != "101" || resumed["up"] != true || resumed["state"] != "RUN" {
		t.Fatalf("resume did not restore alias/state: %#v", resumed)
	}
	if resumed["num_sta"] != 1 || len(resumed["sta_table"].([]map[string]any)) != 1 {
		t.Fatalf("resume did not restore the client row: %#v", resumed)
	}
	clients := resumed["sta_table"].([]map[string]any)
	if clients[0]["vap"] != "ath2" {
		t.Fatalf("nested client not relabeled on resume: %#v", clients[0])
	}
	other := vapByESSID(t, f, lifecycleTestSSID1)
	if other["name"] != "ath3" || other["up"] != true || other["state"] != "RUN" {
		t.Fatalf("5 GHz resume wrong: %#v", other)
	}
	if f["num_sta"] != 2 {
		t.Fatalf("num_sta = %v, want serving plus resumed test client", f["num_sta"])
	}

	// Slot mismatch: slot 1 points at another native AP, so no alias is reused.
	mismatch := s.FieldsForMappedConfig(lifecycleCurrentCfg, map[string]string{
		"1": "WifiMaster0/AccessPoint0", "2": "WifiMaster1/AccessPoint6"})
	row := vapByESSID(t, mismatch, lifecycleTestSSID0)
	if row["name"] != "WifiMaster0/AccessPoint6" {
		t.Fatalf("slot mismatch reused alias: %#v", row)
	}
	if _, exists := row["id"]; exists {
		t.Fatalf("slot mismatch assigned controller id: %#v", row)
	}

	// Duplicate alias in the config: aliasOwners > 1 refuses reuse on both rows.
	collision := s.FieldsForMappedConfig(lifecycleCollisionCfg, lifecycleSlots)
	for _, essid := range []string{lifecycleTestSSID0, lifecycleTestSSID1} {
		row := vapByESSID(t, collision, essid)
		if row["name"] == "ath2" || row["name"] == "ath3" {
			t.Fatalf("colliding alias reused for %q: %#v", essid, row)
		}
		if _, exists := row["id"]; exists {
			t.Fatalf("colliding alias assigned id for %q: %#v", essid, row)
		}
	}
}
