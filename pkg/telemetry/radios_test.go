package telemetry

import "testing"

func TestValidLegacyWlanID(t *testing.T) {
	for _, tc := range []struct {
		id    string
		valid bool
	}{
		{"6adb775a27327fe7fefdc22c", true},
		{"6ADB775A27327FE7FEFDC22C", true},
		{"550e8400-e29b-41d4-a716-446655440000", false},
		{"zzdb775a27327fe7fefdc22c", false},
		{"6adb775a27327fe7fefdc22", false},
	} {
		if got := ValidLegacyWlanID(tc.id); got != tc.valid {
			t.Errorf("ValidLegacyWlanID(%q) = %v, want %v", tc.id, got, tc.valid)
		}
	}
}

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
