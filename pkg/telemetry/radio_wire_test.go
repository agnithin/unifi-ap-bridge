package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeRadioStatsAreNestedAndMatchExactPhysicalIdentity(t *testing.T) {
	noise := -89
	radios := []Radio{{Name: "wifi-ng", Channel: 6}, {Name: "wifi-na", Channel: 36}, {Name: "wifi-na-1", Channel: 161}}
	stats := []RadioStat{{Name: "wifi-na-1", Channel: 161, CuTotal: 47, CuSelfRx: 2, CuSelfTx: 7, NoiseDBm: &noise}, {Name: "wifi-ng", Channel: 6, CuTotal: 48, CuSelfRx: 7, CuSelfTx: 13}, {Name: "wifi-na", Channel: 36, CuTotal: 0}}
	AttachRadioDriverStats(radios, stats)
	b, err := json.Marshal(InformPayload{RadioTable: radios})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Radios []struct {
			Name  string            `json:"name"`
			Stats *RadioDriverStats `json:"athstats"`
		} `json:"radio_table"`
	}
	if err = json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	for _, radio := range wire.Radios {
		if radio.Stats == nil {
			t.Fatalf("missing native stats for %s: %s", radio.Name, b)
		}
	}
	if len(wire.Radios) != 3 || wire.Radios[0].Stats.CuTotal != 48 || wire.Radios[1].Stats.CuTotal != 0 || wire.Radios[2].Stats.CuTotal != 47 || *wire.Radios[2].Stats.NoiseFloor != -89 {
		t.Fatalf("wrong native projection: %s", b)
	}
	for _, r := range wire.Radios {
		if r.Stats.Name != r.Name {
			t.Fatal("physical identity mismatch")
		}
	}
	if strings.Contains(string(b), "n_tx_retries") || strings.Contains(string(b), "satisfaction") {
		t.Fatal("invented counter or score")
	}
	noise = -1
	if *radios[2].DriverStats.NoiseFloor != -89 {
		t.Fatal("projection aliases source noise")
	}
}

func TestNativeRadioStatsOmitUnavailableAmbiguousAndInvalidSamples(t *testing.T) {
	cases := []struct {
		name  string
		stats []RadioStat
	}{
		{"missing", nil},
		{"other identity", []RadioStat{{Name: "wifi-na-1", Channel: 36}}},
		{"changed channel", []RadioStat{{Name: "wifi-na", Channel: 40}}},
		{"duplicate", []RadioStat{{Name: "wifi-na", Channel: 36}, {Name: "wifi-na", Channel: 36}}},
		{"negative", []RadioStat{{Name: "wifi-na", Channel: 36, CuTotal: -1}}},
		{"out of range", []RadioStat{{Name: "wifi-na", Channel: 36, CuSelfTx: 101}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			radios := []Radio{{Name: "wifi-na", Channel: 36, DriverStats: &RadioDriverStats{CuTotal: 55}}}
			AttachRadioDriverStats(radios, tc.stats)
			b, _ := json.Marshal(radios)
			if strings.Contains(string(b), "athstats") {
				t.Fatalf("unknown presented as measurement: %s", b)
			}
		})
	}
}
