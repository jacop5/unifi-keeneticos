// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strings"
)

func validCountrySetting(setting string) bool {
	value := strings.TrimPrefix(setting, "country-code ")
	return setting != value && len(value) == 2 && value[0] >= 'A' && value[0] <= 'Z' && value[1] >= 'A' && value[1] <= 'Z'
}

func VerifyCountryCodes(plan Plan, running string) error {
	blocks := interfaceBlocks(running)
	for _, c := range plan.Commands {
		fields := strings.SplitN(c.Text, " ", 3)
		if len(fields) != 3 || !strings.HasPrefix(fields[2], "country-code ") {
			continue
		}
		found := false
		for _, line := range blocks[fields[1]] {
			found = found || line == fields[2]
		}
		if !found {
			return fmt.Errorf("native radio country does not match plan")
		}
	}
	return nil
}
