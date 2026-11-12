package telemetry

import "encoding/hex"

// ValidLegacyWlanID reports whether id is a valid UniFi WLAN identifier:
// either a 12-byte legacy 24-hex ObjectId or a modern 36-char RFC 4122 UUID.
func ValidLegacyWlanID(id string) bool {
	if len(id) == 24 {
		_, err := hex.DecodeString(id)
		return err == nil
	}
	if len(id) == 36 {
		return isValidUUID(id)
	}
	return false
}

func isValidUUID(u string) bool {
	for i := 0; i < len(u); i++ {
		c := u[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

// AssignRadioStationCounts associates a VAP with its physical radio, not merely
// its frequency band. Legacy profiles may omit RadioName only on unique bands.
func AssignRadioStationCounts(stats []RadioStat, vaps []VAP) {
	indices := make(map[string]int)
	for i := range stats {
		stats[i].NumSta = 0
		indices[stats[i].Name] = i
	}
	for _, vap := range vaps {
		name := vap.RadioName
		if name == "" {
			name = "wifi-" + vap.Radio
			if _, ambiguous := indices[name+"-1"]; ambiguous {
				continue
			}
		}
		if i, ok := indices[name]; ok {
			stats[i].NumSta += vap.NumSta
		}
	}
}
