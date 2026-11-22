// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"testing"
)

func TestFullCollection(t *testing.T) {
	responses := map[string]string{
		"show version":               "hw_id: KN-1810\n",
		"show system":                "hostname: Example\nuptime: 123\n",
		"show interface Bridge0":     "id: Bridge0\naddress: 192.0.2.1\nmac: 02:00:00:00:00:01\nipv6:\n address: fe80::1\n",
		"show interface WifiMaster0": "id: WifiMaster0\nchannel: 6\n",
		"show interface WifiMaster1": "id: WifiMaster1\nchannel: 36\n",
		"show associations":          "station:\n mac: 02:00:00:00:00:02\n authenticated: yes\n ap: WifiMaster0/AccessPoint0\n",
		"show ip arp":                "Example 192.0.2.2 02:00:00:00:00:02 Home\n",
		"show interface":             "Interface, name = \"AP\"\n id: WifiMaster0/AccessPoint0\n type: AccessPoint\n state: up\n ssid: Example\n",
	}
	responses["show interface"] += "Interface, name = \"1\"\n id: GigabitEthernet0/0\n type: Port\n link: up\n speed: 1000\n duplex: full\n"
	responses["show ip dhcp bindings"] = "lease:\n mac: 02:00:00:00:00:02\n ip: 192.0.2.2\n hostname: Example-Client\n expires: 60\n"
	run := func(_ context.Context, c string) ([]byte, error) {
		v, ok := responses[c]
		if !ok {
			return nil, fmt.Errorf("unexpected query")
		}
		return []byte(v), nil
	}
	s, err := Collect(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Ports) != 1 || len(s.VAPs) != 1 || s.Home["address"] != "192.0.2.1" || s.StationFields()[0]["ip"] != "192.0.2.2" {
		t.Fatalf("%#v", s)
	}
	if s.StationFields()[0]["hostname"] != "Example-Client" {
		t.Fatal("DHCP hostname missing from collected client")
	}
	delete(responses, "show ip dhcp bindings")
	s, err = Collect(context.Background(), run)
	if err != nil || len(s.StationFields()) != 1 || len(s.Hostnames) != 0 {
		t.Fatalf("optional DHCP failure broke collection: %v", err)
	}
	responses["show interface Bridge0"] = "mac: 02:00:00:00:00:01\naddress: fe80::1\n"
	if _, err := Collect(context.Background(), run); err == nil {
		t.Fatal("IPv6 accepted as IPv4 management address")
	}
}

func TestNDMErrorCodes(t *testing.T) {
	for _, s := range []string{"Command::Base error[7405600]: no such command", "Core error[0xcffd00d9]: failed"} {
		if !ndmError.MatchString(s) {
			t.Fatalf("missed %q", s)
		}
	}
	if ndmError.MatchString("Network::Interface: setting applied.") {
		t.Fatal("success classified as failure")
	}
}

func TestARPTableNamesWithSpaces(t *testing.T) {
	a := ParseARP("Name IP MAC Interface\nComputer with spaces 192.0.2.10 02:00:00:00:00:01 Home\n 192.0.2.11 02:00:00:00:00:02 Home\ninvalid 999.0.0.1 02:00:00:00:00:03 Home\n")
	if len(a) != 2 || a["02:00:00:00:00:01"] != "192.0.2.10" || a["02:00:00:00:00:02"] != "192.0.2.11" {
		t.Fatal(a)
	}
}

func TestParentFieldsWin(t *testing.T) {
	v := Values("address: 10.9.12.1\nlink: up\nipv6:\n address: fe80::1\nsummary:\n link: running\n")
	if v["address"] != "10.9.12.1" || v["link"] != "up" {
		t.Fatalf("nested fields replaced parent: %#v", v)
	}
}

func TestVAPClientJoin(t *testing.T) {
	s := &Snapshot{Radios: []map[string]string{{"channel": "6", "bandwidth": "40"}, {"channel": "36", "bandwidth": "80"}}, VAPs: []map[string]string{{"id": "WifiMaster1/AccessPoint0", "ssid": "Example", "mac": "02:00:00:00:00:01"}}, Stations: []map[string]string{{"mac": "02:00:00:00:00:02", "ap": "WifiMaster1/AccessPoint0", "authenticated": "yes"}}}
	v := s.VAPFields()
	sta := s.StationFields()
	if len(v) != 1 || v[0]["num_sta"] != 1 || v[0]["channel"] != int64(36) || v[0]["bw"] != int64(80) || sta[0]["essid"] != "Example" || sta[0]["bssid"] != v[0]["bssid"] {
		t.Fatalf("VAP=%#v stations=%#v", v, sta)
	}
}

func TestHardwareTelemetry(t *testing.T) {
	s := &Snapshot{System: map[string]string{"memtotal": "262144", "memfree": "65536", "uptime": "1234", "cpuload": "11"}}
	f := s.Fields()
	stats := f["sys_stats"].(map[string]any)
	if stats["mem_total"] != int64(268435456) || stats["mem_used"] != int64(201326592) || f["uptime"] != int64(1234) {
		t.Fatal(f)
	}
}

func TestMeasuredRadioTelemetry(t *testing.T) {
	s := &Snapshot{Radios: []map[string]string{{"id": "WifiMaster0", "channel": "12", "bandwidth": "40"}, {"id": "WifiMaster1", "channel": "36", "bandwidth": "80"}}, Stations: []map[string]string{{"ap": "WifiMaster1/AccessPoint6", "authenticated": "yes"}, {"ap": "WifiMaster1/AccessPoint6", "authenticated": "no"}}}
	f := s.Fields()
	radios := f["radio_table"].([]map[string]any)
	stats := f["radio_table_stats"].([]map[string]any)
	if radios[0]["ht"] != "40" || radios[1]["ht"] != "80" || radios[1]["channel"] != int64(36) || stats[1]["num_sta"] != 1 || stats[0]["bw"] != int64(40) || stats[1]["bw"] != int64(80) {
		t.Fatalf("radios=%#v stats=%#v", radios, stats)
	}
	for _, row := range radios {
		for _, key := range []string{"nss", "tx_power", "max_txpower", "radio_caps"} {
			if _, exists := row[key]; exists {
				t.Fatalf("unmeasured capability %s advertised", key)
			}
		}
	}
}

func TestVAPContainsControllerClientRows(t *testing.T) {
	s := &Snapshot{VAPs: []map[string]string{{"id": "WifiMaster0/AccessPoint0", "ssid": "Example", "mac": "02:00:00:00:00:01"}}, Stations: []map[string]string{{"ap": "WifiMaster0/AccessPoint0", "mac": "02:00:00:00:00:02", "authenticated": "yes", "txrate": "72.2", "rxrate": "65", "ht": "40", "mode": "11n", "txbytes": "100", "rxbytes": "200"}}}
	v := s.VAPFields()[0]
	clients := v["sta_table"].([]map[string]any)
	if len(clients) != 1 || clients[0]["tx_rate"] != int64(72200) || clients[0]["chwidth"] != 1 || clients[0]["is_11n"] != true || v["tx_bytes"] != int64(100) {
		t.Fatalf("%#v", v)
	}
}

func TestControllerWLANIdentity(t *testing.T) {
	s := &Snapshot{VAPs: []map[string]string{{"id": "WifiMaster0/AccessPoint6", "ssid": "UKN-Test-Hero"}}, Stations: []map[string]string{{"mac": "02:00:00:00:00:01", "ap": "WifiMaster0/AccessPoint6", "authenticated": "yes"}}}
	f := s.FieldsForConfig("wireless.1.ssid=UKN-Test-Hero\nwireless.1.parent=wifi0\nwireless.1.id=2\nwireless.1.devname=ath0\n")
	v := f["vap_table"].([]map[string]any)[0]
	if v["id"] != "2" || v["name"] != "ath0" || f["num_sta"] != 1 || f["sta_table"].([]map[string]any)[0]["vap"] != "ath0" {
		t.Fatal(f)
	}
}

func TestStations(t *testing.T) {
	s := &Snapshot{Stations: Records("\x1b[K\r\n station: \r\n mac: 02:00:00:00:00:01\r\n ap: WifiMaster1/AccessPoint0\r\n authenticated: yes\r\n rssi: -63\r\n txrate: 702\r\n rxrate: 78\r\n txbytes: 4294967297\r\n station:\r\n mac: 02:00:00:00:00:02\r\n authenticated: no\r\n", "station")}
	if len(s.Stations) != 2 {
		t.Fatalf("records: %#v", s.Stations)
	}
	f := s.StationFields()
	if len(f) != 1 || f[0]["tx_rate"] != int64(702000) || f[0]["signal"] != int64(-63) || f[0]["tx_bytes"] != int64(4294967297) || f[0]["radio"] != "na" {
		t.Fatalf("fields: %#v", f)
	}
}

func TestStationMeasuredPowerManagement(t *testing.T) {
	for _, state := range []string{"yes", "no", "", "unknown"} {
		s := &Snapshot{Stations: []map[string]string{{"mac": "02:00:00:00:00:01", "authenticated": "yes", "ap": "WifiMaster0/AccessPoint0", "mode": "11n", "psm": state}}}
		row := s.StationFields()[0]
		got, exists := row["state_pwrmgt"]
		if exists != (state == "yes" || state == "no") {
			t.Fatalf("invented power state for %q", state)
		}
		if exists && got != (state == "yes") {
			t.Fatalf("incorrect power state for %q", state)
		}
		if row["state_ht"] != true {
			t.Fatal("native HT state missing")
		}
	}
}

func TestStationRatesRejectUnavailableAndNonFiniteMeasurements(t *testing.T) {
	for _, rate := range []string{"", "unknown", "NaN", "+Inf", "-Inf", "-1", "1e30", "1e300"} {
		s := &Snapshot{Stations: []map[string]string{{"mac": "02:00:00:00:00:01", "authenticated": "yes", "ap": "WifiMaster0/AccessPoint0", "txrate": rate, "rxrate": rate}}}
		for _, key := range []string{"tx_rate", "rx_rate"} {
			if _, exists := s.StationFields()[0][key]; exists {
				t.Fatalf("invalid rate %q emitted as %s", rate, key)
			}
		}
	}
	for _, sample := range []struct {
		native string
		want   int64
	}{{"0", 0}, {"72.2", 72200}, {"0.5", 500}, {"2402", 2402000}} {
		s := &Snapshot{Stations: []map[string]string{{"mac": "02:00:00:00:00:01", "authenticated": "yes", "ap": "WifiMaster0/AccessPoint0", "txrate": sample.native, "rxrate": sample.native}}}
		row := s.StationFields()[0]
		if row["tx_rate"] != sample.want || row["rx_rate"] != sample.want {
			t.Fatalf("incorrect Mbps to Kbps conversion: %v", row)
		}
	}
}
