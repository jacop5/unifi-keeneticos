// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseMappingJSONAcceptsValidObjects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty object", `{}`, map[string]string{}},
		{"simple", `{"1":"Bridge1","2":"Bridge2"}`, map[string]string{"1": "Bridge1", "2": "Bridge2"}},
		{"whitespace", "{\n  \"a\" : \"b\" \n}", map[string]string{"a": "b"}},
		{"escaped key decodes once", `{"\u0061":"b"}`, map[string]string{"a": "b"}},
		{"empty string value", `{"a":""}`, map[string]string{"a": ""}},
		{"unicode value", `{"a":"caf\u00e9"}`, map[string]string{"a": "café"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMappingJSON([]byte(tc.in))
			if err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
			if got == nil {
				t.Fatal("returned nil map")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestParseMappingJSONRejectsAmbiguousOrInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantSub string
	}{
		{"null", `null`, "must be an object"},
		{"array", `["a"]`, "must be an object"},
		{"empty array", `[]`, "must be an object"},
		{"string scalar", `"x"`, "must be an object"},
		{"number scalar", `42`, "must be an object"},
		{"bool scalar", `true`, "must be an object"},
		{"empty input", ``, "malformed"},
		{"duplicate key", `{"a":"1","a":"2"}`, "duplicate key"},
		{"escaped duplicate key", `{"a":"1","\u0061":"2"}`, "duplicate key"},
		{"unicode escaped duplicate", `{"k-é":"1","k-\u00e9":"2"}`, "duplicate key"},
		{"null value", `{"a":null}`, "values must be strings"},
		{"number value", `{"a":1}`, "values must be strings"},
		{"bool value", `{"a":true}`, "values must be strings"},
		{"nested object value", `{"a":{"b":"c"}}`, "values must be strings"},
		{"nested array value", `{"a":["b"]}`, "values must be strings"},
		{"truncated", `{"a":`, "malformed"},
		{"unquoted key", `{a:"1"}`, "malformed"},
		{"trailing comma", `{"a":"1",}`, "malformed"},
		{"trailing object", `{"a":"1"} {}`, "trailing data"},
		{"trailing scalar", `{"a":"1"} 42`, "trailing data"},
		{"trailing garbage", `{"a":"1"} x`, "trailing data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMappingJSON([]byte(tc.in))
			if err == nil {
				t.Fatalf("input accepted: %v", got)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q missing %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestParseMappingJSONErrorsDoNotExposeInput(t *testing.T) {
	const key = "SECRET-KEY-SSID-7f3a"
	const val = "SECRET-VALUE-9b1c"
	inputs := []string{
		`{"` + key + `":"` + val + `","` + key + `":"` + val + `"}`,
		`{"` + key + `":"` + val + `","\u0053ECRET-KEY-SSID-7f3a":"x"}`,
		`{"` + key + `":` + val + `}`,
		`{"` + key + `":null}`,
		`{"` + key + `":`,
		`{"` + key + `":"` + val + `"} ` + val,
		`null`,
	}
	for _, in := range inputs {
		_, err := parseMappingJSON([]byte(in))
		if err == nil {
			t.Fatalf("input accepted: %s", in)
		}
		if strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("error leaks input: %q for %s", err.Error(), in)
		}
	}
}

func TestLoadMappingFilePreservesReadErrorsAndRedactsDecodeErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	if _, err := loadMappingFile(missing, "-slots"); err == nil || !os.IsNotExist(err) {
		t.Fatalf("file-read error not preserved: %v", err)
	}
	valid := filepath.Join(dir, "valid.json")
	if err := os.WriteFile(valid, []byte(`{"3":"WifiMaster0/AccessPoint6"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadMappingFile(valid, "-slots")
	if err != nil || !reflect.DeepEqual(got, map[string]string{"3": "WifiMaster0/AccessPoint6"}) {
		t.Fatalf("valid file: %v %v", got, err)
	}
	dup := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(dup, []byte(`{"SECRET-KEY":"SECRET-VALUE","\u0053ECRET-KEY":"x"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMappingFile(dup, "-experimental-ft-domains"); err == nil ||
		!strings.Contains(err.Error(), "-experimental-ft-domains") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("decode error not redacted or missing flag name: %v", err)
	}
}

// TestOfflinePlanMappingFlagsRejectAmbiguousJSON exercises the real CLI
// offline -plan path through TestStartupCLIHelper. The fake ndmc sentinel in
// PATH proves no native command runs.
func TestOfflinePlanMappingFlagsRejectAmbiguousJSON(t *testing.T) {
	dir := t.TempDir()
	cfgData, err := os.ReadFile("../../internal/keenetic/testdata/compatible-wpa2.cfg")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"controller.cfg":    string(cfgData),
		"ndmc":              "#!/bin/sh\ntouch \"$UKN_MAPPING_NATIVE_TOUCHED\"\nexit 99\n",
		"state":             "deliberately not a state directory",
		"slots.json":        `{"3":"WifiMaster0/AccessPoint6","4":"WifiMaster1/AccessPoint6"}`,
		"slots-empty.json":  `{}`,
		"networks.json":     `{"10":"Bridge1"}`,
		"ft.json":           `{"UKN-Test-Compatible":"ab"}`,
		"slots-dup.json":    `{"3":"SECRET-SLOT-A","\u0033":"SECRET-SLOT-B"}`,
		"networks-dup.json": `{"10":"SECRET-BRIDGE-A","\u0031\u0030":"SECRET-BRIDGE-B"}`,
		"ft-dup.json":       `{"SecretHomeSSID-a17f":"ab","SecretHomeSSID-\u006117f":"cd"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(dir, "native-touched")
	run := func(t *testing.T, extra ...string) (string, error) {
		t.Helper()
		args := append([]string{"-test.run=^TestStartupCLIHelper$", "--", "-state", filepath.Join(dir, "state")},
			"-plan", filepath.Join(dir, "controller.cfg"))
		args = append(args, extra...)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], args...)
		cmd.Env = append(os.Environ(), "UKN_STARTUP_HELPER=1",
			"UKN_MAPPING_NATIVE_TOUCHED="+sentinel, "PATH="+dir+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("CLI helper timed out: %s", out)
		}
		if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
			t.Fatalf("offline -plan invoked a native command: %s", out)
		}
		return string(out), err
	}
	slots := filepath.Join(dir, "slots.json")
	networks := filepath.Join(dir, "networks.json")
	ftDomains := filepath.Join(dir, "ft.json")
	for _, tc := range []struct {
		name      string
		args      []string
		flagName  string
		secretSub string
	}{
		{"duplicate -slots keys", []string{"-slots", filepath.Join(dir, "slots-dup.json")}, "-slots", "SECRET-SLOT"},
		{"duplicate -networks keys", []string{"-slots", slots, "-networks", filepath.Join(dir, "networks-dup.json")}, "-networks", "SECRET-BRIDGE"},
		{"duplicate -experimental-ft-domains keys", []string{"-slots", slots, "-experimental-ft-domains", filepath.Join(dir, "ft-dup.json")}, "-experimental-ft-domains", "SecretHomeSSID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("ambiguous mapping accepted: %s", out)
			}
			if !strings.Contains(out, tc.flagName) || !strings.Contains(out, "duplicate key") {
				t.Fatalf("unexpected rejection output: %s", out)
			}
			if strings.Contains(out, tc.secretSub) {
				t.Fatalf("rejection output exposes mapping input: %s", out)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		args    []string
		wantOut string
	}{
		{"valid all three mappings", []string{"-slots", slots, "-networks", networks, "-experimental-ft-domains", ftDomains}, "interface WifiMaster0/AccessPoint6"},
		{"valid slots only", []string{"-slots", slots}, "interface WifiMaster0/AccessPoint6"},
		{"valid empty slots object", []string{"-slots", filepath.Join(dir, "slots-empty.json")}, "unsupported: wireless.3.slot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, tc.args...)
			if err != nil {
				t.Fatalf("valid mapping rejected: %v %s", err, out)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Fatalf("plan output missing %q: %s", tc.wantOut, out)
			}
		})
	}
}
