// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

func TestJSONPlanRedactsCredentialCommands(t *testing.T) {
	p := keenetic.Plan{Commands: []keenetic.Command{{Text: "interface WifiMaster0/AccessPoint6 authentication wpa-psk private-password", Secret: true}, {Text: "interface WifiMaster0/AccessPoint6 down"}}, Unsupported: []string{"wireless.1.security"}, AccessLists: []keenetic.AccessList{{Target: "WifiMaster0/AccessPoint6", Policy: "deny", MACs: []string{"02:00:00:00:00:01"}}}}
	n := 2
	report := redactedPlan(p, &n)
	if report["command_targets"].([]string)[0] != "WifiMaster0/AccessPoint6" {
		t.Fatal("redaction lost safe target metadata")
	}
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

func TestGeneratedPreflightJSONPassesScopeChecker(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 required for cross-tool integration test")
	}
	cfg := `wireless.3.ssid=UKN-Test-Pipeline
wireless.3.parent=wifi0
wireless.3.usage=user
aaa.3.ssid=UKN-Test-Pipeline
aaa.3.wpa=2
aaa.3.wpa.psk=synthetic-pipeline-key
aaa.3.wpa.key.1.mgmt=WPA-PSK
aaa.3.wpa.1.pairwise=CCMP
aaa.3.bss_transition=disabled
`
	slots := map[string]string{"3": "WifiMaster0/AccessPoint6"}
	plan, err := keenetic.BuildWLANTestPlan(cfg, slots)
	if err != nil {
		t.Fatal(err)
	}
	running := "interface Bridge0\n    up\n!\ninterface WifiMaster0/AccessPoint6\n    ssid UKN-Test-Previous\n    down\n!\n"
	changes, err := preflightPlan(plan, running, slots, true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(redactedPlan(plan, &changes))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-pipeline-key") {
		t.Fatal("pipeline leaked a credential")
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	checker, err := filepath.Abs("../../tools/check-test-plan.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"WifiMaster0/AccessPoint6", "WifiMaster0/AccessPoint5"} {
		cmd := exec.Command(python, checker, path, "--ap", target)
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		out, err := cmd.CombinedOutput()
		if (err == nil) != (target == slots["3"]) {
			t.Fatalf("target=%s err=%v output=%s", target, err, out)
		}
	}
}
