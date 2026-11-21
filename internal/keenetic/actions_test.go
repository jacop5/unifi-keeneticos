// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"errors"
	"testing"
)

func TestLEDActionAllowlist(t *testing.T) {
	for action, want := range map[string]string{"led:on": "system no led power shutdown", "led:off": "system led power shutdown all"} {
		calls := 0
		err := LEDAction(context.Background(), func(_ context.Context, command string) ([]byte, error) {
			calls++
			if command != want {
				t.Fatalf("got %q want %q", command, want)
			}
			return nil, nil
		}, action)
		if err != nil || calls != 1 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	}
	for _, action := range []string{"reboot", "locate:on", "led:off; reboot", ""} {
		err := LEDAction(context.Background(), func(context.Context, string) ([]byte, error) {
			t.Fatal("unsafe action reached NDM")
			return nil, nil
		}, action)
		if err == nil {
			t.Fatal("unsafe action accepted")
		}
	}
}

func TestLEDActionFailure(t *testing.T) {
	want := errors.New("NDM failed")
	if err := LEDAction(context.Background(), func(context.Context, string) ([]byte, error) { return nil, want }, "led:on"); !errors.Is(err, want) {
		t.Fatal(err)
	}
}
