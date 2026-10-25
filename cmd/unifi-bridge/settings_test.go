package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

func TestSettingsRejectSupersededRequestBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	prior := `{"enabled":true,"branch":"dev","custom":{"keep":1}}`
	if err := os.WriteFile(path, []byte(prior), 0600); err != nil {
		t.Fatal(err)
	}
	input := `{"request_id":"222","enabled":false,"branch":"main"}`
	if err := mergeSettings(path, strings.NewReader(input), "json", "", "111"); err == nil || !strings.Contains(err.Error(), "request id mismatch") {
		t.Fatalf("expected superseded request rejection, got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != prior {
		t.Fatalf("superseded request changed config: %s", after)
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

func TestSharedSettingsPreserveNodeOwnedFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "addon", "config", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := `{"enabled":true,"interval_sec":10,"branch":"main","controller_url":"http://old:8080/inform","state_path":"/node/state.json","uplink_mac":"aa:bb:cc:dd:ee:ff","uplink_remote_port":3,"nodes":["10.0.0.3"],"unknown_node_field":{"keep":true}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	input := `{"enabled":false,"interval_sec":30,"branch":"dev","controller_url":"http://main:8080/inform","state_path":"/main/state.json","uplink_remote_port":1,"unknown_node_field":{"replace":true}}`
	if err := mergeSharedSettings(path, strings.NewReader(input), ""); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"enabled": "false", "interval_sec": "30", "branch": `"dev"`,
		"controller_url": `"http://main:8080/inform"`, "state_path": `"/node/state.json"`,
		"uplink_remote_port": "3", "nodes": `["10.0.0.3"]`,
		"unknown_node_field": `{"keep":true}`,
	} {
		var actualValue, expectedValue any
		if json.Unmarshal(got[key], &actualValue) != nil || json.Unmarshal([]byte(want), &expectedValue) != nil || !reflect.DeepEqual(actualValue, expectedValue) {
			t.Errorf("%s = %s, want %s", key, got[key], want)
		}
	}
}

func TestConcurrentSettingsMergeDoesNotLoseUpdates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "addon", "config", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"enabled":true,"interval_sec":10}`), 0600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(root, "mutation.lock")
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK", lock)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, patch := range []string{`{"enabled":false}`, `{"interval":30}`} {
		wg.Add(1)
		go func(patch string) {
			defer wg.Done()
			errCh <- mergeSettings(path, strings.NewReader(patch), "json", "")
		}(patch)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var got map[string]json.RawMessage
	data, _ := os.ReadFile(path)
	_ = json.Unmarshal(data, &got)
	if string(got["enabled"]) != "false" || string(got["interval_sec"]) != "30" {
		t.Fatalf("concurrent merge lost an update: %s", data)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("settings lock leaked: %v", err)
	}
}

func TestSettingsRejectUnfinishedAddonTransaction(t *testing.T) {
	root := t.TempDir()
	addon := filepath.Join(root, "addon")
	path := filepath.Join(addon, "config", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(addon, ".update-transaction"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := mergeSettings(path, strings.NewReader(`{"enabled":false}`), "json", ""); err == nil {
		t.Fatal("settings mutation proceeded with unfinished update journal")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"enabled":true}` {
		t.Fatalf("config changed despite unfinished transaction: %s", data)
	}
}

func TestSettingsInheritedLockRequiresMatchingOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "addon", "config", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(root, "mutation.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte("1234\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK", lock)
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK_HELD", "1")
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK_OWNER", "9999")
	if err := mergeSettings(path, strings.NewReader(`{"enabled":false}`), "json", ""); err == nil {
		t.Fatal("accepted mismatched inherited lock owner")
	}
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK_OWNER", "1234")
	if err := mergeSettings(path, strings.NewReader(`{"enabled":false}`), "json", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("inherited lock was released by non-owner: %v", err)
	}
}
