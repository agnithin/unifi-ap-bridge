package inform

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// KickExecutor is invoked to deauthenticate a station MAC from the physical AP.
type KickExecutor func(mac string) error

// KickDispatcher handles authenticated `kick-sta` commands from the controller.
// It tracks in-memory deduplication of request IDs so duplicate replies
// do not trigger redundant deauth storms.
type KickDispatcher struct {
	mu      sync.Mutex
	execute KickExecutor
	lastID  [32]byte
	hasID   bool
	lastMAC string
}

// NewKickDispatcher creates a new KickDispatcher with the given executor.
func NewKickDispatcher(execute KickExecutor) *KickDispatcher {
	return &KickDispatcher{execute: execute}
}

// Dispatch processes a verified inform reply command for kick actions.
func (d *KickDispatcher) Dispatch(payload map[string]any, eligibility CommandEligibility, now time.Time) CommandDispatchResult {
	if payload == nil {
		return CommandDispatchResult{}
	}
	typ := payloadString(payload, "_type")
	if typ != "cmd" {
		return CommandDispatchResult{}
	}
	cmd := payloadString(payload, "cmd")
	if cmd != "kick-sta" {
		return CommandDispatchResult{}
	}

	result := CommandDispatchResult{Type: typ, Action: "kick-sta", Status: "ignored"}
	if !eligibility.AuthenticatedGCM || !eligibility.Adopted {
		result.Status = "ineligible"
		return result
	}

	rawMAC := payloadString(payload, "mac")
	normMAC, err := NormalizeMAC(rawMAC)
	if err != nil || normMAC == "" {
		result.Status = "invalid_mac"
		return result
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	requestID, hasID := commandRequestID(payload["_id"])
	if !hasID {
		if timeVal := payload["time"]; timeVal != nil {
			requestID = sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%v", cmd, normMAC, timeVal)))
			hasID = true
		} else {
			requestID = sha256.Sum256([]byte(fmt.Sprintf("%s:%s", cmd, normMAC)))
			hasID = true
		}
	}
	if d.hasID && d.lastID == requestID {
		result.Status = "duplicate"
		return result
	}
	d.lastID = requestID
	d.hasID = true
	d.lastMAC = normMAC

	if d.execute == nil {
		result.Status = "unavailable"
		return result
	}

	if err := d.execute(normMAC); err != nil {
		result.Status = "failed"
		return result
	}
	result.Status = "kicked"
	return result
}
