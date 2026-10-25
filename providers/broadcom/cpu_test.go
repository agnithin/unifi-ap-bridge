package broadcom

import (
	"encoding/json"
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
	"testing"
)

func TestCPUDeltasAndUnknown(t *testing.T) {
	old := parseCPUSample("cpu 100 0 50 800 50 0 0 0 100 0\n")
	next := parseCPUSample("cpu 130 0 70 840 60 0 0 0 130 0\n")
	if old.total != 1000 || old.idle != 850 || cpuBusyPercent(old, next) != 50 {
		t.Fatalf("wrong CPU sample/delta: %+v %+v", old, next)
	}
	for _, pair := range [][2]cpuSample{{{}, next}, {old, old}, {next, old}} {
		if cpuBusyPercent(pair[0], pair[1]) != -1 {
			t.Fatal("invented CPU utilization")
		}
	}
	raw, _ := json.Marshal(telemetry.SystemStats{})
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	if _, ok := obj["cpu"]; ok {
		t.Fatal("unknown CPU must be omitted")
	}
}
func TestNoInventedNoiseOrExperience(t *testing.T) {
	p := ParseStaInfo("smoothed rssi: -65", "na")
	if p.Noise != 0 || p.RSSI != 0 {
		t.Fatal("invented noise/SNR")
	}
	if CalculateSatisfaction(0, 0, 0, 0) != 0 {
		t.Fatal("missing data produced experience score")
	}
}
