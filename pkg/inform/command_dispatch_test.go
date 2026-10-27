package inform

import (
	"errors"
	"testing"
	"time"
)

func TestRebootDispatcherRequiresOptInAdoptedGCMAndAllowlistedAction(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	calls := 0
	executor := func() error { calls++; return nil }
	payload := map[string]any{"_type": "cmd", "cmd": "reboot"}
	eligible := CommandEligibility{AuthenticatedGCM: true, Adopted: true}

	for _, tc := range []struct {
		name        string
		dispatcher  *RebootDispatcher
		eligibility CommandEligibility
		payload     map[string]any
		want        string
	}{
		{name: "disabled by default", dispatcher: NewRebootDispatcher(false, executor), eligibility: eligible, payload: payload, want: "disabled"},
		{name: "CBC is not authenticated", dispatcher: NewRebootDispatcher(true, executor), eligibility: CommandEligibility{Adopted: true}, payload: payload, want: "ineligible"},
		{name: "not adopted", dispatcher: NewRebootDispatcher(true, executor), eligibility: CommandEligibility{AuthenticatedGCM: true}, payload: payload, want: "ineligible"},
		{name: "locate is not an action", dispatcher: NewRebootDispatcher(true, executor), eligibility: eligible, payload: map[string]any{"_type": "cmd", "cmd": "locate"}, want: ""},
		{name: "unknown command is ignored", dispatcher: NewRebootDispatcher(true, executor), eligibility: eligible, payload: map[string]any{"_type": "cmd", "cmd": "run-shell"}, want: ""},
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
	d := NewRebootDispatcher(true, func() error { calls++; return nil })
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

func TestRebootDispatcherAppliesCooldownWithoutRequestIDAndReservesFailures(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	calls := 0
	d := NewRebootDispatcher(true, func() error { calls++; return errors.New("private executor failure") })
	eligibility := CommandEligibility{AuthenticatedGCM: true, Adopted: true}
	payload := map[string]any{"_type": "cmd", "cmd": "reboot"}
	if got := d.Dispatch(payload, eligibility, now); got.Status != "failed" {
		t.Fatalf("unexpected failure result: %+v", got)
	}
	if got := d.Dispatch(payload, eligibility, now.Add(time.Second)); got.Status != "cooldown" {
		t.Fatalf("failed execution did not reserve cooldown: %+v", got)
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d, want 1", calls)
	}
}

func TestRebootDispatcherDoesNotLeakControllerValuesInResult(t *testing.T) {
	d := NewRebootDispatcher(true, func() error { return nil })
	got := d.Dispatch(map[string]any{
		"_type": "cmd", "cmd": "reboot", "_id": "private-id", "key": "private-key", "device_id": "private-device",
	}, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Unix(1_790_000_000, 0))
	if got != (CommandDispatchResult{Type: "cmd", Action: "reboot", Status: "started"}) {
		t.Fatalf("dispatcher returned controller data: %+v", got)
	}
}
