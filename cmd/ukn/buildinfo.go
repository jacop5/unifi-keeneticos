// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"runtime"
	"runtime/debug"
)

type buildReport struct {
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	MIPS      string `json:"gomips,omitempty"`
	Revision  string `json:"revision"`
	Modified  string `json:"modified"`
	VCSTime   string `json:"vcs_time,omitempty"`
}

func buildInformation(info *debug.BuildInfo) buildReport {
	report := buildReport{GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, Revision: "unknown", Modified: "unknown"}
	if info == nil {
		return report
	}
	if info.GoVersion != "" {
		report.GoVersion = info.GoVersion
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if setting.Value != "" {
				report.Revision = setting.Value
			}
		case "vcs.modified":
			if setting.Value == "true" || setting.Value == "false" {
				report.Modified = setting.Value
			}
		case "vcs.time":
			report.VCSTime = setting.Value
		case "GOMIPS":
			report.MIPS = setting.Value
		}
	}
	return report
}
