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
}
