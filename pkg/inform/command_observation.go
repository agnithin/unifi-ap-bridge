package inform

// CommandObservation is the bounded, secret-free summary of a controller
// command seen in an authenticated inform response. It intentionally omits
// controller IDs, device IDs, timestamps, keys, and all unrecognized fields.
type CommandObservation struct {
	Type         string `json:"type"`
	Action       string `json:"action"`
	HasID        bool   `json:"has_id"`
	HasTimestamp bool   `json:"has_timestamp"`
	TargetMAC    string `json:"target_mac,omitempty"`
}

// ObserveAuthenticatedCommand recognizes only command shapes supported by
// published inform protocol sources. Callers must pass true only after an
// authenticated AES-GCM response has been successfully decoded. CBC responses
// do not provide authentication and must not be observed through this API.
//
// Main captures confirm a top-level `_type: reboot` request, a
// `_type: cmd, cmd: set-locate` request, and a `_type: cmd, cmd: kick-sta`
// request after GCM authentication.
func ObserveAuthenticatedCommand(payload map[string]any, authenticated bool) (CommandObservation, bool) {
	if !authenticated || payload == nil {
		return CommandObservation{}, false
	}

	typ, _ := payload["_type"].(string)
	action := ""
	targetMAC := ""

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
		case "kick-sta":
			action = "kick-sta"
			if rawMAC, ok := payload["mac"].(string); ok {
				if normMAC, err := NormalizeMAC(rawMAC); err == nil {
					targetMAC = normMAC
				}
			}
		default:
			return CommandObservation{}, false
		}
	case "setparam", "setstate":
		if _, ok := payload["blocked_sta"]; ok {
			action = "set-blocked-sta"
		} else {
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
		TargetMAC:    targetMAC,
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
