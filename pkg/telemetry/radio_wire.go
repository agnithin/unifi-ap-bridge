package telemetry

// RadioStatsForInform maps local CCA diagnostics to standard inform telemetry
// and removes driver-internal diagnostic fields. The returned slice is a copy
// so local status retains the complete raw sample.
func RadioStatsForInform(stats []RadioStat) []RadioStat {
	if stats == nil {
		return nil
	}
	result := make([]RadioStat, len(stats))
	copy(result, stats)
	for i := range result {
		if result[i].CuInterf == nil && result[i].CCAInterference != nil {
			v := *result[i].CCAInterference
			if v >= 0 && v <= 100 {
				result[i].CuInterf = &v
			}
		}
		result[i].CCAInterference = nil
	}
	return result
}

// AttachRadioDriverStats supplies the native nested inform location consumed
// by the controller, in addition to radio_table_stats used by local status.
// Identity matching is exact: two 5 GHz radios must never share measurements.
func AttachRadioDriverStats(radios []Radio, stats []RadioStat) {
	byName := make(map[string]RadioStat, len(stats))
	duplicate := make(map[string]bool)
	for _, stat := range stats {
		if _, exists := byName[stat.Name]; exists {
			duplicate[stat.Name] = true
		}
		byName[stat.Name] = stat
	}
	for i := range radios {
		radios[i].DriverStats = nil
		stat, ok := byName[radios[i].Name]
		if !ok || duplicate[radios[i].Name] || stat.Name == "" || stat.Channel != radios[i].Channel ||
			stat.CuTotal < 0 || stat.CuTotal > 100 || stat.CuSelfRx < 0 || stat.CuSelfRx > 100 || stat.CuSelfTx < 0 || stat.CuSelfTx > 100 {
			continue
		}
		value := &RadioDriverStats{Name: stat.Name, CuTotal: stat.CuTotal, CuSelfRx: stat.CuSelfRx, CuSelfTx: stat.CuSelfTx}
		interf := stat.CuInterf
		if interf == nil {
			interf = stat.CCAInterference
		}
		if interf != nil && *interf >= 0 && *interf <= 100 {
			v := *interf
			value.CuInterf = &v
			radios[i].CuInterf = &v
		}
		if stat.NoiseDBm != nil {
			noise := *stat.NoiseDBm
			value.NoiseFloor = &noise
		}
		radios[i].DriverStats = value
	}
}
