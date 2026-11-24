// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const wifiSystemInterfaces = `Interface, name = "AP 2G"
 id: WifiMaster0/AccessPoint0
 state: up
Interface, name = "AP_5G"
 id: WifiMaster1/AccessPoint0
 state: up
Interface, name = "Unused_Test"
 id: WifiMaster0/AccessPoint6
 state: down
`

const wifiSystemStatus = `
 wlan, id = Home:
 disabled-by-schedule: no
       band, id = 1:
       enabled: yes
           access-point, name = AP_5G:
             wps:
               enabled: no
               self-pin: 12345670
       band, id = 0:
       enabled: yes
           access-point, name = "AP 2G":
             wps:
               status: disabled
               authentication: synthetic-private-key
`

func TestWiFiSystemInventoryResolvesOnlyNativeAPsAndDropsSecrets(t *testing.T) {
	r, err := ParseWiFiSystem("\x1b[K\r\n"+wifiSystemStatus, wifiSystemInterfaces)
	if err != nil {
		t.Fatal(err)
	}
	if r.SchemaVersion != 1 || len(r.WLANs) != 1 || r.UnresolvedAccessPoints != 0 {
		t.Fatal(r)
	}
	w := r.WLANs[0]
	if w.ID != "Home" || w.DisabledBySchedule == nil || *w.DisabledBySchedule || len(w.Bands) != 2 {
		t.Fatal(w)
	}
	for i, b := range w.Bands {
		if b.Band != i || b.Enabled == nil || !*b.Enabled || len(b.AccessPoints) != 1 {
			t.Fatal(b)
		}
	}
	if w.Bands[0].AccessPoints[0] != "WifiMaster0/AccessPoint0" || w.Bands[1].AccessPoints[0] != "WifiMaster1/AccessPoint0" {
		t.Fatal(w.Bands)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"12345670", "synthetic-private-key", "self-pin", "authentication", "AP 2G", "Unused_Test"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("status leaked unselected fields")
		}
	}
}

func TestWiFiSystemUnknownAliasDoesNotBorrowNestedEnabled(t *testing.T) {
	input := "wlan, id = Test:\nband, id = 0:\naccess-point, name = Unknown:\nwps:\nenabled: yes\n"
	r, err := ParseWiFiSystem(input, wifiSystemInterfaces)
	if err != nil {
		t.Fatal(err)
	}
	if r.UnresolvedAccessPoints != 1 || r.WLANs[0].Bands[0].Enabled != nil || len(r.WLANs[0].Bands[0].AccessPoints) != 0 {
		t.Fatal("unknown alias or nested flag was guessed", r)
	}
	if r.WLANs[0].DisabledBySchedule != nil {
		t.Fatal("missing schedule status was guessed")
	}
}

func TestWiFiSystemAmbiguousAliasAndCanonicalIdentifiers(t *testing.T) {
	interfaces := wifiSystemInterfaces + "Interface, name = AP_5G\n id: WifiMaster0/AccessPoint5\n"
	input := "wlan, id = Test:\nband, id = 1:\naccess-point, name = AP_5G:\n"
	r, err := ParseWiFiSystem(input, interfaces)
	if err != nil || r.UnresolvedAccessPoints != 1 {
		t.Fatal("ambiguous alias resolved", err)
	}
	input = strings.Replace(input, "AP_5G", "WifiMaster1/AccessPoint0", 1)
	r, err = ParseWiFiSystem(input, interfaces)
	if err != nil || r.UnresolvedAccessPoints != 0 || len(r.WLANs[0].Bands[0].AccessPoints) != 1 {
		t.Fatal("canonical ID not resolved", err)
	}
}

func TestWiFiSystemRejectsPartialOrContradictoryStructure(t *testing.T) {
	for _, input := range []string{
		"unexpected status",
		"band, id = 0:\n",
		"wlan, id = A:\naccess-point, name = AP_5G:\n",
		"wlan, id = A:\nband, id = 7:\n",
		"wlan, id = A:\nband, id = 0:\nband, id = 0:\n",
		"wlan, id = A:\nwlan, id = A:\n",
		"wlan, id = A:\ndisabled-by-schedule: perhaps\n",
		"wlan, id = A:\nband, id = 0:\nenabled: yes\nenabled: no\n",
		"wlan, id = A:\nband, id = malformed:\n",
		"wlan, id = A:\nband, id = 0:\naccess-point, name = AP_5G:\naccess-point, name = AP_5G:\n",
	} {
		if _, err := ParseWiFiSystem(input, wifiSystemInterfaces); err == nil {
			t.Fatal("invalid structure accepted")
		}
	}
	r, err := ParseWiFiSystem("", wifiSystemInterfaces)
	if err != nil || len(r.WLANs) != 0 {
		t.Fatal("empty inventory rejected")
	}
}

func TestWiFiSystemCollectionIsReadOnly(t *testing.T) {
	var commands []string
	r, err := CollectWiFiSystem(context.Background(), func(_ context.Context, command string) ([]byte, error) {
		commands = append(commands, command)
		if command == "show mws wlan" {
			return []byte(wifiSystemStatus), nil
		}
		if command == "show interface" {
			return []byte(wifiSystemInterfaces), nil
		}
		t.Fatal("unexpected native command")
		return nil, nil
	})
	if err != nil || len(r.WLANs) != 1 || !reflect.DeepEqual(commands, []string{"show mws wlan", "show interface"}) {
		t.Fatal(err, commands)
	}
}
