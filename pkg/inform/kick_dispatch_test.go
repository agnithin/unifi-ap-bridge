package inform

import (
	"errors"
	"testing"
	"time"
)

func TestKickDispatcherExecutesValidCommand(t *testing.T) {
	var kickedMAC string
	calls := 0
	dispatcher := NewKickDispatcher(func(mac string) error {
		kickedMAC = mac
		calls++
		return nil
	})

	payload := map[string]any{
		"_type": "cmd",
		"cmd":   "kick-sta",
		"mac":   "ee:28:41:7f:fa:ec",
		"_id":   "req-1",
	}

	result := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Now())
	if result.Status != "kicked" || result.Action != "kick-sta" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if calls != 1 || kickedMAC != "ee:28:41:7f:fa:ec" {
		t.Fatalf("calls=%d kickedMAC=%s", calls, kickedMAC)
	}

	// Duplicate dispatch with same _id should be ignored
	dupResult := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Now())
	if dupResult.Status != "duplicate" {
		t.Fatalf("expected duplicate, got: %+v", dupResult)
	}
	if calls != 1 {
		t.Fatalf("duplicate triggered execution: calls=%d", calls)
	}
}

func TestKickDispatcherRejectsIneligibleOrInvalid(t *testing.T) {
	dispatcher := NewKickDispatcher(func(mac string) error { return nil })

	// Ineligible (CBC / unadopted)
	payload := map[string]any{"_type": "cmd", "cmd": "kick-sta", "mac": "ee:28:41:7f:fa:ec", "_id": "req-1"}
	res := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: false, Adopted: true}, time.Now())
	if res.Status != "ineligible" {
		t.Fatalf("expected ineligible, got %+v", res)
	}

	// Invalid MAC
	invalidPayload := map[string]any{"_type": "cmd", "cmd": "kick-sta", "mac": "not-a-mac", "_id": "req-2"}
	res = dispatcher.Dispatch(invalidPayload, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Now())
	if res.Status != "invalid_mac" {
		t.Fatalf("expected invalid_mac, got %+v", res)
	}

	// Nil executor
	nilDispatcher := NewKickDispatcher(nil)
	res = nilDispatcher.Dispatch(map[string]any{"_type": "cmd", "cmd": "kick-sta", "mac": "ee:28:41:7f:fa:ec", "_id": "req-3"}, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Now())
	if res.Status != "unavailable" {
		t.Fatalf("expected unavailable, got %+v", res)
	}

	// Failing executor
	errDispatcher := NewKickDispatcher(func(mac string) error { return errors.New("deauth error") })
	res = errDispatcher.Dispatch(map[string]any{"_type": "cmd", "cmd": "kick-sta", "mac": "ee:28:41:7f:fa:ec", "_id": "req-4"}, CommandEligibility{AuthenticatedGCM: true, Adopted: true}, time.Now())
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %+v", res)
	}
}
