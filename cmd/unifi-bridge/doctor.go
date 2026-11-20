package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

func marshalDiagnosticInform(payload telemetry.InformPayload) ([]byte, error) {
	payload.XAuthKey = "[redacted]"
	if endpoint, err := url.Parse(payload.InformURL); err == nil {
		endpoint.User = nil
		endpoint.RawQuery = ""
		endpoint.ForceQuery = false
		endpoint.Fragment = ""
		payload.InformURL = endpoint.String()
	} else {
		payload.InformURL = "[invalid endpoint omitted]"
	}
	return json.MarshalIndent(payload, "", "  ")
}

func clearAdoptionUnderLock(path string) error {
	if path == "" {
		return errors.New("adoption state path is empty")
	}
	if os.Getenv("UNIFI_BRIDGE_MUTATION_LOCK_HELD") != "1" {
		return errors.New("clear adoption through start.sh --reset-adoption")
	}
	release, err := acquireSettingsMutation()
	if err != nil {
		return err
	}
	defer release()
	// Remove the recoverable generation first. If interrupted before removing
	// the primary, the existing identity remains usable.
	files := []string{path + ".bak", path}
	for _, file := range files {
		if info, err := os.Lstat(file); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("adoption state is not a regular file")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	dir, err := os.Open(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readLocalStatus(dir, name, variable string, dest any) error {
	file, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer file.Close()
	const maxStatus = 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(file, maxStatus+1))
	if err != nil {
		return err
	}
	if len(raw) > maxStatus {
		return errors.New("status file too large")
	}
	body, ok := bytes.CutPrefix(bytes.TrimSpace(raw), []byte("window."+variable+" = "))
	if !ok {
		return errors.New("unexpected status format")
	}
	return json.Unmarshal(bytes.TrimSuffix(body, []byte(";")), dest)
}

func printDoctor(cfg *Config, statusDir, configPath string) error {
	var status struct {
		Running      bool   `json:"running"`
		Updated      int64  `json:"status_updated_at"`
		SampleAt     int64  `json:"sample_updated_at"`
		SampleFresh  bool   `json:"sample_fresh"`
		LastInformAt int64  `json:"last_successful_inform_at"`
		Commit       string `json:"commit"`
		Version      string `json:"version"`
		PendingSave  bool   `json:"pending_state_save"`
		StateError   string `json:"state_save_error"`
		WLANMapping  struct {
			Entries   int    `json:"entries"`
			LastError string `json:"last_error"`
		} `json:"wlan_mapping"`
	}
	fmt.Printf("Configured enabled: %t\n", cfg.Enabled)
	if err := readLocalStatus(statusDir, "status.js", "unifiBridgeStatus", &status); err != nil {
		fmt.Println("Service snapshot: unavailable")
	} else {
		fmt.Printf("Service snapshot: running=%t, age=%s\n", status.Running, statusAge(status.Updated))
		fmt.Printf("Reported build: %s (%s)\n", status.Version, status.Commit)
		fmt.Printf("Telemetry: sample age=%s, latest attempt fresh=%t\n", statusAge(status.SampleAt), status.SampleFresh)
		fmt.Printf("Last successful Inform: %s\n", statusAge(status.LastInformAt))
		fmt.Printf("Adoption persistence: pending=%t, last write failed=%t\n", status.PendingSave, status.StateError != "")
		fmt.Printf("WLAN mapping: %d entries, last read failed=%t\n", status.WLANMapping.Entries, status.WLANMapping.LastError != "")
	}
	for _, item := range []struct{ path, label string }{
		{cfg.StatePath, "Primary adoption file"}, {cfg.StatePath + ".bak", "Backup adoption file"},
		{cfg.WlanMappingFile, "WLAN mapping file"},
	} {
		if item.path == "" {
			continue
		}
		if info, err := os.Stat(item.path); err == nil {
			fmt.Printf("%s: present, last changed %s\n", item.label, statusAge(info.ModTime().Unix()))
		} else {
			fmt.Printf("%s: unavailable\n", item.label)
		}
	}
	var watchdog struct {
		Reason  string `json:"reason"`
		Updated int64  `json:"updated_at"`
	}
	if readLocalStatus(statusDir, "watchdog.js", "unifiBridgeWatchdog", &watchdog) == nil {
		fmt.Printf("Watchdog: %s (reported %s)\n", watchdog.Reason, statusAge(watchdog.Updated))
	}
	if configPath != "" {
		if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(configPath)), ".update-transaction", "active")); err == nil {
			fmt.Println("Recovery: update journal active")
		}
	}
	return nil
}

func statusAge(sec int64) string {
	if sec <= 0 {
		return "unknown"
	}
	age := time.Since(time.Unix(sec, 0))
	if age < 0 {
		return "clock differs"
	}
	return age.Truncate(time.Second).String() + " ago"
}
