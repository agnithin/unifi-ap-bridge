package telemetry

import (
	"sort"
	"sync"
	"time"
)

// PassiveRFPoint is one successful radio-stat sample. Values retain the
// driver's meaning; OBSS and total busy are not presented as additive parts.
type PassiveRFPoint struct {
	ObservedAt time.Time `json:"observed_at"`
	RadioStat
}

// PassiveRFTrend summarizes a minimum number of passive samples for one
// physical radio. A nil pointer means the source never reported that field.
type PassiveRFTrend struct {
	Radio        string    `json:"radio"`
	Samples      int       `json:"samples"`
	From         time.Time `json:"from"`
	Through      time.Time `json:"through"`
	MeanBusy     float64   `json:"mean_busy_percent"`
	MeanSelfRx   float64   `json:"mean_self_rx_percent"`
	MeanSelfTx   float64   `json:"mean_self_tx_percent"`
	MeanOBSS     *float64  `json:"mean_obss_percent,omitempty"`
	MeanNoiseDBm *float64  `json:"mean_noise_dbm,omitempty"`
}

// PassiveRFHistory retains bounded in-memory samples; it performs no scans.
type PassiveRFHistory struct {
	mu         sync.Mutex
	maxSamples int
	maxRadios  int
	byRadio    map[string][]PassiveRFPoint
}

func NewPassiveRFHistory(maxSamples, maxRadios int) *PassiveRFHistory {
	if maxSamples <= 0 {
		maxSamples = 72
	}
	if maxRadios <= 0 {
		maxRadios = 8
	}
	return &PassiveRFHistory{maxSamples: maxSamples, maxRadios: maxRadios, byRadio: make(map[string][]PassiveRFPoint)}
}

// Observe adds a complete successful sample. Invalid, failed, or over-limit
// samples do not alter history.
func (h *PassiveRFHistory) Observe(at time.Time, stats []RadioStat, success bool) {
	if h == nil || !success || at.IsZero() || len(stats) == 0 || len(stats) > h.maxRadios {
		return
	}
	seen := make(map[string]bool, len(stats))
	for _, stat := range stats {
		if stat.Name == "" || seen[stat.Name] {
			return
		}
		seen[stat.Name] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, stat := range stats {
		series := h.byRadio[stat.Name]
		point := PassiveRFPoint{ObservedAt: at, RadioStat: cloneRadioStat(stat)}
		series = append(series, point)
		if excess := len(series) - h.maxSamples; excess > 0 {
			copy(series, series[excess:])
			series = series[:h.maxSamples]
		}
		h.byRadio[stat.Name] = series
	}
}

func (h *PassiveRFHistory) Series(name string) []PassiveRFPoint {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	series := h.byRadio[name]
	out := make([]PassiveRFPoint, len(series))
	for i, point := range series {
		out[i] = point
		out[i].RadioStat = cloneRadioStat(point.RadioStat)
	}
	return out
}

// Trend returns a summary only after minSamples are present. OBSS is shown as
// its own driver-reported component and is not added to self airtime or total.
func (h *PassiveRFHistory) Trend(name string, minSamples int) *PassiveRFTrend {
	series := h.Series(name)
	if minSamples < 2 || len(series) < minSamples {
		return nil
	}
	trend := &PassiveRFTrend{Radio: name, Samples: len(series), From: series[0].ObservedAt, Through: series[len(series)-1].ObservedAt}
	var obssSum, noiseSum float64
	var obssCount, noiseCount int
	for _, point := range series {
		trend.MeanBusy += float64(point.CuTotal)
		trend.MeanSelfRx += float64(point.CuSelfRx)
		trend.MeanSelfTx += float64(point.CuSelfTx)
		if point.CuOBSS != nil {
			obssSum += float64(*point.CuOBSS)
			obssCount++
		}
		if point.NoiseDBm != nil {
			noiseSum += float64(*point.NoiseDBm)
			noiseCount++
		}
	}
	count := float64(len(series))
	trend.MeanBusy /= count
	trend.MeanSelfRx /= count
	trend.MeanSelfTx /= count
	if obssCount > 0 {
		mean := obssSum / float64(obssCount)
		trend.MeanOBSS = &mean
	}
	if noiseCount > 0 {
		mean := noiseSum / float64(noiseCount)
		trend.MeanNoiseDBm = &mean
	}
	return trend
}

// Radios returns the stable sorted names currently retained.
func (h *PassiveRFHistory) Radios() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	names := make([]string, 0, len(h.byRadio))
	for name := range h.byRadio {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cloneRadioStat(stat RadioStat) RadioStat {
	if stat.CuOBSS != nil {
		value := *stat.CuOBSS
		stat.CuOBSS = &value
	}
	if stat.NoiseDBm != nil {
		value := *stat.NoiseDBm
		stat.NoiseDBm = &value
	}
	return stat
}
