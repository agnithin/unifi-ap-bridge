package main

import (
	"encoding/json"
	"github.com/agnithin/unifi-ap-bridge/pkg/models"
	"os"
	"path/filepath"
	"time"
)

// Publish the disabled configuration even when an older installed startup script
// invokes the new binary. No provider or adoption key is read on this path.
func publishDisabledStatus(dir string, cfg *Config) error {
	profile := models.GetProfile(cfg.Model)
	status := map[string]any{"running": false, "enabled": false, "version": version, "commit": commit, "branch": cfg.Branch, "model_code": profile.ModelCode, "model": profile.DisplayName, "firmware": profile.Firmware, "interval_sec": cfg.IntervalSec, "controller_url": cfg.ControllerURL, "total_stations": 0, "stations_2g": 0, "stations_5g": 0, "last_status": "Disabled by configuration", "status_updated_at": time.Now().Unix()}
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	temp := filepath.Join(dir, "status.js.tmp")
	if err = os.WriteFile(temp, append(append([]byte("window.unifiBridgeStatus = "), raw...), ';', '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(dir, "status.js"))
}
