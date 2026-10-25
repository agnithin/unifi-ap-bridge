package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agnithin/unifi-ap-bridge/pkg/models"
)

// mergeSettings preserves fields owned by other components (including WLAN IDs,
// node lists and per-node topology) instead of reconstructing the configuration.
func mergeSettings(path string, input io.Reader, format, gateway string) error {
	raw, err := io.ReadAll(io.LimitReader(input, 65537))
	if err != nil || len(raw) > 65536 {
		return fmt.Errorf("settings input exceeds limit")
	}
	var patch map[string]json.RawMessage
	if format == "custom" {
		patch = make(map[string]json.RawMessage)
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := strings.TrimPrefix(fields[0], "unifibridge_")
			if key == fields[0] {
				continue
			}
			value := strings.Join(fields[1:], " ")
			switch key {
			case "enabled", "auto_gateway":
				if value != "0" && value != "1" {
					return fmt.Errorf("invalid %s", key)
				}
				patch[key] = json.RawMessage(strconv.FormatBool(value == "1"))
			case "interval":
				patch[key] = json.RawMessage(value)
			case "controller_url", "model", "branch":
				b, _ := json.Marshal(value)
				patch[key] = b
			}
		}
	} else if format == "json" {
		if err := json.Unmarshal(raw, &patch); err != nil || patch == nil {
			return fmt.Errorf("settings must be a JSON object")
		}
	} else {
		return fmt.Errorf("unknown settings format")
	}
	current := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(data, &current); err != nil || current == nil {
			return fmt.Errorf("existing configuration is invalid")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for key, value := range patch {
		target := key
		switch key {
		case "enabled", "auto_gateway":
			var b bool
			if string(value) == "null" || json.Unmarshal(value, &b) != nil {
				return fmt.Errorf("invalid %s", key)
			}
		case "interval":
			var n int
			if json.Unmarshal(value, &n) != nil || n < 1 || n > 3600 {
				return fmt.Errorf("interval must be 1..3600 seconds")
			}
			target = "interval_sec"
		case "model":
			var v string
			if json.Unmarshal(value, &v) != nil {
				return fmt.Errorf("invalid model")
			}
			if _, ok := models.Catalog[v]; !ok {
				return fmt.Errorf("unsupported model")
			}
		case "branch":
			var v string
			if json.Unmarshal(value, &v) != nil || (v != "main" && v != "dev") {
				return fmt.Errorf("invalid branch")
			}
		case "controller_url":
			var v string
			if json.Unmarshal(value, &v) != nil {
				return fmt.Errorf("invalid controller URL")
			}
			u, e := url.Parse(v)
			if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
				return fmt.Errorf("invalid controller URL")
			}
		default:
			return fmt.Errorf("unsupported settings field")
		}
		current[target] = value
	}
	var auto bool
	if value, ok := current["auto_gateway"]; ok {
		if json.Unmarshal(value, &auto) != nil {
			return fmt.Errorf("invalid auto_gateway")
		}
	}
	if auto {
		if net.ParseIP(gateway) == nil || gateway == "0.0.0.0" {
			return fmt.Errorf("invalid gateway address")
		}
		value, _ := json.Marshal("http://" + net.JoinHostPort(gateway, "8080") + "/inform")
		current["controller_url"] = value
	}
	// Validate known daemon fields without dropping unknown fields.
	encoded, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	var cfg Config
	if err = json.Unmarshal(encoded, &cfg); err != nil {
		return fmt.Errorf("invalid configuration fields")
	}
	encoded = append(encoded, '\n')
	// Compare canonical JSON before creating a temporary file: no-op settings
	// must not write a fresh copy to flash merely because key order differs.
	if data != nil {
		var previous map[string]json.RawMessage
		if json.Unmarshal(data, &previous) == nil {
			canonical, _ := json.Marshal(previous)
			next, _ := json.Marshal(current)
			if bytes.Equal(canonical, next) {
				info, e := os.Stat(path)
				if e != nil {
					return e
				}
				if info.Mode().Perm() != 0600 {
					return os.Chmod(path, 0600)
				}
				return nil
			}
		}
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(encoded)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp, path)
}
