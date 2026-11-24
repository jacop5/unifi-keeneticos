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

// ActionPolicy keeps disruptive device actions independent of WLAN apply.
type ActionPolicy struct {
	LED          bool
	Reboot       bool
	WLANTestOnly bool
}

// ControllerAction accepts only explicit supported operations. A successful
// reboot dispatch schedules a physical restart; it does not save configuration
// or assert that the restart has occurred.
func ControllerAction(ctx context.Context, run Runner, policy ActionPolicy, action string) error {
	switch action {
	case "led:on", "led:off":
		if !policy.LED {
			return fmt.Errorf("LED control is disabled")
		}
		return LEDAction(ctx, run, action)
	case "reboot":
		if !policy.Reboot || policy.WLANTestOnly {
			return fmt.Errorf("native reboot control is disabled for this scope")
		}
		_, err := run(ctx, "system reboot 5")
		return err
	default:
		return fmt.Errorf("unsupported controller action %q", action)
	}
}
