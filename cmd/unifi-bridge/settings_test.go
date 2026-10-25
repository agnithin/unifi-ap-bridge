package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSettingsPreserveConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{"enabled":true,"auto_gateway":false,"controller_url":"http://controller.test:8080/inform","model":"U6IW","branch":"dev","interval_sec":10,"wlan_mapping":{"custom":"0123456789abcdef01234567"},"nodes":["10.0.0.2"],"uplink_remote_port":3,"extra":{"keep":true}}`
	if e := os.WriteFile(path, []byte(original), 0644); e != nil {
		t.Fatal(e)
	}
	if e := mergeSettings(path, strings.NewReader(`{"enabled":false,"interval":15}`), "json", ""); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(path)
	var got, before map[string]json.RawMessage
	_ = json.Unmarshal(raw, &got)
	_ = json.Unmarshal([]byte(original), &before)
	for _, k := range []string{"wlan_mapping", "nodes", "uplink_remote_port", "extra"} {
		var a, b any
		_ = json.Unmarshal(got[k], &a)
		_ = json.Unmarshal(before[k], &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("lost %s", k)
		}
	}
	if string(got["enabled"]) != "false" || string(got["interval_sec"]) != "15" {
		t.Fatal(string(raw))
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	beforeInfo, _ := os.Stat(path)
	if e := mergeSettings(path, strings.NewReader(`{"enabled":false,"interval":15}`), "json", ""); e != nil {
		t.Fatal(e)
	}
	afterInfo, _ := os.Stat(path)
	if !os.SameFile(beforeInfo, afterInfo) {
		t.Fatal("no-op settings replaced configuration")
	}
}
func TestInvalidSettingsLeaveOriginalIntact(t *testing.T) {
	for _, patch := range []string{`{"interval":0}`, `{"enabled":"false"}`, `{"enabled":null}`, `{"branch":"other"}`, `{"model":"oops"}`, `{"controller_url":"file:///etc/passwd"}`, `{"nodes":[]}`, `{"interval":5} trailing`} {
		t.Run(patch, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			original := []byte(`{"enabled":true}`)
			_ = os.WriteFile(path, original, 0600)
			if mergeSettings(path, strings.NewReader(patch), "json", "") == nil {
				t.Fatal("accepted invalid input")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(original) {
				t.Fatal("modified original")
			}
		})
	}
}
func TestCustomSettingsAndGateway(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(path, []byte(`{"wlan_mapping":{},"nodes":["10.0.0.2"]}`), 0600)
	input := "other_addon_setting keep\nunifibridge_enabled 0\nunifibridge_auto_gateway 1\nunifibridge_interval 15\nunifibridge_model U6IW\nunifibridge_branch dev\n"
	if e := mergeSettings(path, strings.NewReader(input), "custom", "10.0.0.1"); e != nil {
		t.Fatal(e)
	}
	c, e := loadConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	if c.Enabled || c.ControllerURL != "http://10.0.0.1:8080/inform" || c.IntervalSec != 15 {
		t.Fatalf("unexpected settings: %+v", c)
	}
	if e := mergeSettings(path, strings.NewReader(`{"auto_gateway":true}`), "json", "not an IP"); e == nil {
		t.Fatal("accepted invalid gateway")
	}
}
func TestLegacyConfigEnabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(path, []byte(`{"interval_sec":10}`), 0600)
	cfg, e := loadConfig(path)
	if e != nil || !cfg.Enabled {
		t.Fatal("legacy configs must remain enabled")
	}
}
