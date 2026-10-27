package telemetry

import (
	"testing"
	"time"
)

func TestStationTimelineBaselineAndObservedTransitions(t *testing.T) {
	tracker := NewStationTimeline(4, 8)
	t0 := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	station := Station{MAC: "AA:BB", Radio: "ng", Signal: -61}
	vap := VAP{Radio: "ng", BSSID: "ap-24", ESSID: "wifi", StaTable: []Station{station}}
	if got := tracker.Observe([]VAP{vap}, t0, true); len(got) != 0 {
		t.Fatalf("first sample should only seed baseline: %+v", got)
	}
	if got := tracker.Observe(nil, t0.Add(time.Second), false); len(got) != 0 {
		t.Fatalf("failed sample emitted events: %+v", got)
	}
	// A failed sample must not move the previous observation used for timing.
	if got := tracker.Observe([]VAP{{Radio: "na", BSSID: "ap-5", StaTable: []Station{{MAC: "aa:bb", Radio: "na", Signal: -70}}}}, t0.Add(21*time.Second), true); len(got) != 1 {
		t.Fatalf("expected one radio transition: %+v", got)
	} else if got[0].Type != "observed_radio_change" || got[0].PreviousBand != "2.4GHz" || got[0].Band != "5GHz" || got[0].PollGapSeconds != 21 || !got[0].SignalKnown || got[0].Signal != -70 {
		t.Fatalf("incorrect radio transition: %+v", got[0])
	}
	if got := tracker.Observe(nil, t0.Add(time.Minute), true); len(got) != 1 || got[0].Type != "observed_disassociation" {
		t.Fatalf("expected observed disassociation: %+v", got)
	} else if got[0].PreviousObserved != t0.Add(21*time.Second) {
		t.Fatalf("disconnect should use last successful observation: %+v", got[0])
	}
}

func TestStationTimelineBoundedAndAmbiguousSnapshots(t *testing.T) {
	tracker := NewStationTimeline(2, 1)
	t0 := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	first := VAP{Radio: "ng", StaTable: []Station{{MAC: "a"}}}
	tracker.Observe([]VAP{first}, t0, true)
	// Over-limit input is rejected as incomplete, preserving the prior baseline.
	tracker.Observe([]VAP{{StaTable: []Station{{MAC: "a"}, {MAC: "b"}}}}, t0.Add(time.Second), true)
	if events := tracker.Observe([]VAP{first}, t0.Add(2*time.Second), true); len(events) != 0 {
		t.Fatalf("over-limit snapshot changed baseline: %+v", events)
	}
	// Duplicate MAC rows are ambiguous and excluded from both snapshots.
	duplicate := []VAP{{StaTable: []Station{{MAC: "a"}}}, {StaTable: []Station{{MAC: "a"}}}}
	tracker.Observe(duplicate, t0.Add(3*time.Second), true)
	if events := tracker.Events(); len(events) > 2 {
		t.Fatalf("event history exceeded limit: %d", len(events))
	}
}
