package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
			observation, result := authenticatedControllerCommand(&inform.Packet{Mode: tc.mode, Payload: payload}, tc.key, tc.adopted, dispatcher, nil, nil, nil, time.Now())
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
			observation, result := authenticatedControllerCommand(&inform.Packet{Mode: tc.mode, Payload: payload}, key, tc.adopted, dispatcher, nil, nil, nil, time.Now())
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
	observation, result := authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "fixture-only", "time": float64(1790000000), "datetime": "2026-10-27T12:00:00Z", "extra": "must not be retained"}}, []byte("0123456789abcdef"), true, dispatcher, nil, nil, nil, time.Now())
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
	obs, res := authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: payloadSet}, key, true, nil, locateDispatcher, nil, nil, time.Now())
	if obs == nil || obs.Action != "locate" || res == nil || res.Status != "started" {
		t.Fatalf("set-locate failed: obs=%v res=%v", obs, res)
	}
	if calls != 1 || !gotActive {
		t.Fatalf("calls=%d gotActive=%v", calls, gotActive)
	}

	// unset-locate
	payloadUnset := map[string]any{"_type": "cmd", "cmd": "unset-locate", "_id": "locate-2"}
	obs, res = authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: payloadUnset}, key, true, nil, locateDispatcher, nil, nil, time.Now())
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

func TestControllerKickAndMacFilterExecution(t *testing.T) {
	key := []byte("0123456789abcdef")
	var kickedMAC string
	kickDispatcher := inform.NewKickDispatcher(func(mac string) error {
		kickedMAC = mac
		return nil
	})

	var filteredMACs []string
	macFilterDispatcher := inform.NewMacFilterDispatcher(func(blockedMACs []string) error {
		filteredMACs = blockedMACs
		return nil
	})

	// Test kick-sta
	payloadKick := map[string]any{"_type": "cmd", "cmd": "kick-sta", "mac": "ee:28:41:7f:fa:ec", "_id": "kick-1"}
	obs, res := authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: payloadKick}, key, true, nil, nil, kickDispatcher, nil, time.Now())
	if obs == nil || obs.Action != "kick-sta" || res == nil || res.Status != "kicked" {
		t.Fatalf("kick-sta failed: obs=%v res=%v", obs, res)
	}
	if kickedMAC != "ee:28:41:7f:fa:ec" {
		t.Fatalf("kickedMAC=%s", kickedMAC)
	}

	// Test blocked-sta setparam
	payloadFilter := map[string]any{"_type": "setparam", "blocked_sta": "ee:28:41:7f:fa:ec\n38:ab:41:ff:95:05\n"}
	obs, res = authenticatedControllerCommand(&inform.Packet{Mode: inform.ModeGCM, Payload: payloadFilter}, key, true, nil, nil, nil, macFilterDispatcher, time.Now())
	if obs == nil || obs.Action != "set-blocked-sta" || res == nil || res.Status != "updated" {
		t.Fatalf("blocked-sta failed: obs=%v res=%v", obs, res)
	}
	if len(filteredMACs) != 2 {
		t.Fatalf("expected 2 filtered MACs, got %v", filteredMACs)
	}
}

func TestExecutePlatformMacFilterRollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	ifaces := []string{"eth4", "eth5"}

	// State tracker
	type ifState struct {
		mode string
		macs []string
	}
	state := map[string]*ifState{
		"eth4": {mode: "0", macs: nil},
		"eth5": {mode: "0", macs: nil},
	}

	var calls []string
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdStr := strings.Join(append([]string{name}, args...), " ")
		calls = append(calls, cmdStr)

		if len(args) >= 3 && args[0] == "-i" {
			iface := args[1]
			subcmd := args[2]

			// Snapshot read
			if subcmd == "macmode" && len(args) == 3 {
				return []byte(state[iface].mode + "\n"), nil
			}
			if subcmd == "mac" && len(args) == 3 {
				return []byte(strings.Join(state[iface].macs, " ") + "\n"), nil
			}

			// Mutation: fail deliberately on eth5 macmode 1
			if iface == "eth5" && subcmd == "macmode" && len(args) == 4 && args[3] == "1" {
				return nil, errors.New("simulated driver failure on eth5")
			}

			if subcmd == "mac" && len(args) > 3 {
				if args[3] == "none" {
					state[iface].macs = nil
				} else {
					state[iface].macs = args[3:]
				}
				return nil, nil
			}
			if subcmd == "macmode" && len(args) == 4 {
				state[iface].mode = args[3]
				return nil, nil
			}
			if subcmd == "deauthenticate" {
				return nil, nil
			}
		}
		return nil, nil
	}

	blocked := []string{"ee:28:41:7f:fa:ec"}
	err := executePlatformMacFilter(ctx, ifaces, blocked, mockRunner)
	if err == nil {
		t.Fatal("expected error from simulated driver failure, got nil")
	}

	// Verify rollback occurred on eth4: eth4 must be restored to mode 0 and mac none
	hasRollbackEth4 := false
	for _, call := range calls {
		if strings.Contains(call, "-i eth4 macmode 0") {
			hasRollbackEth4 = true
		}
	}
	if !hasRollbackEth4 {
		t.Fatalf("expected rollback call for eth4, got calls: %v", calls)
	}
}

func TestMacFilterTransactionRestoresFailingInterface(t *testing.T) {
	for _, fault := range []string{"snapshot", "clear", "set", "mode", "readback", "timeout"} {
		t.Run(fault, func(t *testing.T) {
			original := map[string]macFilterSnapshot{
				"eth4": {iface: "eth4", mode: "0", macList: []string{"00:11:22:33:44:44"}},
				"eth5": {iface: "eth5", mode: "1", macList: []string{"00:11:22:33:44:55"}},
			}
			states := map[string]macFilterSnapshot{}
			for k, v := range original {
				states[k] = v
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failed, mutated := false, false
			var kicks int
			runner := func(callCtx context.Context, name string, args ...string) ([]byte, error) {
				if err := callCtx.Err(); err != nil {
					return nil, err
				}
				iface, op := args[1], args[2]
				st := states[iface]
				writing := len(args) > 3
				hit := iface == "eth5" && !failed && ((fault == "snapshot" && !writing && !mutated) ||
					(fault == "clear" && op == "mac" && writing && args[3] == "none") ||
					(fault == "set" && op == "mac" && writing && args[3] != "none") ||
					(fault == "mode" && op == "macmode" && writing) ||
					((fault == "readback" || fault == "timeout") && op == "macmode" && !writing && mutated))
				if hit {
					failed = true
					if fault == "timeout" {
						cancel()
						return nil, context.Canceled
					}
					return nil, errors.New("injected failure")
				}
				if op == "deauthenticate" {
					kicks++
					return nil, nil
				}
				if !writing {
					if op == "macmode" {
						return []byte(st.mode), nil
					}
					return []byte(strings.Join(st.macList, " ")), nil
				}
				mutated = true
				if op == "macmode" {
					st.mode = args[3]
				} else if args[3] == "none" {
					st.macList = nil
				} else {
					st.macList = append([]string(nil), args[3:]...)
				}
				states[iface] = st
				return nil, nil
			}
			err := executePlatformMacFilter(ctx, []string{"eth4", "eth5"}, []string{"00:11:22:33:44:66"}, runner)
			if err == nil || !failed {
				t.Fatal("fault was not rejected")
			}
			if !reflect.DeepEqual(states, original) {
				t.Fatalf("policy not restored: %+v", states)
			}
			if kicks != 0 {
				t.Fatal("clients disconnected before transaction committed")
			}
			if fault == "snapshot" && mutated {
				t.Fatal("mutated without complete snapshots")
			}
		})
	}
}

func TestMacFilterRejectsAllowListAndSilentDriverFailure(t *testing.T) {
	for _, mode := range []string{"2", "unexpected", "0"} {
		writes := 0
		runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if len(args) > 3 {
				writes++
				return nil, nil
			} // silently ignore writes
			if args[2] == "macmode" {
				return []byte(mode), nil
			}
			return nil, nil
		}
		if err := executePlatformMacFilter(context.Background(), []string{"eth4"}, []string{"00:11:22:33:44:66"}, runner); err == nil {
			t.Fatal("accepted unsafe mode or ignored policy")
		}
		if mode != "0" && writes != 0 {
			t.Fatal("modified unreadable or ASUS allow policy")
		}
	}
}

func TestControllerStateRepliesRequireGCMWhenAdopted(t *testing.T) {
	key := []byte("0123456789abcdef")
	mac := "fc:34:97:46:7b:10"
	for _, mode := range []inform.CipherMode{inform.ModeCBC, inform.ModeGCM} {
		raw, err := inform.PackInform(map[string]any{"_type": "reset"}, mac, key, inform.PackOptions{Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := unpackControllerResponse(raw, key, mac, true); (err == nil) != (mode == inform.ModeGCM) {
			t.Fatalf("adopted reply mode %s: %v", mode, err)
		}
		if _, err := unpackControllerResponse(raw, key, mac, false); err != nil {
			t.Fatalf("bootstrap mode %s: %v", mode, err)
		}
	}
}
