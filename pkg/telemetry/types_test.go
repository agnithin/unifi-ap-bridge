package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnknownVAPCountersAndVLANAreOmitted(t *testing.T) {
	data, err := json.Marshal(struct {
		VAP  VAP          `json:"vap"`
		Port PortMacEntry `json:"port"`
	}{VAP: VAP{Name: "ath1", ESSID: "test"}, Port: PortMacEntry{MAC: "02:00:00:00:00:11"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	if strings.Contains(encoded, "tx_bytes") || strings.Contains(encoded, "rx_bytes") || strings.Contains(encoded, "tx_success") || strings.Contains(encoded, "tx_total") || strings.Contains(encoded, "wifi_tx_attempts") || strings.Contains(encoded, "wifi_tx_dropped") || strings.Contains(encoded, "vlan") {
		t.Fatalf("unknown VAP counters or VLAN were serialized as values: %s", encoded)
	}
}

func TestVAPIntervalCounterProjectionKeepsMeasuredZeroesAndHidesStationInternals(t *testing.T) {
	zero := int64(0)
	data, err := json.Marshal(VAP{
		TxSuccess: &zero, TxTotal: &zero, TxRetries: &zero, WiFiTxAttempts: &zero, WiFiTxDropped: &zero,
		StaTable: []Station{{MAC: "02:00:00:00:00:11", TxSuccessPackets: 10, TxSuccessPacketsPresent: true, TxRetryExhausted: 1, TxRetryExhaustedPresent: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, field := range []string{`"tx_success":0`, `"tx_total":0`, `"tx_retries":0`, `"wifi_tx_attempts":0`, `"wifi_tx_dropped":0`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("known zero interval field %s missing: %s", field, encoded)
		}
	}
	if strings.Contains(encoded, "TxSuccessPackets") || strings.Contains(encoded, "TxRetryExhausted") || strings.Contains(encoded, "tx_success_packets") {
		t.Fatalf("station cumulative source internals leaked to wire: %s", encoded)
	}
}

// Controller radio_table is configured intent and can retain an older width.
// Runtime bw must remain a numeric driver measurement in the inform payload.
func TestOperatingWidthIsIndependentOfConfiguredWidth(t *testing.T) {
	payload := InformPayload{
		RadioTable:      []Radio{{Name: "wifi-na-1", Radio: "na", Channel: 161, HT: "40", Is11ac: true}},
		RadioTableStats: []RadioStat{{Name: "wifi-na-1", Channel: 161, Bandwidth: 80}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Radios []struct {
			HT     string `json:"ht"`
			Is11ac bool   `json:"is_11ac"`
		} `json:"radio_table"`
		Stats []struct {
			Bandwidth int `json:"bw"`
			Channel   int `json:"channel"`
		} `json:"radio_table_stats"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Radios) != 1 || len(decoded.Stats) != 1 ||
		decoded.Radios[0].HT != "40" || !decoded.Radios[0].Is11ac || decoded.Stats[0].Bandwidth != 80 || decoded.Stats[0].Channel != 161 {
		t.Fatalf("configured width replaced or runtime width missing: %s", data)
	}
	data, err = json.Marshal(RadioStat{Name: "unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"bw"`) {
		t.Fatalf("unknown bandwidth reported as a measurement: %s", data)
	}
}

func TestCCAInterferenceMapsToCuInterfAndStripsDriverInternalFields(t *testing.T) {
	measured := 4
	stats := []RadioStat{{Name: "wifi-ng", Channel: 6, CCAInterference: &measured}}
	local, err := json.Marshal(map[string]any{"radio_stats": stats})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(local), `"cca_interference":4`) {
		t.Fatalf("local status omitted measured CCA interference: %s", local)
	}
	payload, err := json.Marshal(InformPayload{RadioTableStats: RadioStatsForInform(stats)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"cu_interf":4`) {
		t.Fatalf("inform payload missing cu_interf: %s", payload)
	}
	if strings.Contains(string(payload), "cca_interference") {
		t.Fatalf("local CCA driver-internal field leaked into inform payload: %s", payload)
	}
	if stats[0].CCAInterference == nil || *stats[0].CCAInterference != measured {
		t.Fatal("inform projection mutated local status sample")
	}
}

func TestRadioStatsForInformPreservesNilAndEmptySlices(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input []RadioStat
		want  string
	}{
		{name: "nil", input: nil, want: `"radio_table_stats":null`},
		{name: "empty", input: []RadioStat{}, want: `"radio_table_stats":[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := InformPayload{RadioTableStats: RadioStatsForInform(tc.input)}
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("payload = %s, want %s", data, tc.want)
			}
		})
	}
}
