package telemetry

import "math"

// AttachVAPClientSignals projects the arithmetic mean of current station dBm
// measurements into the native VAP signal field. Partial/ambiguous samples are
// omitted rather than reporting zero or pretending to be a historical average.
func AttachVAPClientSignals(vaps []VAP) {
	for i := range vaps {
		vap := &vaps[i]
		vap.AvgClientSignal = nil
		if len(vap.StaTable) == 0 || vap.NumSta != len(vap.StaTable) {
			continue
		}
		seen := make(map[string]bool, len(vap.StaTable))
		sum, valid := 0, true
		for _, sta := range vap.StaTable {
			if sta.MAC == "" || seen[sta.MAC] || sta.Signal < -127 || sta.Signal >= 0 {
				valid = false
				break
			}
			seen[sta.MAC] = true
			sum += sta.Signal
		}
		if valid {
			mean := int(math.Round(float64(sum) / float64(len(vap.StaTable))))
			vap.AvgClientSignal = &mean
		}
	}
}
