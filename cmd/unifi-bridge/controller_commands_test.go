package main

import (
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
			observation, result := authenticatedControllerCommand(&inform.Packet{Mode: tc.mode, Payload: payload}, tc.key, tc.adopted, dispatcher, time.Now())
			if calls != tc.wantCalls || (observation != nil) != tc.wantObservation {
				t.Fatalf("calls=%d observation=%v", calls, observation)
			}
			if !tc.enabled && tc.wantObservation && (result == nil || result.Status != "disabled") {
				t.Fatalf("disabled result=%v", result)
			}
		})
	}
}

func TestControllerLocateIsObservedWithoutAction(t *testing.T) {
	calls := 0
	dispatcher := inform.NewRebootDispatcher(true, func() error { calls++; return nil }, filepath.Join(t.TempDir(), "commands.json"))
	observation, result := authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: map[string]any{"_type": "cmd", "cmd": "locate", "extra": "must not be retained"}}, []byte("0123456789abcdef"), true, dispatcher, time.Now())
	if observation == nil || observation.Action != "locate" || result != nil || calls != 0 {
		t.Fatalf("observation=%v result=%v calls=%d", observation, result, calls)
	}
}
