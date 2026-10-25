package telemetry

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
