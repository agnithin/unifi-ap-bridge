package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

func TestControllerCommandsRequireActualGCMAdoptedPrivateIdentity(t *testing.T) {
	key := []byte("0123456789abcdef")
	payload := map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "fixture-only"}
	for _, tc := range []struct {
		name            string
		mode            inform.CipherMode
		key             []byte
		adopted         bool
		enabled         bool
		wantCalls       int
		wantObservation bool
	}{
		{"opt_in", inform.ModeGCM, key, true, true, 1, true},
		{"default_off", inform.ModeGCM, key, true, false, 0, true},
		{"cbc", inform.ModeCBC, key, true, true, 0, false},
		{"not_adopted", inform.ModeGCM, key, false, true, 0, false},
		{"bootstrap", inform.ModeGCM, inform.DefaultBootstrapKey[:], true, true, 0, false},
		{"invalid_key", inform.ModeGCM, nil, true, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			dispatcher := inform.NewRebootDispatcher(tc.enabled, func() error { calls++; return nil }, filepath.Join(t.TempDir(), "commands.json"))
			observation, result := authenticatedControllerCommand(&inform.Packet{Mode: tc.mode, Payload: payload}, tc.key, tc.adopted, dispatcher, nil, time.Now())
			if calls != tc.wantCalls || (observation != nil) != tc.wantObservation {
				t.Fatalf("calls=%d observation=%v", calls, observation)
			}
			if !tc.enabled && tc.wantObservation && (result == nil || result.Status != "disabled") {
				t.Fatalf("disabled result=%v", result)
			}
		})
	}
}

func TestControllerObservedTopLevelRebootEnvelopeRemainsDefaultOffAndGated(t *testing.T) {
	key := []byte("0123456789abcdef")
	payload := map[string]any{"_type": "reboot", "_id": "observed-fixture", "time": float64(1790000000), "datetime": "2026-10-27T12:00:00Z"}
	for _, tc := range []struct {
		name    string
		mode    inform.CipherMode
		adopted bool
		enabled bool
		want    int
		status  string
	}{
		{"opt-in adopted GCM", inform.ModeGCM, true, true, 1, "started"},
		{"default off", inform.ModeGCM, true, false, 0, "disabled"},
		{"CBC", inform.ModeCBC, true, true, 0, ""},
		{"not adopted", inform.ModeGCM, false, true, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			dispatcher := inform.NewRebootDispatcher(tc.enabled, func() error { calls++; return nil }, filepath.Join(t.TempDir(), "commands.json"))
			observation, result := authenticatedControllerCommand(&inform.Packet{Mode: tc.mode, Payload: payload}, key, tc.adopted, dispatcher, nil, time.Now())
			if calls != tc.want {
				t.Fatalf("executor calls=%d want=%d", calls, tc.want)
			}
			if tc.status == "" {
				if observation != nil || result != nil {
					t.Fatalf("ineligible envelope was processed: observation=%v result=%v", observation, result)
				}
				return
			}
			if observation == nil || observation.Type != "reboot" || observation.Action != "reboot" || result == nil || result.Status != tc.status || result.Type != "reboot" {
				t.Fatalf("unexpected handling: observation=%+v result=%+v", observation, result)
			}
		})
	}
}

func TestControllerLocateIsObservedWithoutAction(t *testing.T) {
	calls := 0
	dispatcher := inform.NewRebootDispatcher(true, func() error { calls++; return nil }, filepath.Join(t.TempDir(), "commands.json"))
	observation, result := authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "fixture-only", "time": float64(1790000000), "datetime": "2026-10-27T12:00:00Z", "extra": "must not be retained"}}, []byte("0123456789abcdef"), true, dispatcher, nil, time.Now())
	if observation == nil || observation.Action != "locate" || result != nil || calls != 0 {
		t.Fatalf("observation=%v result=%v calls=%d", observation, result, calls)
	}
}

func TestControllerLocateExecution(t *testing.T) {
	var gotActive bool
	calls := 0
	locateDispatcher := inform.NewLocateDispatcher(func(active bool) error {
		gotActive = active
		calls++
		return nil
	})
	key := []byte("0123456789abcdef")

	// set-locate
	payloadSet := map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "locate-1"}
	obs, res := authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: payloadSet}, key, true, nil, locateDispatcher, time.Now())
	if obs == nil || obs.Action != "locate" || res == nil || res.Status != "started" {
		t.Fatalf("set-locate failed: obs=%v res=%v", obs, res)
	}
	if calls != 1 || !gotActive {
		t.Fatalf("calls=%d gotActive=%v", calls, gotActive)
	}

	// unset-locate
	payloadUnset := map[string]any{"_type": "cmd", "cmd": "unset-locate", "_id": "locate-2"}
	obs, res = authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: payloadUnset}, key, true, nil, locateDispatcher, time.Now())
	if obs == nil || obs.Action != "stop-locate" || res == nil || res.Status != "started" {
		t.Fatalf("unset-locate failed: obs=%v res=%v", obs, res)
	}
	if calls != 2 || gotActive {
		t.Fatalf("calls=%d gotActive=%v", calls, gotActive)
	}
}

func TestPlatformLocateController(t *testing.T) {
	t.Run("basic start stop", func(t *testing.T) {
		tmpDir := t.TempDir()
		ledFile := filepath.Join(tmpDir, "brightness")
		if err := os.WriteFile(ledFile, []byte("255\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		ctrl := &platformLocateController{
			ledPath: ledFile,
			period:  10 * time.Millisecond,
			timeout: 80 * time.Millisecond,
		}

		// Start locate
		if err := ctrl.Execute(true); err != nil {
			t.Fatalf("Execute(true) failed: %v", err)
		}

		// Allow several blinks
		time.Sleep(35 * time.Millisecond)

		// Stop locate
		if err := ctrl.Execute(false); err != nil {
			t.Fatalf("Execute(false) failed: %v", err)
		}

		// Verify brightness is restored to 255
		data, err := os.ReadFile(ledFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "255\n" {
			t.Fatalf("expected brightness 255\\n, got %q", string(data))
		}
	})

	t.Run("repeated start then stop preserves original brightness", func(t *testing.T) {
		tmpDir := t.TempDir()
		ledFile := filepath.Join(tmpDir, "brightness")
		if err := os.WriteFile(ledFile, []byte("255\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		ctrl := &platformLocateController{
			ledPath: ledFile,
			period:  10 * time.Millisecond,
			timeout: 100 * time.Millisecond,
		}

		if err := ctrl.Execute(true); err != nil {
			t.Fatalf("Execute(true) failed: %v", err)
		}
		time.Sleep(15 * time.Millisecond) // mid-blink

		// Second start while active
		if err := ctrl.Execute(true); err != nil {
			t.Fatalf("second Execute(true) failed: %v", err)
		}
		time.Sleep(15 * time.Millisecond)

		// Stop
		if err := ctrl.Execute(false); err != nil {
			t.Fatalf("Execute(false) failed: %v", err)
		}

		data, err := os.ReadFile(ledFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "255\n" {
			t.Fatalf("expected brightness 255\\n, got %q", string(data))
		}
	})

	t.Run("natural timeout restores brightness", func(t *testing.T) {
		tmpDir := t.TempDir()
		ledFile := filepath.Join(tmpDir, "brightness")
		if err := os.WriteFile(ledFile, []byte("180\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		ctrl := &platformLocateController{
			ledPath: ledFile,
			period:  10 * time.Millisecond,
			timeout: 40 * time.Millisecond,
		}

		if err := ctrl.Execute(true); err != nil {
			t.Fatalf("Execute(true) failed: %v", err)
		}

		// Wait for timeout to expire and defer to run
		time.Sleep(60 * time.Millisecond)

		data, err := os.ReadFile(ledFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "180\n" {
			t.Fatalf("expected brightness 180\\n, got %q", string(data))
		}

		// Subsequent stop should be clean no-op
		if err := ctrl.Execute(false); err != nil {
			t.Fatalf("Execute(false) after timeout failed: %v", err)
		}
	})

	t.Run("initially off LED restores to off", func(t *testing.T) {
		tmpDir := t.TempDir()
		ledFile := filepath.Join(tmpDir, "brightness")
		if err := os.WriteFile(ledFile, []byte("0\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		ctrl := &platformLocateController{
			ledPath: ledFile,
			period:  10 * time.Millisecond,
			timeout: 80 * time.Millisecond,
		}

		if err := ctrl.Execute(true); err != nil {
			t.Fatalf("Execute(true) failed: %v", err)
		}
		time.Sleep(25 * time.Millisecond)

		if err := ctrl.Execute(false); err != nil {
			t.Fatalf("Execute(false) failed: %v", err)
		}

		data, err := os.ReadFile(ledFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "0\n" {
			t.Fatalf("expected brightness 0\\n, got %q", string(data))
		}
	})
}
