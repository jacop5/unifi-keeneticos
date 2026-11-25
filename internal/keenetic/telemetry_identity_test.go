// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strings"
	"testing"
)

const sharedSSIDConfig = `wireless.1.ssid=Shared
wireless.1.parent=wifi0
wireless.1.id=101
wireless.1.devname=wifi0ap1
wireless.2.ssid=Shared
wireless.2.parent=wifi0
wireless.2.id=102
wireless.2.devname=wifi0ap2
`

func sharedSSIDSnapshot() *Snapshot {
	s := &Snapshot{}
	for _, slot := range []string{"1", "2", "0"} {
		ap := "WifiMaster0/AccessPoint" + slot
		s.VAPs = append(s.VAPs, map[string]string{"id": ap, "ssid": "Shared", "mac": "02:00:00:00:01:0" + slot})
		s.Stations = append(s.Stations, map[string]string{"ap": ap, "mac": "02:00:00:00:02:0" + slot,
			"authenticated": "yes", "txbytes": "17", "rxbytes": "23"})
	}
	return s
}

func TestMappedTelemetryKeepsSameSSIDWLANsAndClientViewsDistinct(t *testing.T) {
	s := sharedSSIDSnapshot()
	f := s.FieldsForMappedConfig(sharedSSIDConfig, map[string]string{
		"1": "WifiMaster0/AccessPoint1", "2": "WifiMaster0/AccessPoint2"})
	wantNames := map[string]string{"02:00:00:00:01:01": "wifi0ap1", "02:00:00:00:01:02": "wifi0ap2",
		"02:00:00:00:01:00": "WifiMaster0/AccessPoint0"}
	wantIDs := map[string]string{"wifi0ap1": "101", "wifi0ap2": "102"}
	for _, vap := range f["vap_table"].([]map[string]any) {
		name := wantNames[vap["bssid"].(string)]
		if vap["name"] != name || vap["tx_bytes"] != int64(17) || vap["rx_bytes"] != int64(23) {
			t.Fatalf("wrong identity or traffic: %v", vap)
		}
		if want := wantIDs[name]; want != "" {
			if vap["id"] != want {
				t.Fatal("wrong controller WLAN id")
			}
		} else if _, exists := vap["id"]; exists {
			t.Fatal("unowned native WLAN was assigned controller identity")
		}
		clients := vap["sta_table"].([]map[string]any)
		if len(clients) != 1 || clients[0]["vap"] != name {
			t.Fatal("nested client has inconsistent WLAN identity")
		}
		for _, sta := range f["sta_table"].([]map[string]any) {
			if sta["mac"] == clients[0]["mac"] && sta["vap"] != name {
				t.Fatal("top-level client has inconsistent identity")
			}
		}
	}
	if f["num_sta"] != 3 {
		t.Fatal("mapping changed native client count")
	}
	for _, vap := range s.Fields()["vap_table"].([]map[string]any) {
		if !strings.HasPrefix(vap["name"].(string), "WifiMaster0/") {
			t.Fatal("mapping mutated the native snapshot")
		}
	}
}

func TestAmbiguousTelemetryRetainsNativeIdentities(t *testing.T) {
	slots := map[string]string{"1": "WifiMaster0/AccessPoint1", "2": "WifiMaster0/AccessPoint2"}
	for _, test := range []struct {
		name, config string
		mapping      map[string]string
		fallback     bool
	}{
		{"no-ownership", sharedSSIDConfig, nil, false},
		{"ambiguous-controller-entries", sharedSSIDConfig, nil, true},
		{"ambiguous-native-entries", strings.Split(sharedSSIDConfig, "wireless.2.ssid")[0], nil, true},
		{"duplicate-slots", sharedSSIDConfig, map[string]string{"1": "WifiMaster0/AccessPoint1", "2": "WifiMaster0/AccessPoint1"}, false},
		{"duplicate-aliases", strings.ReplaceAll(sharedSSIDConfig, "wifi0ap2", "wifi0ap1"), slots, false},
		{"wrong-band", strings.ReplaceAll(sharedSSIDConfig, "parent=wifi0", "parent=wifi1"), slots, false},
		{"ssid-drift", strings.ReplaceAll(sharedSSIDConfig, "ssid=Shared", "ssid=Other"), slots, false},
		{"native-name-collision", strings.ReplaceAll(strings.ReplaceAll(sharedSSIDConfig, "wifi0ap1", "WifiMaster0/AccessPoint0"), "wifi0ap2", "WifiMaster0/AccessPoint1"), slots, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := sharedSSIDSnapshot()
			var f map[string]any
			if test.fallback {
				f = s.FieldsForConfig(test.config)
			} else {
				f = s.FieldsForMappedConfig(test.config, test.mapping)
			}
			for _, vap := range f["vap_table"].([]map[string]any) {
				if _, exists := vap["id"]; exists || !strings.HasPrefix(vap["name"].(string), "WifiMaster0/") {
					t.Fatal("ambiguous WLAN was relabeled")
				}
				for _, sta := range vap["sta_table"].([]map[string]any) {
					if sta["vap"] != vap["name"] {
						t.Fatal("ambiguous nested identity changed")
					}
				}
			}
		})
	}
}
