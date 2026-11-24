// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"strconv"
	"strings"
)

// Map only the channel width, not the PHY generation. Regulatory/hardware
// availability remains a native-firmware decision. No radio is inferred.
func nativeRadioWidth(mode, master, channel string) (string, bool) {
	width, family := "", ""
	for _, prefix := range []string{"11nght", "11naht", "11acvht"} {
		if strings.HasPrefix(mode, prefix) {
			width, family = strings.TrimPrefix(mode, prefix), prefix
			break
		}
	}
	if width != "20" && width != "40" && width != "80" && width != "160" {
		return "", false
	}
	if master == "WifiMaster0" {
		if family != "11nght" || (width != "20" && width != "40") {
			return "", false
		}
	} else if master == "WifiMaster1" {
		if family != "11naht" && family != "11acvht" {
			return "", false
		}
	} else {
		return "", false
	}
	if width == "20" || width == "160" {
		return width, true
	}
	side := "above"
	n, _ := strconv.Atoi(channel)
	if master == "WifiMaster0" {
		if n > 7 {
			side = "below"
		}
	} else if channel != "" && channel != "auto" && n != 0 {
		// 5 GHz 40 MHz operating-class pairs, including the shifted upper band.
		switch n {
		case 36, 44, 52, 60, 100, 108, 116, 124, 132, 140, 149, 157, 165, 173:
		case 40, 48, 56, 64, 104, 112, 120, 128, 136, 144, 153, 161, 169, 177:
			side = "below"
		default:
			return "", false
		}
	}
	native := "40-" + side
	if width == "80" {
		native += "/80"
	}
	return native, true
}
