// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import "testing"

func TestBridgeMembershipUsesAliases(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface Bridge0 include WifiMaster0/AccessPoint0"}}}
	running := "interface WifiMaster0/AccessPoint0\n    rename AccessPoint\n    up\n!\ninterface Bridge0\n    include AccessPoint\n!\n"
	c, err := Preimage(p, running)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 0 {
		t.Fatal("existing membership changed", c)
	}
	c, err = Preimage(p, "interface WifiMaster0/AccessPoint0\n    down\n!\ninterface Bridge0\n!\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Undo.Text != "interface Bridge0 no include WifiMaster0/AccessPoint0" {
		t.Fatal(c)
	}
	if _, err := Preimage(p, running+"interface Bridge1\n    include AccessPoint\n!\n"); err == nil {
		t.Fatal("reassigned AP owned by another bridge")
	}
}

func TestPreimageRestoresOnlyTouchedSettings(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0 channel 6"}, {Text: "interface WifiMaster0 channel width 20"}, {Text: "interface WifiMaster0/AccessPoint0 ssid \"New\""}}}
	running := "interface WifiMaster0\n    channel width 40-below\n    up\n!\ninterface WifiMaster0/AccessPoint0\n    ssid Old\n    up\n!\ninterface PPPoE0\n    description WAN\n!\n"
	c, err := Preimage(p, running)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 3 || c[0].Undo.Text != "interface WifiMaster0 no channel" || c[1].Undo.Text != "interface WifiMaster0 channel width 40-below" || c[2].Undo.Text != "interface WifiMaster0/AccessPoint0 ssid Old" {
		t.Fatalf("%#v", c)
	}
}
func TestPreimageRefusesMissingInterfacesAndUnsupported(t *testing.T) {
	if _, err := Preimage(Plan{Commands: []Command{{Text: "interface WifiMaster0 channel 6"}}}, ""); err == nil {
		t.Fatal("missing interface accepted")
	}
	if _, err := Preimage(Plan{Unsupported: []string{"guest"}}, ""); err == nil {
		t.Fatal("unsupported plan accepted")
	}
}

func TestPreimageRestoresSecurityFlags(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0/AccessPoint0 no encryption wpa3"}}}
	c, err := Preimage(p, "interface WifiMaster0/AccessPoint0\n    encryption wpa3\n!\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Undo.Text != "interface WifiMaster0/AccessPoint0 encryption wpa3" {
		t.Fatalf("%#v", c)
	}
}

func TestPreimageRestoresAdministrativeState(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0/AccessPoint0 up"}}}
	c, err := Preimage(p, "interface WifiMaster0/AccessPoint0\n    down\n!\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Undo.Text != "interface WifiMaster0/AccessPoint0 down" {
		t.Fatalf("%#v", c)
	}
	if _, err := Preimage(p, "interface WifiMaster0/AccessPoint0\n!\n"); err == nil {
		t.Fatal("missing administrative state accepted")
	}
}
