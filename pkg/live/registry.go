package live

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	descriptorDirName = "flows/live/servers"
	healthTimeout     = 200 * time.Millisecond
)

// Descriptor identifies a running `flow chart` server instance.
type Descriptor struct {
	Version           int    `json:"version"`
	PID               int    `json:"pid"`
	CreatedAtUnixNano int64  `json:"created_at_unix_nano"`
	BaseURL           string `json:"base_url"`
	Token             string `json:"token"`
	FlowKey           string `json:"flow_key"`
	CanonicalPath     string `json:"canonical_path"`
}

func descriptorDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, descriptorDirName), nil
}

func ensureDescriptorDir() (string, error) {
	dir, err := descriptorDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// RegisterDescriptor writes a descriptor file and returns a cleanup function
// that removes it. Missing fields (Version, PID, CreatedAtUnixNano) are
// populated automatically.
func RegisterDescriptor(d Descriptor) (cleanup func(), path string, err error) {
	dir, err := ensureDescriptorDir()
	if err != nil {
		return nil, "", err
	}
	if d.Version == 0 {
		d.Version = ProtocolVersion
	}
	if d.PID == 0 {
		d.PID = os.Getpid()
	}
	if d.CreatedAtUnixNano == 0 {
		d.CreatedAtUnixNano = time.Now().UnixNano()
	}
	name := fmt.Sprintf("editor-%d-%d.json", d.PID, d.CreatedAtUnixNano)
	full := filepath.Join(dir, name)

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return nil, "", err
	}
	cleanup = func() { _ = os.Remove(full) }
	return cleanup, full, nil
}

// DiscoverDescriptors returns descriptors matching the given flow_key whose
// editor process is alive and reachable. As a side effect, descriptors that
// fail liveness or health checks are deleted. Descriptors with a Version
// other than the current ProtocolVersion are skipped but NOT deleted (a newer
// flow chart may still own them).
func DiscoverDescriptors(flowKey string) ([]Descriptor, error) {
	dir, err := descriptorDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var matches []Descriptor
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "editor-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var d Descriptor
		if err := json.Unmarshal(data, &d); err != nil {
			continue
		}
		if d.Version != ProtocolVersion {
			continue
		}
		if d.FlowKey != flowKey {
			continue
		}
		if !ProcessExists(d.PID) {
			_ = os.Remove(path)
			continue
		}
		if err := healthProbe(d); err != nil {
			_ = os.Remove(path)
			continue
		}
		matches = append(matches, d)
	}
	return matches, nil
}

// healthProbe issues a short GET to the editor's /api/live/health endpoint
// and verifies the response identifies the same descriptor.
func healthProbe(d Descriptor) error {
	client := &http.Client{Timeout: healthTimeout}
	resp, err := client.Get(d.BaseURL + "/api/live/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	var h struct {
		OK      bool   `json:"ok"`
		Version int    `json:"version"`
		FlowKey string `json:"flow_key"`
		PID     int    `json:"pid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return err
	}
	if !h.OK || h.Version != ProtocolVersion || h.FlowKey != d.FlowKey || h.PID != d.PID {
		return fmt.Errorf("descriptor mismatch")
	}
	return nil
}
