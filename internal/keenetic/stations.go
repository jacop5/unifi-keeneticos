// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import "net"

// stationRecords keeps identity validation and counts consistent across device,
// radio and WLAN reports. Ambiguous cross-AP records are omitted until a later
// collection resolves the roam; no band is guessed for an unknown interface.
func (s *Snapshot) stationRecords() []map[string]string {
	rows := []map[string]string{}
	owners := map[string]string{}
	conflicts := map[string]bool{}
	unavailable := map[string]bool{}
	for _, target := range s.InactiveAPs {
		unavailable[target] = true
	}
	for _, vap := range s.VAPs {
		if !nativeVAPUp(vap) {
			unavailable[vap["id"]] = true
		}
	}
	for _, r := range s.Stations {
		mac, err := net.ParseMAC(r["mac"])
		if unavailable[r["ap"]] || r["authenticated"] != "yes" || err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || !regexpAP.MatchString(r["ap"]) {
			continue
		}
		id := mac.String()
		if ap, exists := owners[id]; exists {
			if ap != r["ap"] {
				conflicts[id] = true
			}
			continue
		}
		owners[id] = r["ap"]
		row := make(map[string]string, len(r))
		for k, v := range r {
			row[k] = v
		}
		row["mac"] = id
		rows = append(rows, row)
	}
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		if !conflicts[r["mac"]] {
			out = append(out, r)
		}
	}
	return out
}
