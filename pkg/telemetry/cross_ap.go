package telemetry

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// CrossAPObservation pairs same-station departure and arrival observations
// from different APs. It records correlated snapshots only; it cannot prove
// that one caused the other or measure client interruption/roam latency.
type CrossAPObservation struct {
	Type                  string    `json:"type"` // correlated_ap_observations
	ClientMAC             string    `json:"client_mac"`
	SourceAP              string    `json:"source_ap"`
	DestinationAP         string    `json:"destination_ap"`
	DepartureObservedAt   time.Time `json:"departure_observed_at"`
	DeparturePreviousAt   time.Time `json:"departure_previous_observed_at,omitempty"`
	ArrivalObservedAt     time.Time `json:"arrival_observed_at"`
	ArrivalPreviousAt     time.Time `json:"arrival_previous_observed_at,omitempty"`
	SourcePollGapSeconds  int64     `json:"source_poll_gap_seconds,omitempty"`
	DestinationPollGapSec int64     `json:"destination_poll_gap_seconds,omitempty"`
}

type apStationObservation struct {
	ap    string
	event StationEvent
}

// CrossAPCorrelator keeps a bounded, in-memory window of station events. Use
// it on a trusted host that already receives per-AP diagnostics; it performs
// no persistence and does not require AP-to-AP access.
type CrossAPCorrelator struct {
	mu             sync.Mutex
	window         time.Duration
	maxPending     int
	maxEvents      int
	departures     []apStationObservation
	arrivals       []apStationObservation
	events         []CrossAPObservation
	recentEventIDs map[string]struct{}
	eventIDOrder   []string
	seenPairs      map[string]struct{}
	pairKeyOrder   []string
}

// NewCrossAPCorrelator creates a bounded in-memory correlator. Non-positive
// settings use conservative defaults of a 90-second window, 256 pending
// events per direction, and 256 retained pairs.
func NewCrossAPCorrelator(window time.Duration, maxPending, maxEvents int) *CrossAPCorrelator {
	if window <= 0 {
		window = 90 * time.Second
	}
	if maxPending <= 0 {
		maxPending = 256
	}
	if maxEvents <= 0 {
		maxEvents = 256
	}
	return &CrossAPCorrelator{window: window, maxPending: maxPending, maxEvents: maxEvents, recentEventIDs: make(map[string]struct{}), seenPairs: make(map[string]struct{})}
}

// Observe consumes only AP-local observed association/disassociation events.
// Correlations are returned in the order they are discovered, even when
// timestamp polling caused the departure to arrive at the host after arrival.
func (c *CrossAPCorrelator) Observe(apID string, event StationEvent) []CrossAPObservation {
	if c == nil || strings.TrimSpace(apID) == "" || event.ObservedAt.IsZero() {
		return nil
	}
	mac := strings.ToLower(strings.TrimSpace(event.MAC))
	if mac == "" || (event.Type != "observed_association" && event.Type != "observed_disassociation") {
		return nil
	}
	event.MAC = mac
	observation := apStationObservation{ap: strings.TrimSpace(apID), event: event}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.rememberEvent(eventFingerprint(observation)) {
		return nil
	}
	c.prune(event.ObservedAt)
	newPairs := make([]CrossAPObservation, 0)
	if event.Type == "observed_disassociation" {
		for _, arrival := range c.arrivals {
			if c.matches(observation, arrival) {
				newPairs = append(newPairs, makeCrossAPPair(observation, arrival))
			}
		}
		c.departures = appendBounded(c.departures, observation, c.maxPending)
	} else {
		for _, departure := range c.departures {
			if c.matches(departure, observation) {
				newPairs = append(newPairs, makeCrossAPPair(departure, observation))
			}
		}
		c.arrivals = appendBounded(c.arrivals, observation, c.maxPending)
	}
	created := make([]CrossAPObservation, 0, len(newPairs))
	for _, pair := range newPairs {
		key := pairFingerprint(pair)
		if _, exists := c.seenPairs[key]; exists {
			continue
		}
		c.rememberPair(key, pair)
		created = append(created, pair)
	}
	return created
}

// Events returns a copy of retained correlated observations in discovery order.
func (c *CrossAPCorrelator) Events() []CrossAPObservation {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]CrossAPObservation(nil), c.events...)
}

func (c *CrossAPCorrelator) matches(departure, arrival apStationObservation) bool {
	if departure.ap == arrival.ap || departure.event.MAC != arrival.event.MAC {
		return false
	}
	delta := departure.event.ObservedAt.Sub(arrival.event.ObservedAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= c.window
}

func (c *CrossAPCorrelator) prune(now time.Time) {
	keep := func(observations []apStationObservation) []apStationObservation {
		out := observations[:0]
		for _, observation := range observations {
			if now.Sub(observation.event.ObservedAt) <= c.window {
				out = append(out, observation)
			}
		}
		return out
	}
	c.departures = keep(c.departures)
	c.arrivals = keep(c.arrivals)
}

func (c *CrossAPCorrelator) rememberEvent(key string) bool {
	if _, exists := c.recentEventIDs[key]; exists {
		return false
	}
	limit := c.maxPending * 2
	if limit < c.maxEvents {
		limit = c.maxEvents
	}
	c.recentEventIDs[key] = struct{}{}
	c.eventIDOrder = append(c.eventIDOrder, key)
	if excess := len(c.eventIDOrder) - limit; excess > 0 {
		for _, expired := range c.eventIDOrder[:excess] {
			delete(c.recentEventIDs, expired)
		}
		copy(c.eventIDOrder, c.eventIDOrder[excess:])
		c.eventIDOrder = c.eventIDOrder[:limit]
	}
	return true
}

func (c *CrossAPCorrelator) rememberPair(key string, pair CrossAPObservation) {
	c.seenPairs[key] = struct{}{}
	c.pairKeyOrder = append(c.pairKeyOrder, key)
	c.events = append(c.events, pair)
	if excess := len(c.events) - c.maxEvents; excess > 0 {
		copy(c.events, c.events[excess:])
		c.events = c.events[:c.maxEvents]
	}
	if excess := len(c.pairKeyOrder) - c.maxEvents; excess > 0 {
		for _, expired := range c.pairKeyOrder[:excess] {
			delete(c.seenPairs, expired)
		}
		copy(c.pairKeyOrder, c.pairKeyOrder[excess:])
		c.pairKeyOrder = c.pairKeyOrder[:c.maxEvents]
	}
}

func appendBounded(items []apStationObservation, observation apStationObservation, limit int) []apStationObservation {
	items = append(items, observation)
	if excess := len(items) - limit; excess > 0 {
		copy(items, items[excess:])
		items = items[:limit]
	}
	return items
}

func makeCrossAPPair(departure, arrival apStationObservation) CrossAPObservation {
	return CrossAPObservation{
		Type:                  "correlated_ap_observations",
		ClientMAC:             departure.event.MAC,
		SourceAP:              departure.ap,
		DestinationAP:         arrival.ap,
		DepartureObservedAt:   departure.event.ObservedAt,
		DeparturePreviousAt:   departure.event.PreviousObserved,
		ArrivalObservedAt:     arrival.event.ObservedAt,
		ArrivalPreviousAt:     arrival.event.PreviousObserved,
		SourcePollGapSeconds:  departure.event.PollGapSeconds,
		DestinationPollGapSec: arrival.event.PollGapSeconds,
	}
}

func eventFingerprint(observation apStationObservation) string {
	e := observation.event
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00%s", observation.ap, e.Type, e.MAC, e.ObservedAt.UnixNano(), e.PreviousObserved.UnixNano(), e.BSSID)
}

func pairFingerprint(pair CrossAPObservation) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%d", pair.ClientMAC, pair.SourceAP, pair.DepartureObservedAt.UnixNano(), pair.DestinationAP, pair.ArrivalObservedAt.UnixNano())
}
