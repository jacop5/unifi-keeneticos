// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestStartupCLIHelper(t *testing.T) {
	if os.Getenv("UKN_STARTUP_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"ukn"}, os.Args[i+1:]...)
			flag.CommandLine = flag.NewFlagSet("ukn", flag.ExitOnError)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestDaemonStartupRecoversBeforeCollectionAndAdoption(t *testing.T) {
	for _, mode := range []string{"telemetry-fail", "adoption-fail", "recovery-fail", "state-locked", "probe"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			state := filepath.Join(dir, "state")
			if err := os.Mkdir(state, 0700); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(state, "rollback.json")
			if err := keenetic.SaveUndo(journal, []keenetic.Change{{Undo: keenetic.Command{Text: "restore-fixture"}}}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			adoption := []byte("{invalid-adoption-fixture")
			if err := os.WriteFile(filepath.Join(state, "state.json"), adoption, 0600); err != nil {
				t.Fatal(err)
			}
			events := filepath.Join(dir, "events")
			fake := `#!/bin/sh
printf '%s\n' "$2" >> "$UKN_STARTUP_EVENTS"
if [ "$2" = restore-fixture ]; then
 [ "$UKN_STARTUP_MODE" != recovery-fail ] || exit 5
 exit 0
fi
[ "$UKN_STARTUP_MODE" != telemetry-fail ] || exit 6
case "$2" in
 'show interface Bridge0') printf 'mac: 02:00:00:00:00:01\naddress: 192.0.2.1\n';;
 'show interface WifiMaster0') printf 'id: WifiMaster0\n';;
 'show interface WifiMaster1') printf 'id: WifiMaster1\n';;
 show*) :;;
 *) exit 77;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte(fake), 0700); err != nil {
				t.Fatal(err)
			}
			if mode == "state-locked" {
				lock, err := keenetic.LockState(state)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			}
			args := []string{"-test.run=^TestStartupCLIHelper$", "--", "-state", state, "-url", "http://127.0.0.1:1/inform", "-ssh-listen", ""}
			if mode == "probe" {
				args = append(args, "-probe")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], args...)
			cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1", "UKN_STARTUP_MODE="+mode, "UKN_STARTUP_EVENTS="+events, "PATH="+dir+":"+os.Getenv("PATH"))
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatal("startup fixture timed out")
			}
			if (err == nil) != (mode == "probe") {
				t.Fatalf("startup exit: %v %s", err, output)
			}
			b, readErr := os.ReadFile(events)
			var commands []string
			if readErr == nil {
				commands = strings.Split(strings.TrimSpace(string(b)), "\n")
			} else if !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			switch mode {
			case "state-locked":
				if len(commands) != 0 {
					t.Fatalf("locked state invoked native commands: %v", commands)
				}
			case "recovery-fail":
				if !reflect.DeepEqual(commands, []string{"restore-fixture"}) {
					t.Fatalf("failed recovery continued startup: %v", commands)
				}
			case "probe":
				var snapshot keenetic.Snapshot
				if json.Unmarshal(output, &snapshot) != nil || snapshot.Home["address"] != "192.0.2.1" {
					t.Fatalf("read-only probe failed: %s", output)
				}
				if len(commands) == 0 || commands[0] != "show version" || strings.Contains(string(b), "restore-fixture") {
					t.Fatalf("probe performed recovery: %v", commands)
				}
				if _, err := os.Stat(filepath.Join(state, "daemon.lock")); !os.IsNotExist(err) {
					t.Fatal("probe acquired daemon state lock")
				}
			default:
				if len(commands) < 2 || commands[0] != "restore-fixture" || commands[1] != "show version" {
					t.Fatalf("recovery did not precede collection: %v", commands)
				}
				if mode == "adoption-fail" && !strings.Contains(string(output), "invalid character") {
					t.Fatalf("did not reach adoption-state validation: %s", output)
				}
			}
			retained, journalErr := os.ReadFile(journal)
			switch mode {
			case "probe", "state-locked":
				if journalErr != nil || string(retained) != string(before) {
					t.Fatal("unrelated startup mode modified recovery journal")
				}
			case "recovery-fail":
				if journalErr != nil {
					t.Fatal("failed recovery lost journal")
				}
			default:
				if !os.IsNotExist(journalErr) {
					t.Fatal("successful startup recovery left journal")
				}
			}
			got, err := os.ReadFile(filepath.Join(state, "state.json"))
			if err != nil || string(got) != string(adoption) {
				t.Fatal("startup rewrote adoption state")
			}
		})
	}
}
