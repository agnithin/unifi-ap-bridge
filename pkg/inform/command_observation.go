package inform

// CommandObservation is the bounded, secret-free summary of a controller
// command seen in an authenticated inform response. It intentionally omits
// controller IDs, device IDs, timestamps, keys, and all unrecognized fields.
type CommandObservation struct {
	Type         string `json:"type"`
	Action       string `json:"action"`
	HasID        bool   `json:"has_id"`
	HasTimestamp bool   `json:"has_timestamp"`
}

// ObserveAuthenticatedCommand recognizes only command shapes supported by
// published inform protocol sources. Callers must pass true only after an
// authenticated AES-GCM response has been successfully decoded. CBC responses
// do not provide authentication and must not be observed through this API.
//
// Main captures confirm a top-level `_type: reboot` request and a
// `_type: cmd, cmd: set-locate` request after GCM authentication. The locate
// shape is observed only; this package does not change LED state.
func ObserveAuthenticatedCommand(payload map[string]any, authenticated bool) (CommandObservation, bool) {
	if !authenticated || payload == nil {
		return CommandObservation{}, false
	}

	typ, _ := payload["_type"].(string)
	action := ""
	switch typ {
	case "reboot": // legacy documented form
		action = "reboot"
	case "cmd":
		cmd, _ := payload["cmd"].(string)
		switch cmd {
		case "reboot":
			action = cmd
		case "set-locate":
			action = "locate"
		case "unset-locate":
			action = "stop-locate"
		default:
			return CommandObservation{}, false
		}
	default:
		return CommandObservation{}, false
	}

	return CommandObservation{
		Type:         typ,
		Action:       action,
		HasID:        nonEmptyString(payload["_id"]),
		HasTimestamp: validTimestamp(payload["time"]) || nonEmptyString(payload["datetime"]),
	}, true
}

func nonEmptyString(value any) bool {
	s, ok := value.(string)
	return ok && s != ""
}

func validTimestamp(value any) bool {
	switch v := value.(type) {
	case int:
		return v > 0
	case int32:
		return v > 0
	case int64:
		return v > 0
	case uint:
		return v > 0
	case uint32:
		return v > 0
	case uint64:
		return v > 0
	case float64:
		return v > 0
	case float32:
		return v > 0
	case string:
		return v != ""
	default:
		return false
	}
}
