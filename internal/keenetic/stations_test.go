// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import "testing"

func TestStationIdentityValidationAndCounts(t *testing.T) {
	s := &Snapshot{Radios: []map[string]string{{"id": "WifiMaster0"}, {"id": "WifiMaster1"}}, VAPs: []map[string]string{{"id": "WifiMaster0/AccessPoint6", "ssid": "Example"}}, Stations: []map[string]string{
		{"mac": "02:AA:00:00:00:01", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes", "txbytes": "10"},
		{"mac": "02:aa:00:00:00:01", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes", "txbytes": "10"},
		{"mac": "malformed", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes"},
		{"mac": "01:00:5e:00:00:01", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes"},
		{"mac": "00:00:00:00:00:00", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes"},
		{"mac": "02:00:00:00:00:02", "ap": "WifiMaster2/AccessPoint0", "authenticated": "yes"},
		{"mac": "02:00:00:00:00:03", "ap": "WifiMaster0/AccessPoint6", "authenticated": "no"},
	}}
	f := s.Fields()
	rows := f["sta_table"].([]map[string]any)
	vap := f["vap_table"].([]map[string]any)[0]
	radios := f["radio_table_stats"].([]map[string]any)
	if len(rows) != 1 || rows[0]["mac"] != "02:aa:00:00:00:01" || f["num_sta"] != 1 || vap["num_sta"] != 1 || radios[0]["num_sta"] != 1 || radios[1]["num_sta"] != 0 || vap["tx_bytes"] != int64(10) {
		t.Fatalf("inconsistent validated counts: %v", f)
	}
	if s.Stations[0]["mac"] != "02:AA:00:00:00:01" {
		t.Fatal("reporting mutated native sample")
	}
}

func TestAmbiguousRoamingStationIsNotAssignedToArbitraryAP(t *testing.T) {
	s := &Snapshot{Stations: []map[string]string{
		{"mac": "02:00:00:00:00:01", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes"},
		{"mac": "02:00:00:00:00:01", "ap": "WifiMaster1/AccessPoint6", "authenticated": "yes"},
	}}
	if len(s.StationFields()) != 0 {
		t.Fatal("ambiguous client duplicated or assigned arbitrary band")
	}
	s.Stations = s.Stations[:1]
	if len(s.StationFields()) != 1 {
		t.Fatal("resolved client did not return")
	}
}
