package models

import "testing"

func TestCatalogSwitchProfilesMatchAdvertisedHardware(t *testing.T) {
	for _, tc := range []struct {
		code      string
		ports     int
		hasSwitch bool
	}{
		{"U6M", 1, false},
		{"U6P", 1, false},
		{"U6L", 1, false},
		{"U6IW", 4, true},
		{"U6EntIW", 4, true},
	} {
		profile, ok := Catalog[tc.code]
		if !ok {
			t.Fatalf("model %s is missing from catalog", tc.code)
		}
		if profile.PortCount != tc.ports || profile.HasSwitch != tc.hasSwitch {
			t.Errorf("model %s advertises ports=%d switch=%v, want ports=%d switch=%v", tc.code, profile.PortCount, profile.HasSwitch, tc.ports, tc.hasSwitch)
		}
		if len(profile.Radios) != 2 || profile.Radios[0].Band != "ng" || profile.Radios[1].Band != "na" {
			t.Errorf("model %s radio profile is incomplete or misordered: %+v", tc.code, profile.Radios)
		}
	}
}

func TestU6IWLegacyAliasResolvesToCanonicalProfile(t *testing.T) {
	canonical := GetProfile("U6IW")
	legacy := GetProfile("U6-IW")
	if legacy.ModelCode != canonical.ModelCode || legacy.PortCount != canonical.PortCount || legacy.HasSwitch != canonical.HasSwitch {
		t.Fatalf("legacy U6-IW profile diverged: canonical=%+v legacy=%+v", canonical, legacy)
	}
}
