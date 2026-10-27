package inform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRebootDispatcherRequiresOptInAdoptedGCMAndAllowlistedAction(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	calls := 0
	executor := func() error { calls++; return nil }
	statePath := filepath.Join(t.TempDir(), "controller-command-state.json")
	payload := map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request-eligible"}
	eligible := CommandEligibility{AuthenticatedGCM: true, Adopted: true}

	for _, tc := range []struct {
		name        string
		dispatcher  *RebootDispatcher
		eligibility CommandEligibility
		payload     map[string]any
		want        string
	}{
		{name: "disabled by default", dispatcher: NewRebootDispatcher(false, executor, statePath), eligibility: eligible, payload: payload, want: "disabled"},
		{name: "CBC is not authenticated", dispatcher: NewRebootDispatcher(true, executor, filepath.Join(t.TempDir(), "cbc.json")), eligibility: CommandEligibility{Adopted: true}, payload: payload, want: "ineligible"},
		{name: "not adopted", dispatcher: NewRebootDispatcher(true, executor, filepath.Join(t.TempDir(), "pending.json")), eligibility: CommandEligibility{AuthenticatedGCM: true}, payload: payload, want: "ineligible"},
		{name: "locate is not an action", dispatcher: NewRebootDispatcher(true, executor, filepath.Join(t.TempDir(), "locate.json")), eligibility: eligible, payload: map[string]any{"_type": "cmd", "cmd": "locate"}, want: ""},
		{name: "unknown command is ignored", dispatcher: NewRebootDispatcher(true, executor, filepath.Join(t.TempDir(), "unknown.json")), eligibility: eligible, payload: map[string]any{"_type": "cmd", "cmd": "run-shell"}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls
			got := tc.dispatcher.Dispatch(tc.payload, tc.eligibility, now)
			if got.Status != tc.want {
				t.Fatalf("status=%q, want %q", got.Status, tc.want)
			}
			if calls != before {
				t.Fatal("executor ran for disabled, ineligible, or unallowlisted response")
			}
		})
	}
}

func TestRebootDispatcherStartsOnceAndBoundsDuplicateAndCooldown(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	calls := 0
	d := NewRebootDispatcher(true, func() error { calls++; return nil }, filepath.Join(t.TempDir(), "commands.json"))
	eligibility := CommandEligibility{AuthenticatedGCM: true, Adopted: true}
	payload := map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request-1", "key": "never-log-this"}

	if got := d.Dispatch(payload, eligibility, now); got.Status != "started" || got.Type != "cmd" || got.Action != "reboot" {
		t.Fatalf("unexpected start result: %+v", got)
	}
	if got := d.Dispatch(payload, eligibility, now.Add(time.Second)); got.Status != "duplicate" {
		t.Fatalf("repeated controller id was not rejected: %+v", got)
	}
	second := map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request-2"}
	if got := d.Dispatch(second, eligibility, now.Add(2*time.Second)); got.Status != "cooldown" {
		t.Fatalf("reboot cooldown was not enforced: %+v", got)
	}
	if got := d.Dispatch(second, eligibility, now.Add(rebootCommandCooldown)); got.Status != "started" {
		t.Fatalf("reboot did not resume after cooldown: %+v", got)
	}
	if calls != 2 {
		t.Fatalf("executor calls=%d, want 2", calls)
	}
}

func TestRebootDispatcherAcceptsObservedTopLevelGCMEnvelope(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	calls := 0
	path := filepath.Join(t.TempDir(), "commands.json")
	d := NewRebootDispatcher(true, func() error { calls++; return nil }, path)
	eligibility := CommandEligibility{AuthenticatedGCM: true, Adopted: true}
	payload := map[string]any{
		"_type": "reboot", "_id": "observed-fixture-id", "time": float64(now.Unix()),
		"datetime": "2026-10-27T12:00:00Z",
	}
	if got := d.Dispatch(payload, eligibility, now); got != (CommandDispatchResult{Type: "reboot", Action: "reboot", Status: "started"}) {
		t.Fatalf("observed top-level form was not handled safely: %+v", got)
	}
	if got := d.Dispatch(payload, eligibility, now.Add(time.Second)); got != (CommandDispatchResult{Type: "reboot", Action: "reboot", Status: "duplicate"}) {
		t.Fatalf("observed request ID was not deduplicated: %+v", got)
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d, want 1", calls)
	}

	// The legacy cmd envelope remains independently recognized.
	legacy := NewRebootDispatcher(false, nil, filepath.Join(t.TempDir(), "disabled.json"))
	if got := legacy.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "other"}, eligibility, now); got != (CommandDispatchResult{Type: "cmd", Action: "reboot", Status: "disabled"}) {
		t.Fatalf("existing cmd envelope changed: %+v", got)
	}
}

func TestRebootDispatcherReservesFailedAttemptAndAppliesCooldown(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	calls := 0
	d := NewRebootDispatcher(true, func() error { calls++; return errors.New("private executor failure") }, filepath.Join(t.TempDir(), "commands.json"))
	eligibility := CommandEligibility{AuthenticatedGCM: true, Adopted: true}
	payload := map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "failed-executor"}
	if got := d.Dispatch(payload, eligibility, now); got.Status != "failed" {
		t.Fatalf("unexpected failure result: %+v", got)
	}
	payload["_id"] = "second-request"
	if got := d.Dispatch(payload, eligibility, now.Add(time.Second)); got.Status != "cooldown" {
		t.Fatalf("failed execution did not reserve cooldown: %+v", got)
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d, want 1", calls)
	}
}

func TestRebootDispatcherDoesNotLeakControllerValuesInResult(t *testing.T) {
	d := NewRebootDispatcher(true, func() error { return nil }, filepath.Join(t.TempDir(), "commands.json"))
	got := d.Dispatch(map[string]any{
		"_type": "cmd", "cmd": "reboot", "_id": "private-id", "key": "private-key", "device_id": "private-device",
	}, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Unix(1_790_000_000, 0))
	if got != (CommandDispatchResult{Type: "cmd", Action: "reboot", Status: "started"}) {
		t.Fatalf("dispatcher returned controller data: %+v", got)
	}
}

func TestRebootDispatcherPersistsIDsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-command-state.json")
	eligibility := CommandEligibility{AuthenticatedGCM: true, Adopted: true}
	payload := map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "persistent-request"}
	calls := 0
	first := NewRebootDispatcher(true, func() error { calls++; return nil }, path)
	if got := first.Dispatch(payload, eligibility, time.Unix(1_790_000_000, 0)); got.Status != "started" {
		t.Fatalf("first command not started: %+v", got)
	}
	second := NewRebootDispatcher(true, func() error { calls++; return nil }, path)
	if got := second.Dispatch(payload, eligibility, time.Unix(1_790_000_000, 0)); got.Status != "duplicate" {
		t.Fatalf("reinstantiated dispatcher did not reject persisted ID: %+v", got)
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d, want 1", calls)
	}
}

func TestRebootDispatcherFailedSaveDoesNotExecute(t *testing.T) {
	d := NewRebootDispatcher(true, func() error { t.Fatal("executor ran after save failure"); return nil }, filepath.Join(t.TempDir(), "commands.json"))
	d.saveIDs = func(string, [][32]byte) error { return errors.New("private write failure") }
	got := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request"},
		CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Unix(1_790_000_000, 0))
	if got.Status != "state_error" {
		t.Fatalf("unexpected status after save failure: %+v", got)
	}
}

func TestRebootDispatcherRequiresRequestIDAndFailsClosedOnMalformedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-command-state.json")
	if err := os.WriteFile(path, []byte(`{"schema":9,"ids":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	d := NewRebootDispatcher(true, func() error { calls++; return nil }, path)
	eligibility := CommandEligibility{AuthenticatedGCM: true, Adopted: true}
	if got := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request"}, eligibility, time.Unix(1_790_000_000, 0)); got.Status != "state_error" {
		t.Fatalf("malformed state was not rejected: %+v", got)
	}
	if got := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot"}, eligibility, time.Unix(1_790_000_000, 0)); got.Status != "state_error" {
		t.Fatalf("malformed state should remain fail-closed: %+v", got)
	}
	if calls != 0 {
		t.Fatalf("executor calls=%d, want 0", calls)
	}
}

func TestRebootDispatcherFailsClosedOnNonPrivateStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-command-state.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"ids":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	d := NewRebootDispatcher(true, func() error { calls++; return nil }, path)
	got := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request"},
		CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Unix(1_790_000_000, 0))
	if got.Status != "state_error" || calls != 0 {
		t.Fatalf("non-private state did not fail closed: result=%+v calls=%d", got, calls)
	}
}

func TestRebootDispatcherRequiresRequestID(t *testing.T) {
	d := NewRebootDispatcher(true, func() error { t.Fatal("executor ran without persistent request ID"); return nil }, filepath.Join(t.TempDir(), "commands.json"))
	got := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot"},
		CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Unix(1_790_000_000, 0))
	if got.Status != "missing_id" {
		t.Fatalf("unexpected status without request ID: %+v", got)
	}
}

func TestDisabledRebootDispatcherDoesNotTouchState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "commands.json")
	d := NewRebootDispatcher(false, func() error { t.Fatal("disabled executor ran"); return nil }, path)
	got := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot", "_id": "request"},
		CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Unix(1_790_000_000, 0))
	if got.Status != "disabled" {
		t.Fatalf("unexpected disabled status: %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled dispatcher touched state path: %v", err)
	}
}
