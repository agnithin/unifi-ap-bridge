package telemetry

import "encoding/hex"

// ValidLegacyWlanID reports whether id is a 12-byte legacy UniFi ObjectId.
// Integration API UUIDs are a different identifier and must not be used here.
func ValidLegacyWlanID(id string) bool {
	if len(id) != 24 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
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
