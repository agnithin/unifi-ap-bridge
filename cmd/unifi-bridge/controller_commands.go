package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

// Only authenticated replies for an already adopted, non-bootstrap identity
// reach the opt-in dispatchers. Packet.Mode is the verified response mode,
// rather than an assumption based on the mode of the outgoing request.
func authenticatedControllerCommand(
	packet *inform.Packet,
	key []byte,
	adopted bool,
	rebootDispatcher *inform.RebootDispatcher,
	locateDispatcher *inform.LocateDispatcher,
	kickDispatcher *inform.KickDispatcher,
	macFilterDispatcher *inform.MacFilterDispatcher,
	now time.Time,
) (*inform.CommandObservation, *inform.CommandDispatchResult) {
	if packet == nil || packet.Mode != inform.ModeGCM || !adopted || len(key) != 16 ||
		bytes.Equal(key, inform.DefaultBootstrapKey[:]) {
		return nil, nil
	}
	observation, ok := inform.ObserveAuthenticatedCommand(packet.Payload, true)
	if !ok {
		return nil, nil
	}
	var result inform.CommandDispatchResult
	eligibility := inform.CommandEligibility{AuthenticatedGCM: true, Adopted: true}

	switch observation.Action {
	case "reboot":
		if rebootDispatcher != nil {
			result = rebootDispatcher.Dispatch(packet.Payload, eligibility, now)
		}
	case "locate", "stop-locate":
		if locateDispatcher != nil {
			result = locateDispatcher.Dispatch(packet.Payload, eligibility, now)
		}
	case "kick-sta":
		if kickDispatcher != nil {
			result = kickDispatcher.Dispatch(packet.Payload, eligibility, now)
		}
	case "set-blocked-sta":
		if macFilterDispatcher != nil {
			result = macFilterDispatcher.Dispatch(packet.Payload, eligibility)
		}
	}

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

type platformLocateController struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	ledPath    string
	origBright string
	period     time.Duration
	timeout    time.Duration
}

var defaultLocateController = &platformLocateController{
	period:  250 * time.Millisecond,
	timeout: 15 * time.Second,
}

func (c *platformLocateController) findLEDPath() string {
	if c.ledPath != "" {
		return c.ledPath
	}
	candidates := []string{
		"/sys/class/leds/29/brightness",
		"/sys/class/leds/white/brightness",
		"/sys/class/leds/front/brightness",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			c.ledPath = p
			return p
		}
	}
	return ""
}

func (c *platformLocateController) Execute(active bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if previous worker finished on its own (e.g. natural timeout)
	if c.done != nil {
		select {
		case <-c.done:
			c.cancel = nil
			c.done = nil
			c.origBright = ""
		default:
		}
	}

	path := c.findLEDPath()
	if path == "" {
		// If no hardware LED path is found on this system, treat as no-op without error.
		return nil
	}

	if !active {
		if c.cancel != nil {
			c.cancel()
			if c.done != nil {
				<-c.done
			}
			c.cancel = nil
			c.done = nil
		}
		if c.origBright != "" {
			_ = os.WriteFile(path, []byte(c.origBright+"\n"), 0o644)
			c.origBright = ""
		}
		return nil
	}

	// If locate is already active, stop the prior worker but preserve origBright
	if c.cancel != nil {
		c.cancel()
		if c.done != nil {
			<-c.done
		}
		c.cancel = nil
		c.done = nil
	}

	// Read and save original brightness before blinking if not already preserved
	if c.origBright == "" {
		orig := "255"
		if data, err := os.ReadFile(path); err == nil {
			val := strings.TrimSpace(string(data))
			if val != "" {
				orig = val
			}
		}
		c.origBright = orig
	}

	timeout := c.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	c.cancel = cancel

	period := c.period
	if period <= 0 {
		period = 250 * time.Millisecond
	}

	done := make(chan struct{})
	c.done = done

	go func(ctx context.Context, ledPath, origVal string, d time.Duration, done chan struct{}) {
		defer close(done)
		defer func() {
			_ = os.WriteFile(ledPath, []byte(origVal+"\n"), 0o644)
		}()
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		state := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state = !state
				target := "0\n"
				if state {
					target = origVal + "\n"
					if origVal == "0" {
						target = "255\n"
					}
				}
				_ = os.WriteFile(ledPath, []byte(target), 0o644)
			}
		}
	}(ctx, path, c.origBright, period, done)

	return nil
}

func startPlatformLocate(active bool) error {
	return defaultLocateController.Execute(active)
}

const platformCommandTimeout = 3 * time.Second

func getPlatformWirelessInterfaces() []string {
	ctx, cancel := context.WithTimeout(context.Background(), platformCommandTimeout)
	defer cancel()

	candidates := []string{"eth4", "eth5", "eth6"}
	out, err := exec.CommandContext(ctx, "nvram", "get", "wl_ifnames").Output()
	if err == nil {
		if fields := strings.Fields(string(out)); len(fields) > 0 {
			candidates = fields
		}
	}
	allCandidates := append(candidates, "wl0.1", "wl0.2", "wl1.1", "wl1.2", "wl2.1", "wl2.2")

	var active []string
	for _, iface := range allCandidates {
		isupOut, isupErr := exec.CommandContext(ctx, "wl", "-i", iface, "isup").Output()
		if isupErr == nil && strings.TrimSpace(string(isupOut)) == "1" {
			active = append(active, iface)
		}
	}
	if len(active) == 0 {
		return candidates
	}
	return active
}

func startPlatformKick(mac string) error {
	normMAC, err := inform.NormalizeMAC(mac)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), platformCommandTimeout)
	defer cancel()

	ifaces := getPlatformWirelessInterfaces()
	var lastErr error
	for _, iface := range ifaces {
		if err := exec.CommandContext(ctx, "wl", "-i", iface, "deauthenticate", normMAC).Run(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func startPlatformMacFilter(blockedMACs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), platformCommandTimeout)
	defer cancel()

	ifaces := getPlatformWirelessInterfaces()
	for _, iface := range ifaces {
		expectedMode := "0"
		if len(blockedMACs) == 0 {
			if err := exec.CommandContext(ctx, "wl", "-i", iface, "mac", "none").Run(); err != nil {
				return fmt.Errorf("wl mac none failed on %s: %w", iface, err)
			}
			if err := exec.CommandContext(ctx, "wl", "-i", iface, "macmode", "0").Run(); err != nil {
				return fmt.Errorf("wl macmode 0 failed on %s: %w", iface, err)
			}
		} else {
			expectedMode = "1"
			_ = exec.CommandContext(ctx, "wl", "-i", iface, "mac", "none").Run()
			args := append([]string{"-i", iface, "mac"}, blockedMACs...)
			if err := exec.CommandContext(ctx, "wl", args...).Run(); err != nil {
				return fmt.Errorf("wl mac set failed on %s: %w", iface, err)
			}
			if err := exec.CommandContext(ctx, "wl", "-i", iface, "macmode", "1").Run(); err != nil {
				return fmt.Errorf("wl macmode 1 failed on %s: %w", iface, err)
			}
			for _, mac := range blockedMACs {
				_ = exec.CommandContext(ctx, "wl", "-i", iface, "deauthenticate", mac).Run()
			}
		}
		// Readback verification: verify driver accepted expected mode
		readback, err := exec.CommandContext(ctx, "wl", "-i", iface, "macmode").Output()
		if err == nil {
			if actual := strings.TrimSpace(string(readback)); actual != expectedMode {
				return fmt.Errorf("macmode readback mismatch on %s: got %s, want %s", iface, actual, expectedMode)
			}
		}
	}
	return nil
}

