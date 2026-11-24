// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// CredentialReceipts remembers acknowledged, confirmed writes, not decrypted
// native passwords. The adoption key authenticates fingerprints and is never
// stored in this file. A changed key, request, SSID or native value causes a miss.
type CredentialReceipts struct {
	path   string
	key    []byte
	device string
}

type credentialBinding struct {
	Requested string `json:"requested"`
	Observed  string `json:"observed"`
}

func (b credentialBinding) matches(other credentialBinding) bool {
	return hmac.Equal([]byte(b.Requested), []byte(other.Requested)) && hmac.Equal([]byte(b.Observed), []byte(other.Observed))
}

type credentialReceiptFile struct {
	Schema   int                          `json:"schema"`
	Bindings map[string]credentialBinding `json:"bindings"`
}

type credentialRequest struct{ ssid, setting string }

func NewCredentialReceipts(path, adoptionKey, deviceMAC string) (*CredentialReceipts, error) {
	key, err := hex.DecodeString(adoptionKey)
	mac, macErr := net.ParseMAC(deviceMAC)
	if err != nil || len(key) != 16 || !filepath.IsAbs(path) || macErr != nil || len(mac) != 6 {
		return nil, fmt.Errorf("credential receipts require an absolute path, valid adoption key and device MAC")
	}
	return &CredentialReceipts{path: path, key: key, device: mac.String()}, nil
}

func (r *CredentialReceipts) fingerprint(parts ...string) string {
	h := hmac.New(sha256.New, r.key)
	_ = json.NewEncoder(h).Encode(append([]string{r.device}, parts...))
	return hex.EncodeToString(h.Sum(nil))
}

func credentialRequests(plan Plan) map[string]credentialRequest {
	requests := map[string]credentialRequest{}
	ssids, keys := map[string]int{}, map[string]int{}
	for _, c := range plan.Commands {
		p := strings.SplitN(c.Text, " ", 3)
		if len(p) != 3 || p[0] != "interface" || !regexpAP.MatchString(p[1]) {
			continue
		}
		x := requests[p[1]]
		if strings.HasPrefix(p[2], "ssid ") {
			x.ssid = p[2]
			ssids[p[1]]++
		}
		if c.Secret && strings.HasPrefix(p[2], "authentication wpa-psk ") {
			x.setting = p[2]
			keys[p[1]]++
		}
		requests[p[1]] = x
	}
	for target := range requests {
		if ssids[target] != 1 || keys[target] != 1 {
			delete(requests, target)
		}
	}
	return requests
}

func (r *CredentialReceipts) binding(target string, request credentialRequest, running string) (credentialBinding, bool) {
	ssid, key := "", ""
	ssids, keys := 0, 0
	for _, line := range interfaceBlocks(running)[target] {
		if strings.HasPrefix(line, "ssid ") {
			ssid = line
			ssids++
		}
		if strings.HasPrefix(line, "authentication wpa-psk ") {
			key = line
			keys++
		}
	}
	if ssids != 1 || keys != 1 || !sameStringSetting(request.ssid, ssid, "ssid") {
		return credentialBinding{}, false
	}
	return credentialBinding{Requested: r.fingerprint("requested", target, request.ssid, request.setting), Observed: r.fingerprint("observed", target, ssid, key)}, true
}

// Missing, malformed, public or symlinked files are cache misses, never proof.
func (r *CredentialReceipts) read() map[string]credentialBinding {
	empty := map[string]credentialBinding{}
	f, err := os.OpenFile(r.path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return empty
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() > 65536 {
		return empty
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	var file credentialReceiptFile
	if err != nil || len(b) > 65536 || json.Unmarshal(b, &file) != nil || file.Schema != 1 || len(file.Bindings) > 14 {
		return empty
	}
	for target, entry := range file.Bindings {
		if !regexpAP.MatchString(target) {
			return empty
		}
		for _, value := range []string{entry.Requested, entry.Observed} {
			decoded, err := hex.DecodeString(value)
			if err != nil || len(decoded) != sha256.Size {
				return empty
			}
		}
	}
	if file.Bindings == nil {
		return empty
	}
	return file.Bindings
}

func (r *CredentialReceipts) filter(plan Plan, running string, entries map[string]credentialBinding) Plan {
	requests := credentialRequests(plan)
	result := plan
	result.Commands = nil
	for _, c := range plan.Commands {
		p := strings.SplitN(c.Text, " ", 3)
		if len(p) == 3 && p[0] == "interface" && c.Secret {
			if request, ok := requests[p[1]]; ok && request.setting == p[2] {
				binding, valid := r.binding(p[1], request, running)
				stored, found := entries[p[1]]
				if valid && found && binding.matches(stored) {
					continue
				}
			}
		}
		result.Commands = append(result.Commands, c)
	}
	return result
}

// Preimage provides a receipt-aware, read-only preflight. Ordinary Preimage
// remains independent of persisted state and conservatively includes writes.
func (r *CredentialReceipts) Preimage(plan Plan, running string) ([]Change, error) {
	return Preimage(r.filter(plan, running, r.read()), running)
}

func (r *CredentialReceipts) write(entries map[string]credentialBinding) error {
	b, err := json.Marshal(credentialReceiptFile{Schema: 1, Bindings: entries})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".credential-receipts-*")
	if err != nil {
		return err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	if _, err := f.Write(b); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), r.path); err != nil {
		return err
	}
	return syncDir(r.path)
}
