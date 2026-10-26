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
	if strings.Contains(encoded, "tx_bytes") || strings.Contains(encoded, "rx_bytes") || strings.Contains(encoded, "vlan") {
		t.Fatalf("unknown VAP counters or VLAN were serialized as values: %s", encoded)
	}
}

// Controller radio_table is configured intent and can retain an older width.
// Runtime bw must remain a numeric driver measurement in the inform payload.
func TestOperatingWidthIsIndependentOfConfiguredWidth(t *testing.T) {
	payload := InformPayload{
		RadioTable:      []Radio{{Name: "wifi-na-1", Radio: "na", Channel: 161, HT: "40"}},
		RadioTableStats: []RadioStat{{Name: "wifi-na-1", Channel: 161, Bandwidth: 80}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Radios []struct {
			HT string `json:"ht"`
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
		decoded.Radios[0].HT != "40" || decoded.Stats[0].Bandwidth != 80 || decoded.Stats[0].Channel != 161 {
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
