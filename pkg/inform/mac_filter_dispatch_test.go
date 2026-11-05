package inform

import (
	"errors"
	"reflect"
	"testing"
)

func TestMacFilterDispatcherUpdatesDenyList(t *testing.T) {
	var applied []string
	calls := 0
	dispatcher := NewMacFilterDispatcher(func(blockedMACs []string) error {
		applied = blockedMACs
		calls++
		return nil
	})

	payload := map[string]any{
		"_type":       "setparam",
		"blocked_sta": "ee:28:41:7f:fa:ec\n38:ab:41:ff:95:05\n\n",
	}

	res := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: true, Adopted: true})
	if res.Status != "updated" || res.Action != "blocked-sta" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	expected := []string{"38:ab:41:ff:95:05", "ee:28:41:7f:fa:ec"}
	if !reflect.DeepEqual(applied, expected) {
		t.Fatalf("expected %v, got %v", expected, applied)
	}

	// Unchanged payload
	resUnchanged := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: true, Adopted: true})
	if resUnchanged.Status != "unchanged" {
		t.Fatalf("expected unchanged, got %+v", resUnchanged)
	}
	if calls != 1 {
		t.Fatalf("redundant execution on unchanged list: calls=%d", calls)
	}

	// Empty list (clearing all blocks)
	payloadEmpty := map[string]any{
		"_type":       "setparam",
		"blocked_sta": "",
	}
	resEmpty := dispatcher.Dispatch(payloadEmpty, CommandEligibility{AuthenticatedGCM: true, Adopted: true})
	if resEmpty.Status != "updated" {
		t.Fatalf("expected updated for clear, got %+v", resEmpty)
	}
	if calls != 2 || len(applied) != 0 {
		t.Fatalf("failed to clear blocks: calls=%d applied=%v", calls, applied)
	}
}

func TestMacFilterDispatcherRejectsIneligible(t *testing.T) {
	dispatcher := NewMacFilterDispatcher(func(blockedMACs []string) error { return nil })

	payload := map[string]any{"_type": "setparam", "blocked_sta": "ee:28:41:7f:fa:ec"}
	res := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: false, Adopted: true})
	if res.Status != "ineligible" {
		t.Fatalf("expected ineligible, got %+v", res)
	}

	errDispatcher := NewMacFilterDispatcher(func(blockedMACs []string) error { return errors.New("write error") })
	res = errDispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: true, Adopted: true})
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %+v", res)
	}
}

func TestMacFilterDispatcherFailsClosedOnMalformedMAC(t *testing.T) {
	calls := 0
	dispatcher := NewMacFilterDispatcher(func(blockedMACs []string) error {
		calls++
		return nil
	})

	payload := map[string]any{
		"_type":       "setparam",
		"blocked_sta": "ee:28:41:7f:fa:ec\nnot-a-mac\n",
	}
	res := dispatcher.Dispatch(payload, CommandEligibility{AuthenticatedGCM: true, Adopted: true})
	if res.Status != "invalid_mac" {
		t.Fatalf("expected invalid_mac, got %+v", res)
	}
	if calls != 0 {
		t.Fatalf("executor should not have been called on malformed input: calls=%d", calls)
	}
}
