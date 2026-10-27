package telemetry

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// StationEvent describes a transition observed between successful AP-local
// station snapshots. ObservedAt is the poll time, not a precise association
// time. The AP cannot infer the client's destination after it disappears.
type StationEvent struct {
	Type             string    `json:"type"` // observed_association, observed_disassociation, observed_radio_change
	MAC              string    `json:"mac"`
	ObservedAt       time.Time `json:"observed_at"`
	PreviousObserved time.Time `json:"previous_observed,omitempty"`
	PollGapSeconds   int64     `json:"poll_gap_seconds,omitempty"`
	Radio            string    `json:"radio,omitempty"`
	PreviousRadio    string    `json:"previous_radio,omitempty"`
	Band             string    `json:"band,omitempty"`
	PreviousBand     string    `json:"previous_band,omitempty"`
	BSSID            string    `json:"bssid,omitempty"`
	ESSID            string    `json:"essid,omitempty"`
	Signal           int       `json:"signal,omitempty"`
	SignalKnown      bool      `json:"signal_known,omitempty"`
}

// StationTimeline retains a bounded AP-local history. An event is emitted
// only when both sides of the transition are covered by successful samples.
type StationTimeline struct {
	mu          sync.Mutex
	maxEvents   int
	maxStations int
	stations    map[string]stationObservation
	events      []StationEvent
	initialized bool
}

type stationObservation struct {
	station Station
	at      time.Time
}

// NewStationTimeline creates a tracker with bounded event and station state.
// Non-positive limits use conservative defaults.
func NewStationTimeline(maxEvents, maxStations int) *StationTimeline {
	if maxEvents <= 0 {
		maxEvents = 256
	}
	if maxStations <= 0 {
		maxStations = 2048
	}
	return &StationTimeline{maxEvents: maxEvents, maxStations: maxStations, stations: make(map[string]stationObservation)}
}

// Observe consumes a complete AP-local snapshot. Pass success=false when
// collection failed; failed snapshots never create disconnect events or move
// the baseline. The first successful snapshot establishes a baseline only.
func (t *StationTimeline) Observe(vaps []VAP, at time.Time, success bool) []StationEvent {
	if t == nil || !success || at.IsZero() {
		return nil
	}
	current, complete := snapshotStations(vaps, t.maxStations)
	if !complete {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for mac, observation := range current {
		observation.at = at
		current[mac] = observation
	}
	if !t.initialized {
		t.stations = current
		t.initialized = true
		return nil
	}
	keys := make(map[string]struct{}, len(t.stations)+len(current))
	for mac := range t.stations {
		keys[mac] = struct{}{}
	}
	for mac := range current {
		keys[mac] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for mac := range keys {
		ordered = append(ordered, mac)
	}
	sort.Strings(ordered)
	newEvents := make([]StationEvent, 0)
	for _, mac := range ordered {
		old, wasPresent := t.stations[mac]
		now, isPresent := current[mac]
		switch {
		case !wasPresent && isPresent:
			newEvents = append(newEvents, makeStationEvent("observed_association", now.station, time.Time{}, at))
		case wasPresent && !isPresent:
			newEvents = append(newEvents, makeStationEvent("observed_disassociation", old.station, old.at, at))
		case wasPresent && isPresent && (old.station.Radio != now.station.Radio || old.station.BSSID != now.station.BSSID):
			event := makeStationEvent("observed_radio_change", now.station, old.at, at)
			event.PreviousRadio = old.station.Radio
			event.PreviousBand = stationBand(old.station.Radio)
			newEvents = append(newEvents, event)
		}
	}
	for i := range newEvents {
		t.events = append(t.events, newEvents[i])
	}
	if excess := len(t.events) - t.maxEvents; excess > 0 {
		copy(t.events, t.events[excess:])
		t.events = t.events[:t.maxEvents]
	}
	t.stations = current
	return append([]StationEvent(nil), newEvents...)
}

// Events returns a copy of the retained events in observation order.
func (t *StationTimeline) Events() []StationEvent {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]StationEvent(nil), t.events...)
}

func snapshotStations(vaps []VAP, limit int) (map[string]stationObservation, bool) {
	result := make(map[string]stationObservation)
	duplicates := make(map[string]bool)
	for _, vap := range vaps {
		for _, station := range vap.StaTable {
			mac := strings.ToLower(strings.TrimSpace(station.MAC))
			if mac == "" {
				continue
			}
			station.MAC = mac
			if station.Radio == "" {
				station.Radio = vap.Radio
			}
			if station.BSSID == "" {
				station.BSSID = vap.BSSID
			}
			if station.ESSID == "" {
				station.ESSID = vap.ESSID
			}
			if _, exists := result[mac]; exists {
				duplicates[mac] = true
				continue
			}
			result[mac] = stationObservation{station: station}
		}
	}
	if len(duplicates) > 0 {
		return nil, false // Ambiguity makes absence unsafe to interpret as disconnect.
	}
	if len(result) > limit {
		return nil, false
	}
	return result, true
}

func makeStationEvent(kind string, station Station, previous, at time.Time) StationEvent {
	event := StationEvent{Type: kind, MAC: station.MAC, ObservedAt: at, Radio: station.Radio, Band: stationBand(station.Radio), BSSID: station.BSSID, ESSID: station.ESSID}
	if station.Signal != 0 {
		event.Signal, event.SignalKnown = station.Signal, true
	}
	if !previous.IsZero() {
		event.PreviousObserved = previous
		if at.After(previous) {
			event.PollGapSeconds = int64(at.Sub(previous).Round(time.Second) / time.Second)
		}
	}
	return event
}

func stationBand(radio string) string {
	switch strings.ToLower(radio) {
	case "ng":
		return "2.4GHz"
	case "na":
		return "5GHz"
	default:
		return ""
	}
}
