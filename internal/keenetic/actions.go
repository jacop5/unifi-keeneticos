// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
)

// LEDAction accepts only the two controller LED operations. In particular,
// locate and reboot must not fall through to a general NDM command runner.
// These commands change runtime configuration only; startup-config is not saved.
func LEDAction(ctx context.Context, run Runner, action string) error {
	var command string
	switch action {
	case "led:on":
		command = "system no led power shutdown"
	case "led:off":
		command = "system led power shutdown all"
	default:
		return fmt.Errorf("unsupported LED action %q", action)
	}
	_, err := run(ctx, command)
	return err
}
