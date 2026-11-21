// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestCountryMappingPrecedesChannels(t *testing.T) {
	cfg := "radio.1.phyname=wifi0\nradio.1.channel=6\nradio.2.phyname=wifi1\nradio.2.channel=36\nradio.countrycode=380\nradio.2.countrycode=00840\n"
	p, err := BuildPlan(cfg, nil)
	if err != nil || len(p.Unsupported) != 0 || len(p.Commands) != 4 {
		t.Fatalf("plan=%#v err=%v", p, err)
	}
	for i, want := range []string{"interface WifiMaster0 country-code IT", "interface WifiMaster0 channel 6", "interface WifiMaster1 country-code US", "interface WifiMaster1 channel 36"} {
		if p.Commands[i].Text != want {
			t.Fatalf("command %d: %s want %s", i, p.Commands[i].Text, want)
		}
	}
	for _, value := range []string{"0", "9999", "US", "840; reboot"} {
		p, err := BuildPlan("radio.1.phyname=wifi0\nradio.1.countrycode="+value+"\n", nil)
		if err != nil || len(p.Unsupported) != 1 {
			t.Fatalf("unknown country accepted: %q %v %v", value, p.Unsupported, err)
		}
		if _, err := Preimage(p, ""); err == nil {
			t.Fatal("unsupported country allowed to apply")
		}
	}
}

func TestCountryInverseRequiresExplicitPreimage(t *testing.T) {
	p := Plan{Commands: []Command{{Text: "interface WifiMaster0 country-code IT"}, {Text: "interface WifiMaster0 channel 6"}, {Text: "interface WifiMaster0 channel width 40-above"}}}
	running := "interface WifiMaster0\n    country-code DE\n    channel 13\n    channel width 40-below\n    up\n!\n"
	changes, err := Preimage(p, running)
	if err != nil || len(changes) != 7 || changes[3].Undo.Text != "interface WifiMaster0 country-code DE" {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
	for _, bad := range []string{strings.ReplaceAll(running, "    country-code DE\n", ""), strings.ReplaceAll(running, "country-code DE", "country-code INVALID")} {
		if _, err := Preimage(p, bad); err == nil {
			t.Fatal("missing/invalid country inverse accepted")
		}
	}
	matching := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(running, "DE", "IT"), "channel 13", "channel 6"), "40-below", "40-above")
	if changes, err := Preimage(p, matching); err != nil || len(changes) != 0 {
		t.Fatalf("matching country changed: %v %v", changes, err)
	}
	if VerifyCountryCodes(p, running) == nil || VerifyCountryCodes(p, "") == nil || VerifyCountryCodes(p, matching) != nil {
		t.Fatal("country confirmation failed to distinguish actual state")
	}
	for _, commands := range [][]Command{p.Commands[:1], p.Commands[:2], {p.Commands[0], p.Commands[2]}, {p.Commands[1], p.Commands[0], p.Commands[2]}} {
		if _, err := Preimage(Plan{Commands: commands}, running); err == nil {
			t.Fatal("incomplete or misordered regulatory transition accepted")
		}
	}
	if _, err := Preimage(p, strings.ReplaceAll(running, "    up\n", "")); err == nil {
		t.Fatal("regulatory transition without a radio administrative preimage accepted")
	}
}

func TestCountryTransitionHonorsDisabledRadioAndRecoveryBarrier(t *testing.T) {
	p, err := BuildPlan("radio.1.phyname=wifi0\nradio.1.countrycode=840\nradio.1.channel=6\nradio.1.ieee_mode=11nght20\nradio.1.status=disabled\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Preimage(p, "interface WifiMaster0\n    country-code DE\n    channel 13\n    channel width 40-below\n    down\n!\n")
	if err != nil || len(changes) == 0 {
		t.Fatalf("%v %v", changes, err)
	}
	for _, change := range changes {
		if strings.HasSuffix(change.Forward.Text, " up") || strings.HasSuffix(change.Undo.Text, " up") {
			t.Fatal("disabled radio was enabled by transition or recovery")
		}
	}
	last := changes[len(changes)-1]
	if last.Forward.Text != "interface WifiMaster0 down" || last.Undo.Text != "interface WifiMaster0 down" {
		t.Fatal("recovery lost its leading quiesce barrier")
	}
}

func TestCountryRollbackReturnsDomainBeforeRestrictedChannel(t *testing.T) {
	p, err := BuildPlan("radio.1.phyname=wifi0\nradio.1.countrycode=840\nradio.1.channel=6\nradio.1.ieee_mode=11nght40\nradio.1.status=enabled\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Preimage(p, "interface WifiMaster0\n    country-code DE\n    channel 13\n    channel width 40-below\n    up\n!\n")
	if err != nil {
		t.Fatal(err)
	}
	// Model the constraint that restoring channel 13 under US is rejected,
	// and that changing regulatory domains requires a quiesced neutral radio.
	country, channel, width, up := "DE", 13, "40-below", true
	run := func(command string) error {
		setting := strings.TrimPrefix(command, "interface WifiMaster0 ")
		switch {
		case setting == "down":
			up = false
		case setting == "up":
			up = true
		case setting == "no channel":
			channel = 0
		case strings.HasPrefix(setting, "channel width "):
			width = strings.TrimPrefix(setting, "channel width ")
		case strings.HasPrefix(setting, "channel "):
			n, err := strconv.Atoi(strings.TrimPrefix(setting, "channel "))
			if err != nil || (country == "US" && n > 11) {
				return fmt.Errorf("channel is not allowed in current domain")
			}
			channel = n
		case strings.HasPrefix(setting, "country-code "):
			if up || channel != 0 || width != "20" {
				return fmt.Errorf("country changed without neutralizing the radio")
			}
			country = strings.TrimPrefix(setting, "country-code ")
		default:
			return fmt.Errorf("unexpected model command")
		}
		return nil
	}
	for _, change := range changes {
		if err := run(change.Forward.Text); err != nil {
			t.Fatal(err)
		}
	}
	if country != "US" || channel != 6 || width != "40-above" || !up {
		t.Fatal("requested radio configuration not reached")
	}
	for i := len(changes) - 1; i >= 0; i-- {
		if err := run(changes[i].Undo.Text); err != nil {
			t.Fatalf("rollback ordering failed: %v", err)
		}
	}
	if country != "DE" || channel != 13 || width != "40-below" || !up {
		t.Fatal("original radio configuration was not restored")
	}
	for cut := 0; cut <= len(changes); cut++ {
		country, channel, width, up = "DE", 13, "40-below", true
		for _, change := range changes[:cut] {
			if err := run(change.Forward.Text); err != nil {
				t.Fatal(err)
			}
		}
		// A durable journal contains every inverse, even if a crash happened
		// before that forward command ran. Every possible cut must recover.
		for i := len(changes) - 1; i >= 0; i-- {
			if err := run(changes[i].Undo.Text); err != nil {
				t.Fatalf("crash cut %d: %v", cut, err)
			}
		}
		if country != "DE" || channel != 13 || width != "40-below" || !up {
			t.Fatalf("crash cut %d did not restore original radio", cut)
		}
	}
}

func TestCountryChangesNeverEnterConnectedWiFiPlan(t *testing.T) {
	cfg := strings.ReplaceAll(wlan, "Home WiFi", "UKN-Test-Country") + "radio.countrycode=380\nradio.1.countrycode=840\n"
	p, err := BuildWLANTestPlan(cfg, map[string]string{"1": "WifiMaster0/AccessPoint6"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Commands {
		if strings.Contains(c.Text, "country-code") || strings.HasPrefix(c.Text, "interface WifiMaster0 ") {
			t.Fatal("shared radio command escaped connected test guard")
		}
	}
}
