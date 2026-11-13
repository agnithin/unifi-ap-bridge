package inform

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyControllerResponse(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	mac := "fc:34:97:46:7b:10"

	sm, err := NewStateManager(statePath, mac)
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}

	if sm.IsAdopted() {
		t.Errorf("expected initially not adopted")
	}

	// Test setparam with mgmt_cfg multiline
	payload := map[string]any{
		"_type":    "setparam",
		"mgmt_cfg": "authkey=0123456789abcdef0123456789abcdef\ncfgversion=a1b2c3d4e5f6\nuse_aes_gcm=true\n",
	}

	changed, err := sm.ApplyControllerResponse(payload)
	if err != nil {
		t.Fatalf("apply response failed: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true")
	}
	if !sm.IsAdopted() {
		t.Errorf("expected sm to be adopted")
	}
	if sm.GetMode() != ModeGCM {
		t.Errorf("expected mode GCM, got %s", sm.GetMode())
	}
	if sm.GetCfgVersion() != "a1b2c3d4e5f6" {
		t.Errorf("expected cfgversion a1b2c3d4e5f6, got %s", sm.GetCfgVersion())
	}

	// Verify persistence
	if err := sm.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	sm2, err := NewStateManager(statePath, mac)
	if err != nil {
		t.Fatalf("failed to reload state manager: %v", err)
	}
	if !sm2.IsAdopted() {
		t.Errorf("expected sm2 to be adopted")
	}
	if sm2.GetMode() != ModeGCM {
		t.Errorf("expected mode GCM on reload, got %s", sm2.GetMode())
	}
}

func TestRejectMalformedAuthKeyWithoutChangingIdentity(t *testing.T) {
	sm, err := NewStateManager(filepath.Join(t.TempDir(), "state.json"), "fc:34:97:46:7b:10")
	if err != nil {
		t.Fatal(err)
	}
	good := "0123456789abcdef0123456789abcdef"
	if _, err := sm.ApplyControllerResponse(map[string]any{"_type": "setparam", "authkey": good}); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []map[string]any{
		{"_type": "setparam", "authkey": "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"_type": "setparam", "authkey": "short"},
		{"_type": "cmd", "cmd": "set-adopt", "key": "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
	} {
		changed, err := sm.ApplyControllerResponse(payload)
		if changed || err == nil {
			t.Fatal("accepted malformed auth key")
		}
		if !sm.IsAdopted() || hex.EncodeToString(sm.GetKey()) != good {
			t.Fatal("malformed response changed identity")
		}
	}
}

func TestRejectCorruptedPersistedAuthKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"mac":"fc:34:97:46:7b:10","authkey":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStateManager(path, "fc:34:97:46:7b:10"); err == nil {
		t.Fatal("loaded invalid auth key")
	}
}

func TestStateRecoveryValidatesEveryCandidate(t *testing.T) {
	mac := "fc:34:97:46:7b:10"
	good := `{"schema":1,"mac":"fc:34:97:46:7b:10","authkey":"0123456789abcdef0123456789abcdef"}`
	for _, primary := range []string{"{", `{"schema":2}`, `{"schema":1,"mac":"fc:34:97:46:7b:10","authkey":"bad"}`} {
		t.Run(primary, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			os.WriteFile(path, []byte(primary), 0600)
			os.WriteFile(path+".bak", []byte(good), 0600)
			sm, err := NewStateManager(path, mac)
			if err != nil || !sm.IsAdopted() {
				t.Fatalf("recovery: %v", err)
			}
			if err := sm.Save(); err != nil {
				t.Fatal(err)
			}
			backup, _ := os.ReadFile(path + ".bak")
			if string(backup) != good {
				t.Fatal("corrupt primary replaced valid backup")
			}
		})
	}
	for _, backup := range []string{`{"schema":1,"mac":"fc:34:97:46:7b:10","adopted":true}`, `{"schema":1,"mac":"fc:34:97:46:7b:10","authkey":"bad"}`} {
		path := filepath.Join(t.TempDir(), "state.json")
		os.WriteFile(path, []byte("{"), 0600)
		os.WriteFile(path+".bak", []byte(backup), 0600)
		if _, err := NewStateManager(path, mac); err == nil {
			t.Fatal("accepted invalid backup")
		}
		os.Remove(path)
		if _, err := NewStateManager(path, mac); err == nil {
			t.Fatal("bootstrapped over invalid backup")
		}
	}
}

func TestStateSaveKeepsPrivatePreviousGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	sm, err := NewStateManager(path, "fc:34:97:46:7b:10")
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Save(); err != nil {
		t.Fatal(err)
	}
	prior, _ := os.ReadFile(path)
	os.Chmod(path, 0644)
	// A pre-existing predictable temp symlink must never be followed.
	victim := filepath.Join(filepath.Dir(path), "victim")
	os.WriteFile(victim, []byte("untouched"), 0600)
	os.Symlink(victim, path+".tmp")
	sm.ApplyControllerResponse(map[string]any{"_type": "setparam", "authkey": "0123456789abcdef0123456789abcdef"})
	if err := sm.Save(); err != nil {
		t.Fatal(err)
	}
	backup, _ := os.ReadFile(path + ".bak")
	if string(backup) != string(prior) {
		t.Fatal("backup is not previous generation")
	}
	for _, name := range []string{path, path + ".bak"} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("state permissions: %v", err)
		}
	}
	data, _ := os.ReadFile(victim)
	if string(data) != "untouched" {
		t.Fatal("followed temp symlink")
	}
}
