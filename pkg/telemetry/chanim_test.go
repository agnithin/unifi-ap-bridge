package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseChanimDetailsUsesNamedOptionalColumns(t *testing.T) {
	raw := "version 3\nchanspec tx inbss obss nocat nopkt doze txop goodtx badtx glitch badplcp knoise idle timestamp\n0x1006 10 1 19 8 6 0 51 1 1 2441 54 -91 61 103535032\n"
	details := ParseChanimDetails(raw)
	if details.OBSSPercent == nil || *details.OBSSPercent != 19 || details.NoiseDBm == nil || *details.NoiseDBm != -91 || details.NoCatPercent == nil || *details.NoCatPercent != 8 || details.NoPktPercent == nil || *details.NoPktPercent != 6 {
		t.Fatalf("parsed details = %+v", details)
	}
	// Column order may vary; the parser follows the driver's header.
	details = ParseChanimDetails("chanspec nopkt knoise idle nocat obss\n0x1006 2 -88 60 4 0\n")
	if details.OBSSPercent == nil || *details.OBSSPercent != 0 || details.NoiseDBm == nil || *details.NoiseDBm != -88 {
		t.Fatalf("reordered details = %+v", details)
	}
	if details.NoCatPercent == nil || *details.NoCatPercent != 4 || details.NoPktPercent == nil || *details.NoPktPercent != 2 {
		t.Fatalf("reordered CCA details = %+v", details)
	}
}

func TestParseChanimDetailsOmitsUnknownAndInvalidValues(t *testing.T) {
	for _, raw := range []string{
		"chanspec tx idle\n0x1006 10 90\n",
		"chanspec obss knoise\n0x1006 no 7\n",
		"chanspec obss knoise\n0x1006 101 -151\n",
		"chanspec nocat nopkt\n0x1006 101 -1\n",
		"chanspec nocat nopkt\n0x1006 -1 101\n",
	} {
		details := ParseChanimDetails(raw)
		if details.OBSSPercent != nil || details.NoiseDBm != nil || details.NoCatPercent != nil || details.NoPktPercent != nil {
			t.Fatalf("invalid fields should be unknown, got %+v", details)
		}
	}
	data, err := json.Marshal(RadioStat{Name: "wifi-na"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "noise_dbm") || strings.Contains(string(data), "cu_obss") || strings.Contains(string(data), "cu_nocat") || strings.Contains(string(data), "cu_nopkt") {
		t.Fatalf("unknown optional data must be omitted: %s", data)
	}
}
