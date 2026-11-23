// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"encoding/json"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"strings"
	"testing"
)

func TestJSONPlanRedactsCredentialCommands(t *testing.T) {
	p := keenetic.Plan{Commands: []keenetic.Command{{Text: "private-password", Secret: true}, {Text: "interface WifiMaster0/AccessPoint6 down"}}, Unsupported: []string{"wireless.1.security"}, AccessLists: []keenetic.AccessList{{Target: "WifiMaster0/AccessPoint6", Policy: "deny", MACs: []string{"02:00:00:00:00:01"}}}}
	n := 2
	report := redactedPlan(p, &n)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-password") || strings.Contains(string(data), "02:00:00:00:00:01") {
		t.Fatal("private plan detail leaked")
	}
	if report["planner_supported"] != false || report["preflight_passed"] != true {
		t.Fatal(report)
	}
	if redactedPlan(keenetic.Plan{}, nil)["preflight_passed"] != false {
		t.Fatal("unperformed preflight claimed")
	}
	if err := validateOfflineMode("requested", "", nil, "-plan-json"); err == nil {
		t.Fatal("JSON mode without plan accepted")
	}
}
