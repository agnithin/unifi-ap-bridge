package inform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestObserveAuthenticatedCommandSanitizesDocumentedShapes(t *testing.T) {
	tests := []struct {
		name       string
		payload    map[string]any
		wantType   string
		wantAction string
		wantID     bool
		wantTime   bool
	}{
		{
			name: "current cmd reboot",
			payload: map[string]any{"_type": "cmd", "cmd": "reboot", "key": "secret",
				"_id": "controller-request", "time": float64(1790000000), "device_id": "device"},
			wantType: "cmd", wantAction: "reboot", wantID: true, wantTime: true,
		},
		{
			name:     "observed top-level reboot",
			payload:  map[string]any{"_type": "reboot", "_id": "request", "time": float64(1790000000), "datetime": "2026-10-26T12:00:00Z"},
			wantType: "reboot", wantAction: "reboot", wantID: true, wantTime: true,
		},
		{
			name:     "observed set-locate",
			payload:  map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "request", "time": float64(1790000000), "datetime": "2026-10-26T12:00:00Z"},
			wantType: "cmd", wantAction: "locate", wantID: true, wantTime: true,
		},
		{
			name:     "observed unset-locate",
			payload:  map[string]any{"_type": "cmd", "cmd": "unset-locate", "_id": "request", "time": float64(1790000000), "datetime": "2026-10-26T12:00:00Z"},
			wantType: "cmd", wantAction: "stop-locate", wantID: true, wantTime: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ObserveAuthenticatedCommand(tt.payload, true)
			if !ok {
				t.Fatal("did not recognize documented observation")
			}
			if got.Type != tt.wantType || got.Action != tt.wantAction || got.HasID != tt.wantID || got.HasTimestamp != tt.wantTime {
				t.Fatalf("unexpected observation: %+v", got)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"secret", "controller-request", "device", "1790000000", "2026-10-26"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("observation leaked source field %q: %s", secret, encoded)
				}
			}
		})
	}
}

func TestObserveAuthenticatedCommandRejectsUnauthenticatedAndUnknownShapes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		payload       map[string]any
		authenticated bool
	}{
		{name: "unauthenticated", payload: map[string]any{"_type": "cmd", "cmd": "reboot"}},
		{name: "unknown action", payload: map[string]any{"_type": "cmd", "cmd": "run-shell"}, authenticated: true},
		{name: "unknown type", payload: map[string]any{"_type": "setparam", "cmd": "reboot"}, authenticated: true},
		{name: "missing action", payload: map[string]any{"_type": "cmd"}, authenticated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := ObserveAuthenticatedCommand(tc.payload, tc.authenticated); ok || got != (CommandObservation{}) {
				t.Fatalf("unexpected observation: %+v, recognized=%v", got, ok)
			}
		})
	}
}

func TestObserveAuthenticatedCommandOnlyRecordsValidMetadataPresence(t *testing.T) {
	got, ok := ObserveAuthenticatedCommand(map[string]any{
		"_type": "cmd", "cmd": "set-locate", "_id": map[string]any{"bad": true}, "time": false,
	}, true)
	if !ok {
		t.Fatal("expected allowed action")
	}
	if got.HasID || got.HasTimestamp {
		t.Fatalf("invalid metadata marked present: %+v", got)
	}
}
