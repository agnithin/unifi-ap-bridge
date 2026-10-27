package inform

import (
	"crypto/sha256"
	"sync"
	"time"
)

const (
	rebootCommandCooldown   = 60 * time.Second
	maxRememberedCommandIDs = 64
	maxCommandIDLength      = 128
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
	mu      sync.Mutex
	enabled bool
	execute RebootExecutor
	lastRun time.Time
	ids     map[[32]byte]struct{}
	idOrder [][32]byte
}

// NewRebootDispatcher configures the opt-in, fixed reboot dispatcher. Pass
// enabled=false by default. The executor is dependency-injected for safe tests.
func NewRebootDispatcher(enabled bool, execute RebootExecutor) *RebootDispatcher {
	return &RebootDispatcher{enabled: enabled, execute: execute, ids: make(map[[32]byte]struct{})}
}

// Dispatch inspects a successfully decoded controller response. It never
// handles locate or unknown commands. now is injectable for deterministic tests.
func (d *RebootDispatcher) Dispatch(payload map[string]any, eligibility CommandEligibility, now time.Time) CommandDispatchResult {
	result := CommandDispatchResult{Type: "cmd", Action: "reboot", Status: "ignored"}
	if payload == nil || payload["_type"] != "cmd" || payload["cmd"] != "reboot" {
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
	requestID, hasID := boundedRequestID(payload["_id"])
	if hasID {
		if _, seen := d.ids[requestID]; seen {
			result.Status = "duplicate"
			return result
		}
	}
	if !d.lastRun.IsZero() && now.Sub(d.lastRun) < rebootCommandCooldown {
		result.Status = "cooldown"
		return result
	}
	if d.execute == nil {
		result.Status = "unavailable"
		return result
	}

	// Reserve before invoking the executor so a repeated response cannot start
	// a second reboot if the first invocation returns an error or races a retry.
	d.lastRun = now
	if hasID {
		d.rememberID(requestID)
	}
	if err := d.execute(); err != nil {
		result.Status = "failed"
		return result
	}
	result.Status = "started"
	return result
}

func boundedRequestID(value any) ([32]byte, bool) {
	s, ok := value.(string)
	if !ok || len(s) == 0 || len(s) > maxCommandIDLength {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(s)), true
}

func (d *RebootDispatcher) rememberID(id [32]byte) {
	if len(d.idOrder) == maxRememberedCommandIDs {
		delete(d.ids, d.idOrder[0])
		d.idOrder = d.idOrder[1:]
	}
	d.ids[id] = struct{}{}
	d.idOrder = append(d.idOrder, id)
}
