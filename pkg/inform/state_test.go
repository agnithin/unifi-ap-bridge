package inform

import (
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
		"_type": "setparam",
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
