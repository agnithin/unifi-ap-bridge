package main

import (
	"errors"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

// exchangeHealth is owned by the serial exchange loop.
type exchangeHealth struct {
	StationEvents      []telemetry.StationEvent
	RFTrends           []*telemetry.PassiveRFTrend
	WLANMappingStatus  string
	WLANMappingEntries int
	CommandObservation *inform.CommandObservation
	CommandResult      *inform.CommandDispatchResult
	CommandObservedAt  int64
	PendingStateSave   bool
	StateSaveError     string
	LastAttempt        time.Time
	LastSuccess        time.Time
	LastInformError    string
	LastFullSample     time.Time
	Performance        exchangePerformance

	rejections int
}

// Repeated authentication/not-found responses trigger a diagnostic only.
// Bad requests, transport failures, and other HTTP results break the sequence.
func (h *exchangeHealth) observePost(err error) bool {
	var response *inform.HTTPError
	if errors.As(err, &response) && (response.StatusCode == 401 || response.StatusCode == 404) {
		h.rejections++
		if h.rejections >= 3 {
			h.rejections = 0
			return true
		}
	} else {
		h.rejections = 0
	}
	return false
}

func (h *exchangeHealth) lastSuccessString() string {
	if h.LastSuccess.IsZero() {
		return ""
	}
	return h.LastSuccess.Format("2006-01-02 15:04:05")
}

type exchangePerformance struct {
	Kind                 string           `json:"kind"`
	CollectMS            int64            `json:"collect_ms"`
	DriverCommands       int              `json:"driver_commands"`
	Stages               map[string]int64 `json:"stage_ms,omitempty"`
	InformMS             int64            `json:"inform_ms"`
	TotalMS              int64            `json:"total_ms"`
	SampleDeadlineMisses uint64           `json:"sample_deadline_misses"`
	RoamToInformMS       *int64           `json:"roam_to_inform_ms,omitempty"`
}

// Near the next full sample, let that sample publish the pending roam event.
func roamRefreshDelay(now, lastExchange, nextFull time.Time, debounce, minInterval time.Duration) (time.Duration, bool) {
	due := now.Add(debounce)
	if earliest := lastExchange.Add(minInterval); due.Before(earliest) {
		due = earliest
	}
	return due.Sub(now), nextFull.Sub(due) > minInterval
}
