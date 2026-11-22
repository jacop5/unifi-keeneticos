// SPDX-License-Identifier: GPL-2.0-or-later
package device

import (
	"testing"
	"time"
)

func TestRebootFormsPreservePhysicalBootClock(t *testing.T) {
	for _, physical := range []bool{false, true} {
		for _, reply := range []string{`{"_type":"reboot"}`, `{"_type":"cmd","cmd":"reboot"}`, `{"_type":"cmd","cmd":"restart"}`} {
			boot := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
			now := boot.Add(time.Hour)
			x := &Session{S: NewState("http://192.0.2.1/inform"), Boot: boot, PhysicalDevice: physical}
			outcome, err := x.Handle([]byte(reply), now)
			if err != nil || len(outcome.Actions) != 1 || outcome.Actions[0] != "reboot" {
				t.Fatalf("reboot request was not dispatched: %v", err)
			}
			want := now
			if physical {
				want = boot
			}
			if !x.Boot.Equal(want) {
				t.Fatalf("physical=%t reboot form %s moved boot clock incorrectly", physical, reply)
			}
		}
	}
}
