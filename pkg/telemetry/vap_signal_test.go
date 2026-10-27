package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVAPSignalUsesCurrentCompleteDBmMeasurements(t *testing.T) {
	v := []VAP{{NumSta: 2, StaTable: []Station{{MAC: "02:00:00:00:00:01", Signal: -60}, {MAC: "02:00:00:00:00:02", Signal: -69}}}}
	AttachVAPClientSignals(v)
	b, _ := json.Marshal(v[0])
	if v[0].AvgClientSignal == nil || *v[0].AvgClientSignal != -65 || !strings.Contains(string(b), `"avg_client_signal":-65`) {
		t.Fatalf("incorrect signal projection: %s", b)
	}
}
func TestVAPSignalOmitsEmptyPartialInvalidAndDuplicateMeasurements(t *testing.T) {
	for _, tc := range []struct {
		name     string
		count    int
		stations []Station
	}{
		{"empty", 0, nil}, {"partial", 2, []Station{{MAC: "02:00:00:00:00:01", Signal: -60}}},
		{"unknown", 1, []Station{{MAC: "02:00:00:00:00:01", Signal: 0}}},
		{"positive", 1, []Station{{MAC: "02:00:00:00:00:01", Signal: 20}}},
		{"out of range", 1, []Station{{MAC: "02:00:00:00:00:01", Signal: -128}}},
		{"duplicate", 2, []Station{{MAC: "02:00:00:00:00:01", Signal: -60}, {MAC: "02:00:00:00:00:01", Signal: -62}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := -50
			v := []VAP{{NumSta: tc.count, StaTable: tc.stations, AvgClientSignal: &old}}
			AttachVAPClientSignals(v)
			b, _ := json.Marshal(v[0])
			if strings.Contains(string(b), "avg_client_signal") {
				t.Fatalf("unavailable signal serialized: %s", b)
			}
		})
	}
}
