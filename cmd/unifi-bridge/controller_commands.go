package main

import (
	"bytes"
	"os/exec"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

// Only authenticated replies for an already adopted, non-bootstrap identity
// reach the opt-in dispatcher. Packet.Mode is the verified response mode,
// rather than an assumption based on the mode of the outgoing request.
func authenticatedControllerCommand(packet *inform.Packet, key []byte, adopted bool, dispatcher *inform.RebootDispatcher, now time.Time) (*inform.CommandObservation, *inform.CommandDispatchResult) {
	if packet == nil || packet.Mode != inform.ModeGCM || !adopted || len(key) != 16 ||
		bytes.Equal(key, inform.DefaultBootstrapKey[:]) {
		return nil, nil
	}
	observation, ok := inform.ObserveAuthenticatedCommand(packet.Payload, true)
	if !ok {
		return nil, nil
	}
	if dispatcher == nil {
		return &observation, nil
	}
	result := dispatcher.Dispatch(packet.Payload, inform.CommandEligibility{AuthenticatedGCM: true, Adopted: true}, now)
	if result.Type == "" {
		return &observation, nil
	}
	return &observation, &result
}

// No controller-provided executable or arguments enter this fixed operation.
// The dispatcher defaults off. Runtime/firmware acceptance is a separate step.
func startPlatformReboot() error {
	command := exec.Command("/sbin/reboot")
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
