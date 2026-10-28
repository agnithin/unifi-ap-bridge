package main

import (
	"context"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/capture"
)

func TestPacketCaptureCLIOptInAndFixedInterface(t *testing.T) {
	called := false
	var gotIface string
	runner := func(_ context.Context, req capture.Request) (string, error) {
		called = true
		gotIface = req.Interface
		if (req.Interface != "br0" && req.Interface != "br20") || !req.Allow || req.ClientMAC != "02:11:22:33:44:55" || req.Duration != 5*time.Second || req.PacketLimit != 20 || req.SnapLen != 96 {
			t.Fatalf("unexpected request: %+v", req)
		}
		return "/tmp/test.pcap", nil
	}
	if _, err := runPacketCaptureCLI("", "02:11:22:33:44:55", 5*time.Second, 20, 96, false, runner); err == nil {
		t.Fatal("missing explicit opt-in accepted")
	}
	if called {
		t.Fatal("opt-in rejection reached runner")
	}
	path, err := runPacketCaptureCLI("", "02:11:22:33:44:55", 5*time.Second, 20, 96, true, runner)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/test.pcap" || !called || gotIface != "br0" {
		t.Fatalf("path=%q called=%v iface=%s", path, called, gotIface)
	}

	// Test custom bridge interface
	called = false
	path, err = runPacketCaptureCLI("br20", "02:11:22:33:44:55", 5*time.Second, 20, 96, true, runner)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/test.pcap" || !called || gotIface != "br20" {
		t.Fatalf("path=%q called=%v iface=%s", path, called, gotIface)
	}
}
