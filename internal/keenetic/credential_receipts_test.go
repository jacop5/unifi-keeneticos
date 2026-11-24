// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type receiptModel struct {
	target, ssid, key   string
	state               string
	writes, health      int
	failSet, failHealth bool
}

func (m *receiptModel) run(_ context.Context, command string) ([]byte, error) {
	if command == "show running-config" {
		state := m.state
		if state == "" {
			state = "up"
		}
		return []byte("interface " + m.target + "\n    ssid " + m.ssid + "\n    " + m.key + "\n    " + state + "\n!\n"), nil
	}
	prefix := "interface " + m.target + " "
	if !strings.HasPrefix(command, prefix) {
		return nil, errors.New("unexpected target")
	}
	m.writes++
	setting := strings.TrimPrefix(command, prefix)
	switch {
	case strings.HasPrefix(setting, "ssid "):
		m.ssid = strings.Trim(strings.TrimPrefix(setting, "ssid "), `"`)
	case strings.HasPrefix(setting, `authentication wpa-psk "`):
		if m.failSet {
			return nil, errors.New("injected native failure")
		}
		h := sha256.Sum256([]byte(setting))
		m.key = "authentication wpa-psk ns3 " + hex.EncodeToString(h[:])
	case strings.HasPrefix(setting, "authentication wpa-psk ns3 "):
		m.key = setting
	case setting == "no authentication wpa-psk":
		m.key = ""
	case setting == "up" || setting == "down":
		m.state = setting
	default:
		return nil, errors.New("unexpected setting")
	}
	return nil, nil
}

func receiptPlan(target, ssid, password string) Plan {
	return Plan{Commands: []Command{{Text: "interface " + target + ` ssid "` + ssid + `"`}, {Text: "interface " + target + ` authentication wpa-psk "` + password + `"`, Secret: true}, {Text: "interface " + target + " up"}}}
}

func receiptFixture(t *testing.T) (*receiptModel, *CredentialReceipts, string) {
	t.Helper()
	dir := t.TempDir()
	r, err := NewCredentialReceipts(filepath.Join(dir, "receipts.json"), strings.Repeat("a", 32), "02:00:00:00:00:01")
	if err != nil {
		t.Fatal(err)
	}
	return &receiptModel{target: "WifiMaster0/AccessPoint6", ssid: "Synthetic-Receipt-SSID", key: "authentication wpa-psk ns3 opaque-before"}, r, filepath.Join(dir, "rollback.json")
}

func applyReceipt(m *receiptModel, r *CredentialReceipts, journal string, plan Plan) error {
	return ApplyPlanWatchedWithReceipts(context.Background(), m.run, plan, journal, func(context.Context) error {
		m.health++
		if m.failHealth {
			return errors.New("injected health failure")
		}
		return nil
	}, nil, r)
}

func TestCredentialReceiptsSkipOnlyConfirmedWritesAfterRestart(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	if m.writes != 1 || m.health != 1 {
		t.Fatal("first credential write/health missing")
	}
	st, err := os.Stat(r.path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("receipt is not private")
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"synthetic-secret-passphrase", m.ssid, "opaque-before", "ns3", strings.Repeat("a", 32)} {
		if strings.Contains(string(b), secret) {
			t.Fatal("receipt exposed credential/SSID/key material")
		}
	}
	restarted, err := NewCredentialReceipts(r.path, strings.Repeat("a", 32), "02:00:00:00:00:01")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyReceipt(m, restarted, journal, plan); err != nil {
		t.Fatal(err)
	}
	if m.writes != 1 || m.health != 2 {
		t.Fatalf("repeated write or skipped health: writes=%d health=%d", m.writes, m.health)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("no-op created rollback journal")
	}
}

func TestCredentialReceiptDriftForcesWrite(t *testing.T) {
	for _, drift := range []string{"password", "ssid", "native-key", "native-ssid", "adoption-key", "target", "device"} {
		t.Run(drift, func(t *testing.T) {
			m, r, journal := receiptFixture(t)
			plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
			if err := applyReceipt(m, r, journal, plan); err != nil {
				t.Fatal(err)
			}
			switch drift {
			case "password":
				plan = receiptPlan(m.target, m.ssid, "changed-secret-passphrase")
			case "ssid":
				plan = receiptPlan(m.target, "Changed-Synthetic-SSID", "synthetic-secret-passphrase")
			case "native-key":
				m.key = "authentication wpa-psk ns3 externally-changed"
			case "native-ssid":
				m.ssid = "External-Synthetic-SSID"
			case "adoption-key":
				var err error
				r, err = NewCredentialReceipts(r.path, strings.Repeat("b", 32), "02:00:00:00:00:01")
				if err != nil {
					t.Fatal(err)
				}
			case "target":
				m.target = "WifiMaster1/AccessPoint6"
				plan = receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
			case "device":
				var err error
				r, err = NewCredentialReceipts(r.path, strings.Repeat("a", 32), "02:00:00:00:00:02")
				if err != nil {
					t.Fatal(err)
				}
			}
			before := m.writes
			if err := applyReceipt(m, r, journal, plan); err != nil {
				t.Fatal(err)
			}
			if m.writes <= before {
				t.Fatal("drift reused stale receipt")
			}
		})
	}
}

func TestCredentialReceiptsNeverLearnFailedOperations(t *testing.T) {
	for _, failure := range []string{"native", "health", "watchdog"} {
		t.Run(failure, func(t *testing.T) {
			m, r, journal := receiptFixture(t)
			before := m.key
			m.failSet = failure == "native"
			m.failHealth = failure == "health"
			var arm func(string) error
			if failure == "watchdog" {
				arm = func(string) error { return errors.New("injected watchdog failure") }
			}
			err := ApplyPlanWatchedWithReceipts(context.Background(), m.run, receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase"), journal, func(context.Context) error {
				if m.failHealth {
					return errors.New("health failed")
				}
				return nil
			}, arm, r)
			if err == nil {
				t.Fatal("injected failure succeeded")
			}
			if _, err := os.Stat(r.path); !os.IsNotExist(err) {
				t.Fatal("failed operation created receipt")
			}
			if m.key != before {
				t.Fatal("failed operation did not restore native credential")
			}
		})
	}
}

func TestCredentialReceiptInvalidFilesAreMisses(t *testing.T) {
	for _, bad := range []string{"missing", "json", "schema", "public", "oversize", "symlink"} {
		t.Run(bad, func(t *testing.T) {
			m, r, journal := receiptFixture(t)
			plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
			if err := applyReceipt(m, r, journal, plan); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(r.path)
			if err != nil {
				t.Fatal(err)
			}
			switch bad {
			case "missing":
				err = os.Remove(r.path)
			case "json":
				err = os.WriteFile(r.path, []byte("invalid"), 0600)
			case "schema":
				err = os.WriteFile(r.path, []byte(`{"schema":99,"bindings":{}}`), 0600)
			case "public":
				err = os.Chmod(r.path, 0644)
			case "oversize":
				err = os.WriteFile(r.path, []byte(strings.Repeat("x", 65537)), 0600)
			case "symlink":
				other := filepath.Join(t.TempDir(), "external-receipt")
				if err := os.WriteFile(other, original, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(r.path); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(other, r.path)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := m.writes
			if err := applyReceipt(m, r, journal, plan); err != nil {
				t.Fatal(err)
			}
			if m.writes != before+1 {
				t.Fatal("invalid receipt suppressed credential write")
			}
		})
	}
}

func TestCredentialReceiptDoesNotBypassPendingRecoveryOrHealth(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	m.failHealth = true
	if err := applyReceipt(m, r, journal, plan); err == nil {
		t.Fatal("cached credentials bypassed health failure")
	}
	if m.writes != 1 {
		t.Fatal("failed no-op health mutated native state")
	}
	m.failHealth = false
	if err := SaveUndo(journal, []Change{{Undo: Command{Text: "pending-inverse"}}}); err != nil {
		t.Fatal(err)
	}
	before := m.health
	if err := applyReceipt(m, r, journal, plan); err == nil {
		t.Fatal("cached credentials bypassed pending journal")
	}
	if m.health != before || m.writes != 1 {
		t.Fatal("pending recovery invoked health or mutation")
	}
}

func TestCredentialReceiptRemovalAndPersistenceFailure(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	open := Plan{Commands: []Command{{Text: "interface " + m.target + " no authentication wpa-psk", Secret: true}}}
	if err := applyReceipt(m, r, journal, open); err != nil {
		t.Fatal(err)
	}
	if len(r.read()) != 0 {
		t.Fatal("removed authentication retained receipt")
	}
	if err := os.Remove(r.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(r.path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal("optional cache persistence changed confirmed result")
	}
}

func TestCredentialReceiptRejectsInvalidKeysAndFiles(t *testing.T) {
	for _, key := range []string{"", strings.Repeat("z", 32), "aabb"} {
		if _, err := NewCredentialReceipts(filepath.Join(t.TempDir(), "cache"), key, "02:00:00:00:00:01"); err == nil {
			t.Fatal("invalid adoption key accepted")
		}
	}
	m, r, journal := receiptFixture(t)
	m.key = `authentication wpa-psk "synthetic-secret-passphrase"`
	if err := applyReceipt(m, r, journal, receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.path); !os.IsNotExist(err) {
		t.Fatal("no-op readback was learned as a write")
	}
	bad := credentialReceiptFile{Schema: 1, Bindings: map[string]credentialBinding{"Bridge0": {Requested: strings.Repeat("a", 64), Observed: strings.Repeat("a", 64)}}}
	b, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if len(r.read()) != 0 {
		t.Fatal("invalid target accepted")
	}
}

func TestCredentialReceiptSurvivesPauseAndSkipsResumeCredential(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	pause := Plan{Commands: []Command{{Text: "interface " + m.target + " down"}}}
	if err := applyReceipt(m, r, journal, pause); err != nil {
		t.Fatal(err)
	}
	if len(r.read()) != 1 {
		t.Fatal("pause discarded unchanged credential receipt")
	}
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	if m.writes != 3 || m.health != 3 {
		t.Fatalf("resume unnecessarily rewrote key: writes=%d health=%d", m.writes, m.health)
	}
}

func TestCredentialReceiptNotPublishedAfterRecoveryOrCancellation(t *testing.T) {
	for _, mode := range []string{"recovery", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			m, r, journal := receiptFixture(t)
			before := m.key
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := ApplyPlanWatchedWithReceipts(ctx, m.run, receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase"), journal, func(context.Context) error {
				if mode == "recovery" {
					return RecoverUndo(context.Background(), journal, m.run)
				}
				cancel()
				return nil
			}, nil, r)
			if err == nil {
				t.Fatal("lost/canceled transaction confirmed")
			}
			if m.key != before {
				t.Fatal("preimage was not restored")
			}
			if _, err := os.Stat(r.path); !os.IsNotExist(err) {
				t.Fatal("unconfirmed receipt persisted")
			}
		})
	}
}

func TestCredentialReceiptAwarePreflightIsReadOnly(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	native, err := m.run(context.Background(), "show running-config")
	if err != nil {
		t.Fatal(err)
	}
	changes, err := r.Preimage(plan, string(native))
	if err != nil || len(changes) != 0 {
		t.Fatalf("cached preflight: %v changes=%d", err, len(changes))
	}
	uncached, err := Preimage(plan, string(native))
	if err != nil || len(uncached) != 1 {
		t.Fatal("ordinary preflight must remain conservative without receipts")
	}
	if m.writes != 1 || m.health != 1 {
		t.Fatal("preflight executed mutation or health")
	}
}

func TestCredentialReceiptAmbiguousReadbackNeverSkips(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	b, err := m.run(context.Background(), "show running-config")
	if err != nil {
		t.Fatal(err)
	}
	for _, native := range []string{
		strings.Replace(string(b), m.key, "", 1),
		strings.Replace(string(b), m.key, m.key+"\n    authentication wpa-psk ns3 duplicate", 1),
		strings.Replace(string(b), "ssid "+m.ssid, "ssid "+m.ssid+"\n    ssid "+m.ssid, 1),
	} {
		changes, err := r.Preimage(plan, native)
		if err != nil || len(changes) == 0 {
			t.Fatalf("ambiguous readback suppressed write: %v changes=%d", err, len(changes))
		}
	}
	plan.Unsupported = []string{"synthetic-unsupported-setting"}
	if err := applyReceipt(m, r, journal, plan); err == nil {
		t.Fatal("receipt bypassed unsupported plan")
	}
	if m.writes != 1 || m.health != 1 {
		t.Fatal("rejected plan performed work")
	}
}

func TestCredentialReceiptRechecksNativeValueAfterHealth(t *testing.T) {
	m, r, journal := receiptFixture(t)
	plan := receiptPlan(m.target, m.ssid, "synthetic-secret-passphrase")
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	err := ApplyPlanWatchedWithReceipts(context.Background(), m.run, plan, journal, func(context.Context) error {
		m.key = "authentication wpa-psk ns3 changed-during-health"
		return nil
	}, nil, r)
	if err == nil {
		t.Fatal("credential changed during health was confirmed")
	}
	if m.writes != 1 {
		t.Fatal("no-op health failure mutated native state")
	}
	if err := applyReceipt(m, r, journal, plan); err != nil {
		t.Fatal(err)
	}
	if m.writes != 2 {
		t.Fatal("drift did not force a credential write")
	}
}
