// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import "testing"

func TestPhysicalPortMeasurements(t *testing.T) {
	s := &Snapshot{Ports: []map[string]string{
		{"id": "GigabitEthernet1/0", "label": "0", "link": "up", "speed": "1000", "duplex": "full", "auto-negotiation": "on"},
		{"id": "GigabitEthernet0/1", "label": "2", "link": "down", "speed": "1000", "duplex": "full"},
		{"id": "GigabitEthernet0/0", "label": "1", "link": "up", "speed": "100", "duplex": "half", "auto-negotiation": "off"},
		{"id": "GigabitEthernet0/Vlan1", "link": "up", "speed": "1000"},
	}}
	ports := s.PortFields()
	if len(ports) != 3 {
		t.Fatal(ports)
	}
	if ports[0]["port_idx"] != 1 || ports[0]["ifname"] != "GigabitEthernet0/0" || ports[0]["speed"] != int64(100) || ports[0]["full_duplex"] != false || ports[0]["autoneg"] != false {
		t.Fatal(ports[0])
	}
	if ports[1]["speed"] != int64(0) || ports[1]["full_duplex"] != false || ports[1]["up"] != false {
		t.Fatal("down port retained stale negotiated values")
	}
	if ports[2]["port_idx"] != 3 || ports[2]["name"] != "0" || ports[2]["speed"] != int64(1000) || ports[2]["autoneg"] != true {
		t.Fatal(ports[2])
	}
	for _, p := range ports {
		for _, key := range []string{"is_uplink", "poe_caps", "media", "rx_bytes", "tx_bytes", "enable"} {
			if _, ok := p[key]; ok {
				t.Fatalf("invented port field %s", key)
			}
		}
	}
}

func TestPhysicalPortsDoNotGuessUnknownMeasurements(t *testing.T) {
	s := &Snapshot{Ports: []map[string]string{
		{"id": "FastEthernet0/0", "link": "up", "speed": "unknown", "duplex": "unknown", "auto-negotiation": "unknown"},
		{"id": "GigabitEthernet1/0", "link": "unknown"},
	}}
	ports := s.PortFields()
	if len(ports) != 1 || ports[0]["name"] != "FastEthernet0/0" {
		t.Fatal(ports)
	}
	for _, key := range []string{"speed", "full_duplex", "autoneg"} {
		if _, ok := ports[0][key]; ok {
			t.Fatalf("unknown %s was fabricated", key)
		}
	}
}
