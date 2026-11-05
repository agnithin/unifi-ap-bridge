package inform

import (
	"sort"
	"strings"
	"sync"
)

// MacFilterExecutor is invoked when the set of blocked client MACs changes.
type MacFilterExecutor func(blockedMACs []string) error

// MacFilterDispatcher manages the active blocked client list received from the controller.
type MacFilterDispatcher struct {
	mu          sync.Mutex
	execute     MacFilterExecutor
	lastBlocked []string
	initialized bool
}

// NewMacFilterDispatcher creates a new MacFilterDispatcher.
func NewMacFilterDispatcher(execute MacFilterExecutor) *MacFilterDispatcher {
	return &MacFilterDispatcher{execute: execute}
}

// Dispatch parses blocked_sta from a setparam/setstate envelope and updates the MAC filter.
func (d *MacFilterDispatcher) Dispatch(payload map[string]any, eligibility CommandEligibility) CommandDispatchResult {
	if payload == nil {
		return CommandDispatchResult{}
	}
	typ := payloadString(payload, "_type")
	if typ != "setparam" && typ != "setstate" {
		return CommandDispatchResult{}
	}

	rawBlocked, hasBlocked := payload["blocked_sta"].(string)
	if !hasBlocked {
		return CommandDispatchResult{}
	}

	result := CommandDispatchResult{Type: typ, Action: "blocked-sta", Status: "ignored"}
	if !eligibility.AuthenticatedGCM || !eligibility.Adopted {
		result.Status = "ineligible"
		return result
	}

	macMap := make(map[string]struct{})
	for _, line := range strings.Split(rawBlocked, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		norm, err := NormalizeMAC(line)
		if err != nil || norm == "" {
			result.Status = "invalid_mac"
			return result
		}
		macMap[norm] = struct{}{}
	}

	macList := make([]string, 0, len(macMap))
	for m := range macMap {
		macList = append(macList, m)
	}
	sort.Strings(macList)

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if identical to last applied set
	if d.initialized && sameStringSlices(d.lastBlocked, macList) {
		result.Status = "unchanged"
		return result
	}

	if d.execute == nil {
		result.Status = "unavailable"
		return result
	}

	if err := d.execute(macList); err != nil {
		result.Status = "failed"
		return result
	}

	d.lastBlocked = macList
	d.initialized = true
	result.Status = "updated"
	return result
}

func sameStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
