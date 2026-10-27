package telemetry

import (
	"testing"
	"time"
)

func TestPassiveRFHistoryBoundsAndSummarizesKnownFields(t *testing.T) {
	history := NewPassiveRFHistory(3, 2)
	start := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		obss, noise := i*10, -90+i
		history.Observe(start.Add(time.Duration(i)*time.Minute), []RadioStat{{Name: "wifi-na", CuTotal: 20 + i, CuSelfRx: 2, CuSelfTx: 3, CuOBSS: &obss, NoiseDBm: &noise}}, true)
	}
	if got := history.Trend("wifi-na", 4); got != nil {
		t.Fatalf("trend should require min samples after ring eviction: %+v", got)
	}
	trend := history.Trend("wifi-na", 3)
	if trend == nil || trend.Samples != 3 || trend.MeanBusy != 22 || trend.MeanOBSS == nil || *trend.MeanOBSS != 20 || trend.MeanNoiseDBm == nil || *trend.MeanNoiseDBm != -88 {
		t.Fatalf("unexpected trend: %+v", trend)
	}
	if trend.From != start.Add(time.Minute) || trend.Through != start.Add(3*time.Minute) {
		t.Fatalf("unexpected trend window: %+v", trend)
	}
	// Returned values do not alias retained pointer fields.
	*history.Series("wifi-na")[0].CuOBSS = 999
	if got := *history.Series("wifi-na")[0].CuOBSS; got != 10 {
		t.Fatalf("caller mutation changed history: %d", got)
	}
}

func TestPassiveRFHistoryIgnoresFailedOrAmbiguousSamples(t *testing.T) {
	history := NewPassiveRFHistory(2, 2)
	start := time.Now().UTC()
	history.Observe(start, []RadioStat{{Name: "wifi-ng", CuTotal: 11}}, true)
	history.Observe(start.Add(time.Second), []RadioStat{{Name: "wifi-ng", CuTotal: 99}}, false)
	history.Observe(start.Add(2*time.Second), []RadioStat{{Name: "wifi-ng"}, {Name: "wifi-ng"}}, true)
	history.Observe(start.Add(3*time.Second), []RadioStat{{Name: ""}}, true)
	series := history.Series("wifi-ng")
	if len(series) != 1 || series[0].CuTotal != 11 {
		t.Fatalf("failed/ambiguous sample changed history: %+v", series)
	}
	if got := history.Trend("wifi-ng", 2); got != nil {
		t.Fatalf("insufficient samples produced a trend: %+v", got)
	}
}
