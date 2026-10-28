package inform

import (
	"sync"
	"time"
)

// LocateExecutor is invoked to start (active=true) or stop (active=false)
// locating the physical device.
type LocateExecutor func(active bool) error

// LocateDispatcher handles authenticated `set-locate` and `unset-locate` commands.
// It tracks in-memory deduplication of request IDs so duplicate replies
// do not trigger redundant actions.
type LocateDispatcher struct {
	mu      sync.Mutex
	execute LocateExecutor
	lastID  [32]byte
	hasID   bool
	active  bool
}

// NewLocateDispatcher creates a new LocateDispatcher with the given executor.
func NewLocateDispatcher(execute LocateExecutor) *LocateDispatcher {
	return &LocateDispatcher{execute: execute}
}

// Dispatch processes a verified inform reply command for locate actions.
func (d *LocateDispatcher) Dispatch(payload map[string]any, eligibility CommandEligibility, now time.Time) CommandDispatchResult {
	if payload == nil {
		return CommandDispatchResult{}
	}
	typ := payloadString(payload, "_type")
	if typ != "cmd" {
		return CommandDispatchResult{}
	}
	cmd := payloadString(payload, "cmd")
	var active bool
	var action string
	switch cmd {
	case "set-locate":
		active = true
		action = "locate"
	case "unset-locate":
		active = false
		action = "stop-locate"
	default:
		return CommandDispatchResult{}
	}

	result := CommandDispatchResult{Type: typ, Action: action, Status: "ignored"}
	if !eligibility.AuthenticatedGCM || !eligibility.Adopted {
		result.Status = "ineligible"
		return result
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	requestID, hasID := commandRequestID(payload["_id"])
	if !hasID {
		result.Status = "missing_id"
		return result
	}
	if d.hasID && d.lastID == requestID {
		result.Status = "duplicate"
		return result
	}
	d.lastID = requestID
	d.hasID = true

	if d.execute == nil {
		result.Status = "unavailable"
		return result
	}

	if err := d.execute(active); err != nil {
		result.Status = "failed"
		return result
	}
	d.active = active
	result.Status = "started"
	return result
}
