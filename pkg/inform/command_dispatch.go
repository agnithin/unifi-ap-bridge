package inform

import (
	"sync"
	"time"
)

const (
	rebootCommandCooldown   = 60 * time.Second
	maxRememberedCommandIDs = 64
)

// RebootExecutor is called only for the fixed, allowlisted reboot action. A
// production implementation should start the platform reboot command without
// accepting any command text or arguments from the controller.
type RebootExecutor func() error

// CommandEligibility is established by the caller from the current session:
// the response must have passed GCM authentication and the device must already
// be adopted under a non-default key.
type CommandEligibility struct {
	AuthenticatedGCM bool
	Adopted          bool
}

// CommandDispatchResult contains only fixed, sanitized values suitable for
// status or logs. It deliberately carries no controller-provided strings.
type CommandDispatchResult struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	Status string `json:"status"`
}

// RebootDispatcher supports only the current documented `cmd: reboot` action.
// It is disabled by default and has no generic remote-execution path.
type RebootDispatcher struct {
	mu          sync.Mutex
	enabled     bool
	execute     RebootExecutor
	statePath   string
	stateLoaded bool
	stateErr    error
	ids         map[[32]byte]struct{}
	idOrder     [][32]byte
	saveIDs     func(string, [][32]byte) error
	lastRun     time.Time
}

// NewRebootDispatcher configures the opt-in, fixed reboot dispatcher. Pass
// enabled=false by default. The executor is dependency-injected for safe tests.
func NewRebootDispatcher(enabled bool, execute RebootExecutor, statePath ...string) *RebootDispatcher {
	d := &RebootDispatcher{enabled: enabled, execute: execute, ids: make(map[[32]byte]struct{}), saveIDs: writeCommandIDState}
	if len(statePath) == 1 {
		d.statePath = statePath[0]
	} else if enabled {
		d.stateErr = errCommandIDStorePathRequired
	}
	return d
}

// Dispatch inspects a successfully decoded controller response. It never
// handles locate or unknown commands. now is injectable for deterministic tests.
func (d *RebootDispatcher) Dispatch(payload map[string]any, eligibility CommandEligibility, now time.Time) CommandDispatchResult {
	result := CommandDispatchResult{Type: "cmd", Action: "reboot", Status: "ignored"}
	if payload == nil || payloadString(payload, "_type") != "cmd" || payloadString(payload, "cmd") != "reboot" {
		return CommandDispatchResult{}
	}
	if !d.enabled {
		result.Status = "disabled"
		return result
	}
	if !eligibility.AuthenticatedGCM || !eligibility.Adopted {
		result.Status = "ineligible"
		return result
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.loadIDs(); err != nil {
		result.Status = "state_error"
		return result
	}
	requestID, hasID := commandRequestID(payload["_id"])
	if !hasID {
		result.Status = "missing_id"
		return result
	}
	if _, seen := d.ids[requestID]; seen {
		result.Status = "duplicate"
		return result
	}
	if !d.lastRun.IsZero() && now.Sub(d.lastRun) < rebootCommandCooldown {
		result.Status = "cooldown"
		return result
	}
	if d.execute == nil {
		result.Status = "unavailable"
		return result
	}

	// Persist before invoking the executor so a physical reboot cannot erase
	// the deduplication record while the controller still has this reply queued.
	nextOrder := append(append([][32]byte(nil), d.idOrder...), requestID)
	if len(nextOrder) > maxRememberedCommandIDs {
		nextOrder = nextOrder[len(nextOrder)-maxRememberedCommandIDs:]
	}
	if err := d.saveIDs(d.statePath, nextOrder); err != nil {
		result.Status = "state_error"
		return result
	}
	d.idOrder = nextOrder
	d.ids = make(map[[32]byte]struct{}, len(nextOrder))
	for _, id := range nextOrder {
		d.ids[id] = struct{}{}
	}
	d.lastRun = now
	if err := d.execute(); err != nil {
		result.Status = "failed"
		return result
	}
	result.Status = "started"
	return result
}

func payloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
