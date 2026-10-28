package inform

import (
	"errors"
	"testing"
	"time"
)

func TestLocateDispatcher(t *testing.T) {
	eligible := CommandEligibility{AuthenticatedGCM: true, Adopted: true}

	t.Run("set-locate starts locate", func(t *testing.T) {
		var gotActive bool
		calls := 0
		d := NewLocateDispatcher(func(active bool) error {
			gotActive = active
			calls++
			return nil
		})

		payload := map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "id-1"}
		res := d.Dispatch(payload, eligible, time.Now())
		if res.Type != "cmd" || res.Action != "locate" || res.Status != "started" {
			t.Fatalf("unexpected result: %+v", res)
		}
		if calls != 1 || !gotActive {
			t.Fatalf("calls=%d gotActive=%v", calls, gotActive)
		}

		// Duplicate ID returns duplicate
		res = d.Dispatch(payload, eligible, time.Now())
		if res.Status != "duplicate" {
			t.Fatalf("expected duplicate, got %+v", res)
		}
		if calls != 1 {
			t.Fatalf("calls should not increase on duplicate: %d", calls)
		}

		// unset-locate stops locate
		payloadStop := map[string]any{"_type": "cmd", "cmd": "unset-locate", "_id": "id-2"}
		res = d.Dispatch(payloadStop, eligible, time.Now())
		if res.Type != "cmd" || res.Action != "stop-locate" || res.Status != "started" {
			t.Fatalf("unexpected stop result: %+v", res)
		}
		if calls != 2 || gotActive {
			t.Fatalf("calls=%d gotActive=%v", calls, gotActive)
		}
	})

	t.Run("ineligible requests rejected", func(t *testing.T) {
		calls := 0
		d := NewLocateDispatcher(func(active bool) error { calls++; return nil })
		payload := map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "id-1"}

		for _, inelig := range []CommandEligibility{
			{AuthenticatedGCM: false, Adopted: true},
			{AuthenticatedGCM: true, Adopted: false},
			{AuthenticatedGCM: false, Adopted: false},
		} {
			res := d.Dispatch(payload, inelig, time.Now())
			if res.Status != "ineligible" {
				t.Fatalf("expected ineligible, got %+v", res)
			}
		}
		if calls != 0 {
			t.Fatalf("ineligible calls=%d", calls)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		d := NewLocateDispatcher(func(active bool) error { return nil })
		payload := map[string]any{"_type": "cmd", "cmd": "set-locate"}
		res := d.Dispatch(payload, eligible, time.Now())
		if res.Status != "missing_id" {
			t.Fatalf("expected missing_id, got %+v", res)
		}
	})

	t.Run("executor error returns failed", func(t *testing.T) {
		d := NewLocateDispatcher(func(active bool) error { return errors.New("hardware error") })
		payload := map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "id-err"}
		res := d.Dispatch(payload, eligible, time.Now())
		if res.Status != "failed" {
			t.Fatalf("expected failed, got %+v", res)
		}
	})

	t.Run("nil executor returns unavailable", func(t *testing.T) {
		d := NewLocateDispatcher(nil)
		payload := map[string]any{"_type": "cmd", "cmd": "set-locate", "_id": "id-nil"}
		res := d.Dispatch(payload, eligible, time.Now())
		if res.Status != "unavailable" {
			t.Fatalf("expected unavailable, got %+v", res)
		}
	})

	t.Run("unrelated command ignored", func(t *testing.T) {
		d := NewLocateDispatcher(func(active bool) error { return nil })
		res := d.Dispatch(map[string]any{"_type": "cmd", "cmd": "reboot"}, eligible, time.Now())
		if res.Type != "" {
			t.Fatalf("expected empty result for non-locate command, got %+v", res)
		}
	})
}
