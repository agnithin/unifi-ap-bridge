package main

import (
	"context"
	"errors"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/capture"
)

// runPacketCaptureCLI is deliberately separate from the daemon/inform loop.
// A nil runner selects the real Linux AF_PACKET implementation; tests inject a
// harmless runner and never open a socket.
func runPacketCaptureCLI(clientMAC string, duration time.Duration, packetLimit, snapLen int, allow bool, runner func(context.Context, capture.Request) (string, error)) (string, error) {
	if !allow {
		return "", errors.New("LAN packet capture requires --allow-packet-capture")
	}
	req := capture.Request{Interface: "br0", ClientMAC: clientMAC, Duration: duration, PacketLimit: packetLimit, SnapLen: snapLen, Allow: true}
	if runner == nil {
		runner = capture.Run
	}
	return runner(context.Background(), req)
}
