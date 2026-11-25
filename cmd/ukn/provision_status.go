// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

// This is a local historical diagnostic, not controller protocol state or a
// current-health assertion. Never store raw errors, commands or system_cfg.
type provisionScope struct {
	WLANTestOnly bool     `json:"wlan_test_only"`
	Interfaces   []string `json:"interfaces"`
	Ignored      []string `json:"ignored_settings"`
}

type provisionStatus struct {
	SchemaVersion        int             `json:"schema_version"`
	Phase                string          `json:"phase"`
	ReceivedVersion      string          `json:"received_cfgversion,omitempty"`
	ApplyEnabled         bool            `json:"apply_enabled"`
	WLANTestOnly         bool            `json:"wlan_test_only"`
	Unsupported          []string        `json:"unsupported"`
	Ignored              []string        `json:"ignored_settings"`
	UpdatedAt            *time.Time      `json:"updated_at,omitempty"`
	LastConfirmedVersion string          `json:"last_confirmed_cfgversion,omitempty"`
	LastConfirmedAt      *time.Time      `json:"last_confirmed_at,omitempty"`
	LastConfirmedScope   *provisionScope `json:"last_confirmed_scope,omitempty"`
	RetryAt              *time.Time      `json:"retry_at,omitempty"`
}

func emptyProvisionStatus() provisionStatus {
	return provisionStatus{SchemaVersion: 1, Phase: "unknown", Unsupported: []string{}, Ignored: []string{}}
}

func readProvisionStatus(path string) (provisionStatus, error) {
	result := emptyProvisionStatus()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if len(b) > 65536 {
		return result, fmt.Errorf("provision report exceeds size limit")
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return emptyProvisionStatus(), fmt.Errorf("invalid provision report JSON")
	}
	if result.SchemaVersion != 1 {
		return emptyProvisionStatus(), fmt.Errorf("unsupported provision report schema")
	}
	return result, nil
}

type provisionReporter struct {
	path            string
	report          provisionStatus
	apply, testOnly bool
}

func (r *provisionReporter) record(version, phase string, plan *keenetic.Plan, retry time.Time) error {
	switch phase {
	case "starting", "config-unavailable", "config-unverified", "plan-rejected", "unsupported", "log-only", "not-adopted", "ownership-rejected", "applying", "apply-failed", "confirmed":
	default:
		return fmt.Errorf("invalid provision report phase")
	}
	if phase == "confirmed" && (!r.apply || plan == nil || len(plan.Unsupported) != 0) {
		return fmt.Errorf("confirmation requires an enabled supported apply")
	}
	now := time.Now().UTC()
	r.report.SchemaVersion = 1
	r.report.Phase, r.report.ReceivedVersion = phase, version
	r.report.ApplyEnabled, r.report.WLANTestOnly = r.apply, r.testOnly
	r.report.UpdatedAt = &now
	r.report.Unsupported, r.report.Ignored = []string{}, []string{}
	if plan != nil {
		r.report.Unsupported = append(r.report.Unsupported, plan.Unsupported...)
		r.report.Ignored = append(r.report.Ignored, plan.Ignored...)
	}
	r.report.RetryAt = nil
	if !retry.IsZero() {
		t := retry.UTC()
		r.report.RetryAt = &t
	}
	if phase == "confirmed" {
		r.report.LastConfirmedVersion = version
		r.report.LastConfirmedAt = &now
		r.report.LastConfirmedScope = confirmedScope(plan, r.testOnly)
	}
	b, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".provision-status-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, r.path)
}

// Store only native target identifiers, never credentials or SSID values.
func confirmedScope(plan *keenetic.Plan, testOnly bool) *provisionScope {
	targets := map[string]bool{}
	for _, c := range plan.Commands {
		parts := strings.SplitN(c.Text, " ", 3)
		if len(parts) == 3 && parts[0] == "interface" {
			targets[parts[1]] = true
		}
	}
	for _, a := range plan.AccessLists {
		targets[a.Target] = true
	}
	for _, s := range plan.Schedules {
		targets[s.Target] = true
	}
	result := &provisionScope{WLANTestOnly: testOnly, Interfaces: []string{}, Ignored: append([]string{}, plan.Ignored...)}
	for target := range targets {
		result.Interfaces = append(result.Interfaces, target)
	}
	sort.Strings(result.Interfaces)
	return result
}
