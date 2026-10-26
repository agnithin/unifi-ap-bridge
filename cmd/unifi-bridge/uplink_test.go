package main

import (
	"testing"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

func TestConfiguredUplinkFallbackRespectsVerifiedAndDownStates(t *testing.T) {
	cfg := Config{UplinkMAC: "aa:bb:cc:dd:ee:ff", UplinkRemotePort: 3}
	down := &telemetry.Uplink{LinkKnown: true, Up: false}
	applyConfiguredUplinkFallback(down, &cfg)
	if down.UplinkMAC != "" || down.UplinkRemotePort != 0 {
		t.Fatalf("configured parent resurrected a disconnected link: %+v", down)
	}
	verified := &telemetry.Uplink{LinkKnown: true, Up: true, UplinkSource: "lldp_uplink", UplinkMAC: "00:11:22:33:44:55"}
	applyConfiguredUplinkFallback(verified, &cfg)
	if verified.UplinkRemotePort != 0 {
		t.Fatalf("configured port overwrote a verified unknown port: %+v", verified)
	}
	unknown := &telemetry.Uplink{}
	applyConfiguredUplinkFallback(unknown, &cfg)
	if unknown.UplinkMAC != cfg.UplinkMAC || unknown.UplinkRemotePort != cfg.UplinkRemotePort {
		t.Fatalf("configuration fallback was not used for unknown link: %+v", unknown)
	}
	if unknown.UplinkSource != "configured_topology" {
		t.Fatalf("configured topology source is not identified: %+v", unknown)
	}
	for _, invalid := range []string{"", "not-a-mac", "01:00:5e:00:00:01", "00:00:00:00:00:00"} {
		got := &telemetry.Uplink{}
		applyConfiguredUplinkFallback(got, &Config{UplinkMAC: invalid})
		if got.UplinkMAC != "" {
			t.Errorf("invalid configured gateway identity %q accepted as %q", invalid, got.UplinkMAC)
		}
	}
	localAdmin := &telemetry.Uplink{}
	applyConfiguredUplinkFallback(localAdmin, &Config{UplinkMAC: "6e:63:f8:30:9a:05"})
	if localAdmin.UplinkMAC != "6e:63:f8:30:9a:05" {
		t.Fatalf("explicit locally administered topology was rewritten: %+v", localAdmin)
	}
}
