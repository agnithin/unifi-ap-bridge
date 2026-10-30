package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

func TestAdoptionRecoveryRequiresConsecutiveRejections(t *testing.T) {
	for _, interruption := range []error{nil, errors.New("timeout"), &inform.HTTPError{StatusCode: 400}, &inform.HTTPError{StatusCode: 503}} {
		h := &exchangeHealth{}
		reject := &inform.HTTPError{StatusCode: 401}
		if h.observePost(reject) || h.observePost(reject) {
			t.Fatal("premature reset")
		}
		if h.observePost(interruption) {
			t.Fatal("unrelated failure reset identity")
		}
		if h.observePost(reject) || h.observePost(&inform.HTTPError{StatusCode: 404}) {
			t.Fatal("sequence did not restart")
		}
		if !h.observePost(reject) {
			t.Fatal("third consecutive rejection did not recover")
		}
		if h.observePost(reject) {
			t.Fatal("counter did not reset")
		}
	}
}

func TestHeartbeatOnlyReflectsSuccess(t *testing.T) {
	h := &exchangeHealth{LastAttempt: time.Now()}
	if h.lastSuccessString() != "" {
		t.Fatal("attempt reported as success")
	}
	h.LastSuccess = time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC)
	h.LastAttempt = h.LastSuccess.Add(time.Minute)
	h.observePost(errors.New("timeout"))
	if h.lastSuccessString() != "2026-10-24 22:00:00" {
		t.Fatal("failure changed last success")
	}
}

func TestConfigNormalizesInvalidInterval(t *testing.T) {
	for _, raw := range []string{`{"interval_sec":0}`, `{"interval_sec":-10}`, `{}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.IntervalSec != 10 {
			t.Fatalf("interval %d", cfg.IntervalSec)
		}
	}
}

func TestAdoptionSaveRetryOnTransientFailure(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a state path inside a read-only directory to simulate transient storage failure
	readOnlyDir := filepath.Join(tmpDir, "ro_dir")
	if err := os.Mkdir(readOnlyDir, 0o500); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(readOnlyDir, "state.json")

	stateMgr, err := inform.NewStateManager(statePath, "00:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}

	health := &exchangeHealth{}

	// Controller sends an adoption response that changes state
	setDefPayload := map[string]any{
		"_type":    "setparam",
		"mgmt_cfg": "authkey=0123456789abcdef0123456789abcdef\ncfgversion=a1b2c3d4e5f6\nuse_aes_gcm=true\n",
	}

	changed, err := stateMgr.ApplyControllerResponse(setDefPayload)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected state change")
	}

	if changed {
		health.PendingStateSave = true
	}

	// First attempt to save fails because directory is read-only
	if health.PendingStateSave {
		if err := stateMgr.Save(); err != nil {
			health.StateSaveError = err.Error()
		} else {
			health.PendingStateSave = false
			health.StateSaveError = ""
		}
	}

	if !health.PendingStateSave || health.StateSaveError == "" {
		t.Fatalf("expected pending state save with error, got pending=%v err=%q", health.PendingStateSave, health.StateSaveError)
	}

	// Subsequent heartbeat receives a noop response (changed == false)
	noopPayload := map[string]any{"_type": "noop"}
	changed2, err := stateMgr.ApplyControllerResponse(noopPayload)
	if err != nil {
		t.Fatal(err)
	}
	if changed2 {
		t.Fatal("noop response should not change state")
	}

	// Restore write permissions to simulate restored storage
	if err := os.Chmod(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Retry occurs because PendingStateSave is true even though changed2 is false
	if health.PendingStateSave {
		if err := stateMgr.Save(); err != nil {
			health.StateSaveError = err.Error()
		} else {
			health.PendingStateSave = false
			health.StateSaveError = ""
		}
	}

	if health.PendingStateSave || health.StateSaveError != "" {
		t.Fatalf("expected state save to succeed on retry, got pending=%v err=%q", health.PendingStateSave, health.StateSaveError)
	}

	// Verify state file was actually written with the adopted auth key
	savedMgr, err := inform.NewStateManager(statePath, "00:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}
	if !savedMgr.IsAdopted() {
		t.Fatal("persisted state was not marked adopted")
	}
}
