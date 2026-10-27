package telemetry

import (
	"testing"
	"time"
)

func TestCrossAPCorrelatorPairsObservedDepartureAndArrivalInEitherOrder(t *testing.T) {
	correlator := NewCrossAPCorrelator(30*time.Second, 8, 8)
	t0 := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	arrival := StationEvent{Type: "observed_association", MAC: "AA:BB", ObservedAt: t0.Add(12 * time.Second), PreviousObserved: t0, PollGapSeconds: 12}
	departure := StationEvent{Type: "observed_disassociation", MAC: "aa:bb", ObservedAt: t0.Add(20 * time.Second), PreviousObserved: t0.Add(10 * time.Second), PollGapSeconds: 10}
	if pairs := correlator.Observe("node", arrival); len(pairs) != 0 {
		t.Fatalf("arrival without departure should not pair yet: %+v", pairs)
	}
	pairs := correlator.Observe("main", departure)
	if len(pairs) != 1 {
		t.Fatalf("expected one cross-AP observation pair: %+v", pairs)
	}
	got := pairs[0]
	if got.Type != "correlated_ap_observations" || got.ClientMAC != "aa:bb" || got.SourceAP != "main" || got.DestinationAP != "node" || got.DeparturePreviousAt != t0.Add(10*time.Second) || got.ArrivalPreviousAt != t0 || got.SourcePollGapSeconds != 10 || got.DestinationPollGapSec != 12 {
		t.Fatalf("unexpected cross-AP evidence: %+v", got)
	}
	if got.DepartureObservedAt != t0.Add(20*time.Second) || got.ArrivalObservedAt != t0.Add(12*time.Second) {
		t.Fatalf("poll observation times should remain explicit, not reordered: %+v", got)
	}
	if again := correlator.Observe("main", departure); len(again) != 0 {
		t.Fatalf("duplicate event produced another correlation: %+v", again)
	}
	if events := correlator.Events(); len(events) != 1 {
		t.Fatalf("event history=%d, want 1", len(events))
	}
}

func TestCrossAPCorrelatorDoesNotInventSameAPOrOutOfWindowMatches(t *testing.T) {
	correlator := NewCrossAPCorrelator(5*time.Second, 4, 4)
	t0 := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	departure := StationEvent{Type: "observed_disassociation", MAC: "client", ObservedAt: t0}
	correlator.Observe("main", departure)
	if got := correlator.Observe("main", StationEvent{Type: "observed_association", MAC: "client", ObservedAt: t0.Add(time.Second)}); len(got) != 0 {
		t.Fatalf("same AP association should not be cross-AP paired: %+v", got)
	}
	if got := correlator.Observe("node", StationEvent{Type: "observed_association", MAC: "client", ObservedAt: t0.Add(6 * time.Second)}); len(got) != 0 {
		t.Fatalf("outside-window association should not be paired: %+v", got)
	}
	if got := correlator.Observe("node", StationEvent{Type: "observed_radio_change", MAC: "client", ObservedAt: t0.Add(time.Second)}); len(got) != 0 {
		t.Fatalf("radio change is not an arrival/departure: %+v", got)
	}
	if events := correlator.Events(); len(events) != 0 {
		t.Fatalf("unexpected correlations: %+v", events)
	}
}

func TestCrossAPCorrelatorBoundsPendingAndHistory(t *testing.T) {
	correlator := NewCrossAPCorrelator(time.Minute, 2, 1)
	t0 := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for i, mac := range []string{"a", "b", "c"} {
		at := t0.Add(time.Duration(i) * time.Second)
		correlator.Observe("main", StationEvent{Type: "observed_disassociation", MAC: mac, ObservedAt: at})
		correlator.Observe("node", StationEvent{Type: "observed_association", MAC: mac, ObservedAt: at.Add(time.Second)})
	}
	if events := correlator.Events(); len(events) != 1 || events[0].ClientMAC != "c" {
		t.Fatalf("pair history exceeded capacity: %+v", events)
	}
	// Feeding irrelevant input cannot create correlations and does not crash.
	if got := correlator.Observe("", StationEvent{Type: "observed_disassociation", MAC: "x", ObservedAt: t0}); len(got) != 0 {
		t.Fatalf("empty AP identity accepted: %+v", got)
	}
}
