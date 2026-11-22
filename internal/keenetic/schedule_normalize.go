// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var scheduleWindowKey = regexp.MustCompile(`^(wireless\.[^.]+\.schedule_(?:sun|mon|tue|wed|thu|fri|sat))(?:\.([0-9]+))?$`)

// normalizeScheduleWindows preserves the controller's repeated unindexed
// legacy keys before ParseCfg's map can discard all but the last window.
// Modern indexed keys remain unchanged; mixing both encodings is rejected.
func normalizeScheduleWindows(text string) (string, error) {
	lines := strings.Split(text, "\n")
	legacyCount := map[string]int{}
	indexed := map[string]bool{}
	seen := map[string]bool{}
	for i, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		match := scheduleWindowKey.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		base := match[1]
		if match[2] != "" {
			if legacyCount[base] != 0 || seen[key] {
				return "", fmt.Errorf("mixed or duplicate indexed WLAN schedule windows")
			}
			indexed[base], seen[key] = true, true
			continue
		}
		if indexed[base] || legacyCount[base] >= 32 {
			return "", fmt.Errorf("mixed or excessive legacy WLAN schedule windows")
		}
		lines[i] = base + "." + strconv.Itoa(legacyCount[base]) + "=" + value
		legacyCount[base]++
	}
	return strings.Join(lines, "\n"), nil
}
