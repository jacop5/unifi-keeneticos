// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func nativeActionHandler(run keenetic.Runner, policy keenetic.ActionPolicy, journal string, onReboot func()) func(context.Context, string) error {
	return func(ctx context.Context, action string) error {
		if action == "reboot" && policy.Reboot && !policy.WLANTestOnly {
			// A reboot would interrupt the guarded transaction and its recovery path.
			if _, err := os.Stat(journal); !os.IsNotExist(err) {
				return fmt.Errorf("reboot refused: rollback state is pending or unreadable")
			}
		}
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := keenetic.ControllerAction(c, run, policy, action); err != nil {
			return err
		}
		if action == "reboot" && onReboot != nil {
			onReboot()
		}
		return nil
	}
}
