// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"runtime"
	"runtime/debug"
	"testing"
)

func TestBuildInformationPreservesUnknownProvenance(t *testing.T) {
	r := buildInformation(nil)
	if r.Revision != "unknown" || r.Modified != "unknown" || r.Arch != runtime.GOARCH || r.OS != runtime.GOOS {
		t.Fatal(r)
	}
	r = buildInformation(&debug.BuildInfo{GoVersion: "go-test", Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}, {Key: "vcs.time", Value: "2026-11-23T00:00:00Z"}, {Key: "GOMIPS", Value: "softfloat"}}})
	if r.GoVersion != "go-test" || r.Revision != "abc123" || r.Modified != "true" || r.MIPS != "softfloat" || r.VCSTime == "" {
		t.Fatal(r)
	}
	r = buildInformation(&debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "invalid"}}})
	if r.Modified != "unknown" {
		t.Fatal("missing provenance fabricated")
	}
}
