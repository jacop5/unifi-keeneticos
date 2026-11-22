// SPDX-License-Identifier: GPL-2.0-or-later

// Package keenetic implements the KeeneticOS NDM backend. NDM output is
// deliberately parsed independently of the OpenWrt ubus/UCI backend.
package keenetic

import (
	"context"
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/translate"
	"math"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Values reads scalar fields from one NDM object. Repeated fields are kept
// separately by Records, rather than flattening all stations into one object.
func Values(text string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(ansi.ReplaceAllString(text, ""), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.TrimSpace(v) != "" {
			// Parent fields precede nested status objects. Preserve the first
			// occurrence so nested IPv6 addresses and summary link states do
			// not overwrite the interface's IPv4 address or physical link.
			key := strings.TrimSpace(k)
			if _, exists := m[key]; !exists {
				m[key] = strings.TrimSpace(v)
			}
		}
	}
	return m
}

func Records(text, marker string) []map[string]string {
	var out []map[string]string
	var lines []string
	flush := func() {
		if len(lines) > 0 {
			out = append(out, Values(strings.Join(lines, "\n")))
		}
		lines = nil
	}
	for _, line := range strings.Split(ansi.ReplaceAllString(text, ""), "\n") {
		if strings.TrimSpace(line) == marker+":" {
			flush()
			continue
		}
		lines = append(lines, line)
	}
	flush()
	var filtered []map[string]string
	for _, r := range out {
		if len(r) > 0 {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// Runner allows identical collection locally on Entware and in fixture tests.
type Runner func(context.Context, string) ([]byte, error)

func Local(ctx context.Context, command string) ([]byte, error) {
	b, err := exec.CommandContext(ctx, "ndmc", "-c", command).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ndmc execution: %w", err)
	}
	if ndmError.Match(b) {
		return nil, fmt.Errorf("ndmc rejected command")
	}
	return b, nil
}

var ndmError = regexp.MustCompile(`(?m)(?:error|critical)\[(?:0x)?[0-9a-fA-F]+\]:`)

type Snapshot struct {
	Version, System, Home map[string]string
	Radios                []map[string]string
	Stations              []map[string]string
	VAPs                  []map[string]string
	Ports                 []map[string]string
	ARP                   map[string]string
	Hostnames             map[string]string
}

func Collect(ctx context.Context, run Runner) (*Snapshot, error) {
	s := &Snapshot{}
	for _, q := range []struct {
		command string
		dst     *map[string]string
	}{
		{"show version", &s.Version}, {"show system", &s.System}, {"show interface Bridge0", &s.Home},
	} {
		b, err := run(ctx, q.command)
		if err != nil {
			return nil, err
		}
		*q.dst = Values(string(b))
	}
	for _, id := range []string{"WifiMaster0", "WifiMaster1"} {
		b, err := run(ctx, "show interface "+id)
		if err != nil {
			return nil, err
		}
		r := Values(string(b))
		if r["id"] != id {
			return nil, fmt.Errorf("missing radio %s", id)
		}
		s.Radios = append(s.Radios, r)
	}
	b, err := run(ctx, "show associations")
	if err != nil {
		return nil, err
	}
	s.Stations = Records(string(b), "station")
	b, err = run(ctx, "show ip arp")
	if err != nil {
		return nil, err
	}
	s.ARP = ParseARP(string(b))
	// DHCP may be unavailable when this device is an AP or uses an external
	// DHCP server. Optional names must not prevent radio/client collection.
	if leases, err := run(ctx, "show ip dhcp bindings"); err == nil {
		s.Hostnames = ParseDHCPHostnames(string(leases))
	}
	b, err = run(ctx, "show interface")
	if err != nil {
		return nil, err
	}
	for _, block := range strings.Split(string(b), "Interface, name = ") {
		v := Values(block)
		if v["type"] == "Port" && nativeEthernetPort.MatchString(v["id"]) {
			s.Ports = append(s.Ports, v)
		}
		if v["type"] == "AccessPoint" && v["state"] == "up" && v["ssid"] != "" {
			s.VAPs = append(s.VAPs, v)
		}
	}
	mac, macErr := net.ParseMAC(s.Home["mac"])
	if macErr != nil || len(mac) != 6 || net.ParseIP(s.Home["address"]).To4() == nil {
		return nil, fmt.Errorf("Bridge0 has no management MAC/IP")
	}
	return s, nil
}

func number(v string) int64 { n, _ := strconv.ParseInt(v, 10, 64); return n }

// ParseARP locates adjacent IP/MAC columns rather than splitting on hostnames,
// which can contain spaces or be absent in NDM's human-readable table.
func ParseARP(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(ansi.ReplaceAllString(text, ""), "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			ip := net.ParseIP(fields[i])
			mac, err := net.ParseMAC(fields[i+1])
			if ip != nil && ip.To4() != nil && err == nil && len(mac) == 6 {
				out[strings.ToLower(mac.String())] = ip.String()
				break
			}
		}
	}
	return out
}

// StationFields converts NDM rates (Mbps) to the inform protocol's Kbps.
func (s *Snapshot) StationFields() []map[string]any {
	out := []map[string]any{}
	for _, r := range s.Stations {
		if r["mac"] == "" || r["authenticated"] != "yes" {
			continue
		}
		band := "ng"
		name := "wifi0"
		if strings.HasPrefix(r["ap"], "WifiMaster1/") {
			band = "na"
			name = "wifi1"
		}
		out = append(out, map[string]any{"mac": strings.ToLower(r["mac"]), "radio": band, "radio_name": name, "vap": r["ap"], "signal": number(r["rssi"]), "tx_bytes": number(r["txbytes"]), "rx_bytes": number(r["rxbytes"]), "uptime": number(r["uptime"]), "authorized": true})
		row := out[len(out)-1]
		if hostname := s.Hostnames[strings.ToLower(r["mac"])]; hostname != "" {
			row["hostname"] = hostname
		}
		if ip := s.ARP[strings.ToLower(r["mac"])]; ip != "" {
			row["ip"] = ip
		}
		for _, direction := range []string{"tx", "rx"} {
			rate, err := strconv.ParseFloat(r[direction+"rate"], 64)
			// Omit unavailable or invalid measurements rather than converting
			// NaN/overflow to a negative link rate.
			if err == nil && !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= 0 && rate*1000 < float64(math.MaxInt64) {
				row[direction+"_rate"] = int64(rate * 1000)
			}
		}
		row["state"] = 15
		row["is_guest"] = false
		row["is_11n"] = r["mode"] == "11n" || r["mode"] == "11ac" || r["mode"] == "11ax"
		row["state_ht"] = row["is_11n"]
		if r["psm"] == "yes" || r["psm"] == "no" {
			row["state_pwrmgt"] = r["psm"] == "yes"
		}
		row["is_11ac"] = r["mode"] == "11ac"
		row["is_11ax"] = r["mode"] == "11ax"
		if width, ok := map[string]int{"20": 0, "40": 1, "80": 2, "160": 3}[r["ht"]]; ok {
			row["chwidth"] = width
		}
		index := 0
		if band == "na" {
			index = 1
		}
		if index < len(s.Radios) {
			row["channel"] = number(s.Radios[index]["channel"])
		}
		for _, v := range s.VAPs {
			if v["id"] == r["ap"] {
				out[len(out)-1]["essid"] = v["ssid"]
				out[len(out)-1]["bssid"] = v["mac"]
			}
		}
	}
	return out
}

func (s *Snapshot) VAPFields() []map[string]any {
	out := []map[string]any{}
	stations := s.StationFields()
	for _, v := range s.VAPs {
		band, name, index := "ng", "wifi0", 0
		if strings.HasPrefix(v["id"], "WifiMaster1/") {
			band, name, index = "na", "wifi1", 1
		}
		channel, width := "", ""
		if index < len(s.Radios) {
			channel = s.Radios[index]["channel"]
			width = s.Radios[index]["bandwidth"]
		}
		count := 0
		for _, sta := range s.Stations {
			if sta["ap"] == v["id"] && sta["authenticated"] == "yes" {
				count++
			}
		}
		clients := []map[string]any{}
		var tx, rx int64
		for _, sta := range stations {
			if sta["vap"] == v["id"] {
				clients = append(clients, sta)
				tx += sta["tx_bytes"].(int64)
				rx += sta["rx_bytes"].(int64)
			}
		}
		out = append(out, map[string]any{"name": v["id"], "radio": band, "radio_name": name, "essid": v["ssid"], "bssid": v["mac"], "channel": number(channel), "bw": number(width), "num_sta": count, "up": true, "is_guest": false, "state": "RUN", "usage": "user", "sta_table": clients, "tx_bytes": tx, "rx_bytes": rx})
	}
	return out
}

func (s *Snapshot) Fields() map[string]any {
	total := number(s.System["memtotal"]) * 1024
	free := number(s.System["memfree"]) * 1024
	stats := map[string]any{"mem_total": total, "mem_used": total - free, "mem_buffer": number(s.System["membuffers"]) * 1024}
	radios := []map[string]any{}
	radioTable := []map[string]any{}
	for i, r := range s.Radios {
		band, name := "ng", "wifi0"
		if i == 1 {
			band, name = "na", "wifi1"
		}
		count := 0
		for _, sta := range s.Stations {
			if strings.HasPrefix(sta["ap"], r["id"]+"/") && sta["authenticated"] == "yes" {
				count++
			}
		}
		row := map[string]any{"name": name, "radio": band, "channel": number(r["channel"])}
		// Publish native measurements without advertising guessed power or
		// antenna capabilities from the emulated device profile.
		if width := r["bandwidth"]; width != "" {
			row["ht"] = width
		}
		radioTable = append(radioTable, row)
		// The controller uses bw for measured operating width; radio_table.ht
		// may retain its desired configuration while this daemon is log-only.
		radios = append(radios, map[string]any{"name": name, "radio": band, "channel": number(r["channel"]), "ht": r["bandwidth"], "bw": number(r["bandwidth"]), "num_sta": count, "user-num_sta": count, "temperature": number(r["temperature"])})
	}
	count := len(s.StationFields())
	return map[string]any{"port_table": s.PortFields(), "num_sta": count, "user-num_sta": count, "stat": map[string]any{"user-num_sta": count}, "sta_table": s.StationFields(), "vap_table": s.VAPFields(), "sys_stats": stats, "system-stats": map[string]any{"cpu": s.System["cpuload"], "mem": fmt.Sprintf("%.1f", 100*float64(total-free)/float64(max(total, 1))), "uptime": number(s.System["uptime"])}, "uptime": number(s.System["uptime"]), "radio_table": radioTable, "radio_table_stats": radios}
}

func (s *Snapshot) FieldsForConfig(text string) map[string]any {
	f := s.Fields()
	cfg := translate.ParseCfg(text)
	for _, vap := range f["vap_table"].([]map[string]any) {
		for key, ssid := range cfg {
			if !strings.HasPrefix(key, "wireless.") || !strings.HasSuffix(key, ".ssid") || ssid != vap["essid"] {
				continue
			}
			prefix := strings.TrimSuffix(key, "ssid")
			if cfg[prefix+"parent"] != vap["radio_name"] {
				continue
			}
			if id := cfg[prefix+"id"]; id != "" {
				vap["id"] = id
			}
			if name := cfg[prefix+"devname"]; name != "" {
				old := vap["name"]
				vap["name"] = name
				for _, sta := range f["sta_table"].([]map[string]any) {
					if sta["vap"] == old {
						sta["vap"] = name
					}
				}
			}
			break
		}
	}
	return f
}
