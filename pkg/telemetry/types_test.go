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
