// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"testing"
)

func TestCollectionPreservesInactiveWLANsAndDropsTheirStaleStations(t *testing.T) {
	responses := map[string]string{
		"show version": "hw_id: KN-1810\n", "show system": "hostname: State-Lab\n",
		"show interface Bridge0":     "mac: 02:00:00:00:00:01\naddress: 192.0.2.1\n",
		"show interface WifiMaster0": "id: WifiMaster0\nchannel: 6\n",
		"show interface WifiMaster1": "id: WifiMaster1\nchannel: 36\n",
		"show ip arp":                "", "show ip dhcp bindings": "", "show ip hotspot": "",
		"show interface": `Interface, name = "up"
 id: WifiMaster1/AccessPoint0
 type: AccessPoint
 state: up
 link: up
 ssid: UKN-Test-Up
 summary:
  link: disabled
Interface, name = "unlinked"
 id: WifiMaster1/AccessPoint6
 type: AccessPoint
 state: up
 link: down
 ssid: UKN-Test-Unlinked
 summary:
  link: running
Interface, name = "paused"
 id: WifiMaster0/AccessPoint5
 type: AccessPoint
 state: down
 link: down
 ssid: UKN-Test-Paused
Interface, name = "unused"
 id: WifiMaster0/AccessPoint4
 type: AccessPoint
 state: down
 link: down
`,
	}
	for i, ap := range []string{"WifiMaster1/AccessPoint0", "WifiMaster1/AccessPoint6", "WifiMaster0/AccessPoint5", "WifiMaster0/AccessPoint4"} {
		responses["show associations"] += fmt.Sprintf("station:\n mac: 02:00:00:00:00:%02x\n ap: %s\n authenticated: yes\n txbytes: 100\n rxbytes: 200\n", i+2, ap)
	}
	s, err := Collect(context.Background(), func(_ context.Context, query string) ([]byte, error) {
		response, found := responses[query]
		if !found {
			return nil, fmt.Errorf("unexpected query")
		}
		return []byte(response), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.VAPs) != 3 || len(s.InactiveAPs) != 3 {
		t.Fatal("configured inactive or unused AP evidence lost")
	}
	cfg := "wireless.1.ssid=UKN-Test-Up\nwireless.1.parent=wifi1\nwireless.1.devname=ath1\nwireless.2.ssid=UKN-Test-Unlinked\nwireless.2.parent=wifi1\nwireless.2.devname=ath2\nwireless.3.ssid=UKN-Test-Paused\nwireless.3.parent=wifi0\nwireless.3.devname=ath3\n"
	f := s.FieldsForMappedConfig(cfg, map[string]string{"1": "WifiMaster1/AccessPoint0", "2": "WifiMaster1/AccessPoint6", "3": "WifiMaster0/AccessPoint5"})
	if f["num_sta"] != 1 || len(f["sta_table"].([]map[string]any)) != 1 {
		t.Fatal("inactive associations survived in device counts")
	}
	for _, radio := range f["radio_table_stats"].([]map[string]any) {
		want := 0
		if radio["radio"] == "na" {
			want = 1
		}
		if radio["num_sta"] != want {
			t.Fatal("radio count disagrees with active WLANs")
		}
	}
	for _, vap := range f["vap_table"].([]map[string]any) {
		up := vap["name"] == "ath1"
		state, count, tx := "INIT", 0, int64(0)
		if up {
			state, count, tx = "RUN", 1, 100
		}
		if vap["up"] != up || vap["state"] != state || vap["num_sta"] != count || len(vap["sta_table"].([]map[string]any)) != count || vap["tx_bytes"] != tx {
			t.Fatalf("inconsistent WLAN state/counts: %#v", vap)
		}
	}
}

func TestVAPMissingLinkRetainsAdministrativeFallback(t *testing.T) {
	s := &Snapshot{VAPs: []map[string]string{{"id": "WifiMaster0/AccessPoint6", "ssid": "UKN-Test-Fallback", "state": "up"}}}
	v := s.VAPFields()[0]
	if v["up"] != true || v["state"] != "RUN" {
		t.Fatal("missing measurement invented a down transition")
	}
	s.VAPs[0]["state"] = "down"
	v = s.VAPFields()[0]
	if v["up"] != false || v["state"] != "INIT" {
		t.Fatal("administrative down ignored without link measurement")
	}
}
