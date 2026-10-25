package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectPreservesFullProfileTelemetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	data := `{"mac":"00:11:22:33:44:55","ip":"10.0.0.2","hostname":"test-ap","model":"XT8","firmware":"1.0","clients":["00:11:22:33:44:66"],"vaps":[{"name":"ath0","radio":"wifi-ng","bssid":"00:11:22:33:44:77","ssid":"test","channel":6}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := NewProfileProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	sample, err := provider.Collect(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sample.Radios) == 0 || len(sample.VAPs) != 1 || len(sample.RadioStats) == 0 || len(sample.Ports) == 0 || len(sample.Ethernet) == 0 || sample.Uplink == nil || len(sample.LLDP) == 0 || sample.System == nil || sample.LegacySystem == nil {
		t.Fatalf("profile collection omitted existing telemetry sections: %+v", sample)
	}
}
