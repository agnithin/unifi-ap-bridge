package telemetry

// RadioStatsForInform removes fields that are local diagnostics and must not
// be sent to the controller. The returned slice is a copy so local status
// retains the complete sample.
func RadioStatsForInform(stats []RadioStat) []RadioStat {
	if stats == nil {
		return nil
	}
	result := make([]RadioStat, len(stats))
	copy(result, stats)
	for i := range result {
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
		if stat.NoiseDBm != nil {
			noise := *stat.NoiseDBm
			value.NoiseFloor = &noise
		}
		radios[i].DriverStats = value
	}
}
