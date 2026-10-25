package telemetry

import "testing"

func TestPhysicalRadioCountsDoNotDoubleCount(t *testing.T) {
	stats := []RadioStat{{Name: "wifi-ng"}, {Name: "wifi-na"}, {Name: "wifi-na-1"}}
	vaps := []VAP{{Radio: "ng", RadioName: "wifi-ng", NumSta: 18}, {Radio: "na", RadioName: "wifi-na", NumSta: 8}, {Radio: "na", RadioName: "wifi-na-1", NumSta: 3}}
	AssignRadioStationCounts(stats, vaps)
	if stats[0].NumSta != 18 || stats[1].NumSta != 8 || stats[2].NumSta != 3 {
		t.Fatalf("counts: %+v", stats)
	}
	AssignRadioStationCounts(stats, []VAP{{Radio: "na", NumSta: 10}})
	if stats[1].NumSta != 0 || stats[2].NumSta != 0 {
		t.Fatal("ambiguous legacy VAP was assigned")
	}
}
