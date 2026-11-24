// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestProvisionReportSeparatesReceiptConfirmationAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provision-status.json")
	initial, err := readProvisionStatus(path)
	if err != nil || initial.Phase != "unknown" || initial.LastConfirmedAt != nil {
		t.Fatal("missing report fabricated confirmation")
	}
	r := provisionReporter{path: path, report: initial, apply: true, testOnly: true}
	plan := keenetic.Plan{Commands: []keenetic.Command{{Text: "credential-do-not-store", Secret: true}}, Ignored: []string{"wireless.3.dtim_period"}}
	for _, phase := range []string{"starting", "log-only", "not-adopted", "plan-rejected", "ownership-rejected", "applying", "apply-failed"} {
		if err := r.record("received-1", phase, &plan, time.Time{}); err != nil {
			t.Fatal(err)
		}
		got, err := readProvisionStatus(path)
		if err != nil || got.LastConfirmedAt != nil || got.LastConfirmedVersion != "" {
			t.Fatalf("%s became confirmed", phase)
		}
	}
	if err := r.record("confirmed-1", "confirmed", &plan, time.Time{}); err != nil {
		t.Fatal(err)
	}
	confirmed := r.report.LastConfirmedAt
	plan.Unsupported = []string{"wireless.3.security"}
	retry := time.Now().Add(time.Minute)
	if err := r.record("received-2", "apply-failed", &plan, retry); err != nil {
		t.Fatal(err)
	}
	got, err := readProvisionStatus(path)
	if err != nil || got.ReceivedVersion != "received-2" || got.LastConfirmedVersion != "confirmed-1" || !got.LastConfirmedAt.Equal(*confirmed) || got.RetryAt == nil {
		t.Fatalf("history lost: %+v %v", got, err)
	}
	restarted := provisionReporter{path: path, report: got}
	if err := restarted.record("received-3", "log-only", &plan, time.Time{}); err != nil {
		t.Fatal(err)
	}
	got, err = readProvisionStatus(path)
	if err != nil || got.ApplyEnabled || got.RetryAt != nil || got.LastConfirmedVersion != "confirmed-1" {
		t.Fatal("restart/log-only state incorrect")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "credential-do-not-store") {
		t.Fatal("report leaked raw command")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("report not private")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".provision-status-*"))
	if len(leftovers) != 0 {
		t.Fatal("temporary report left behind")
	}
}

func TestProvisionStatusCLIHelper(t *testing.T) {
	if os.Getenv("UKN_STATUS_HELPER") != "1" {
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

func TestProvisionStatusCLIIsReadOnlyAndRejectsActions(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "native-called")
	if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte("#!/bin/sh\nprintf native > \"$UKN_NATIVE_MARKER\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r := provisionReporter{path: filepath.Join(dir, "provision-status.json"), report: emptyProvisionStatus()}
	if err := r.record("v1", "log-only", nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	for _, conflict := range []string{"", "-apply", "-probe", "-models", "-build-info", "-rollback-probe", "-plan"} {
		args := []string{"-test.run=^TestProvisionStatusCLIHelper$", "--", "-state", dir, "-provision-status"}
		if conflict != "" {
			args = append(args, conflict)
			if conflict == "-plan" || conflict == "-rollback-probe" {
				args = append(args, "unused")
			}
		}
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "UKN_STATUS_HELPER=1", "UKN_NATIVE_MARKER="+marker, "PATH="+dir+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if (err == nil) != (conflict == "") {
			t.Fatalf("conflict %q: %v %s", conflict, err, out)
		}
		if conflict == "" {
			var got provisionStatus
			if json.Unmarshal(out, &got) != nil || got.Phase != "log-only" {
				t.Fatal("status query failed")
			}
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("status query invoked native runner")
		}
	}
}

func TestProvisionReportCannotConfirmDisabledOrUnsupportedApply(t *testing.T) {
	for _, tc := range []struct {
		apply bool
		plan  *keenetic.Plan
	}{
		{false, &keenetic.Plan{}}, {true, nil}, {true, &keenetic.Plan{Unsupported: []string{"security"}}},
	} {
		path := filepath.Join(t.TempDir(), "provision-status.json")
		r := provisionReporter{path: path, report: emptyProvisionStatus(), apply: tc.apply}
		if r.record("v1", "confirmed", tc.plan, time.Time{}) == nil {
			t.Fatal("invalid confirmation accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid confirmation wrote a report")
		}
	}
}
