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
	rejections         int
}

// Only repeated authentication/not-found responses permit adoption recovery.
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
