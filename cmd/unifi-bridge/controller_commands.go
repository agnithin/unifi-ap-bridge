package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

// Adopted sessions require authentication for every state-changing reply,
// including reset and key rotation, before any dispatcher or state manager runs.
func unpackControllerResponse(raw, key []byte, mac string, adopted bool) (*inform.Packet, error) {
	packet, err := inform.UnpackInform(raw, key, mac)
	if err != nil {
		return nil, err
	}
	if adopted && packet.Mode != inform.ModeGCM {
		return nil, errors.New("adopted controller response requires authenticated GCM")
	}
	return packet, nil
}

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

type platformCmdRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

var defaultPlatformCmdRunner platformCmdRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

type macFilterSnapshot struct {
	iface   string
	mode    string
	macList []string
}

func readInterfaceMacList(ctx context.Context, iface string, runner platformCmdRunner) ([]string, error) {
	out, err := runner(ctx, "wl", "-i", iface, "mac")
	if err != nil {
		return nil, err
	}
	var macs []string
	for _, token := range strings.Fields(string(out)) {
		if token == "mac" {
			continue
		} // Broadcom wl prefixes list entries with mac.
		norm, err := inform.NormalizeMAC(token)
		if err != nil {
			return nil, fmt.Errorf("invalid driver MAC list on %s", iface)
		}
		macs = append(macs, norm)
	}
	sort.Strings(macs)
	return macs, nil
}

func snapshotInterfaceMacFilter(ctx context.Context, iface string, runner platformCmdRunner) (macFilterSnapshot, error) {
	snap := macFilterSnapshot{iface: iface}
	out, err := runner(ctx, "wl", "-i", iface, "macmode")
	if err != nil {
		return snap, err
	}
	snap.mode = strings.TrimSpace(string(out))
	if snap.mode != "0" && snap.mode != "1" && snap.mode != "2" {
		return snap, fmt.Errorf("invalid MAC mode on %s", iface)
	}
	snap.macList, err = readInterfaceMacList(ctx, iface, runner)
	return snap, err
}

func writeInterfaceMacFilter(ctx context.Context, snap macFilterSnapshot, runner platformCmdRunner) error {
	if _, err := runner(ctx, "wl", "-i", snap.iface, "mac", "none"); err != nil {
		return err
	}
	if len(snap.macList) > 0 {
		args := append([]string{"-i", snap.iface, "mac"}, snap.macList...)
		if _, err := runner(ctx, "wl", args...); err != nil {
			return err
		}
	}
	if _, err := runner(ctx, "wl", "-i", snap.iface, "macmode", snap.mode); err != nil {
		return err
	}
	actual, err := snapshotInterfaceMacFilter(ctx, snap.iface, runner)
	if err != nil {
		return err
	}
	if actual.mode != snap.mode || strings.Join(actual.macList, " ") != strings.Join(snap.macList, " ") {
		return fmt.Errorf("MAC filter readback mismatch on %s", snap.iface)
	}
	return nil
}

func rollbackMacFilter(snaps []macFilterSnapshot, runner platformCmdRunner) error {
	// The mutation deadline may have expired; restoration gets its own bounded
	// context and includes the partially modified interface that failed.
	ctx, cancel := context.WithTimeout(context.Background(), platformCommandTimeout)
	defer cancel()
	var result error
	for i := len(snaps) - 1; i >= 0; i-- {
		if err := writeInterfaceMacFilter(ctx, snaps[i], runner); err != nil {
			result = errors.Join(result, fmt.Errorf("restore %s: %w", snaps[i].iface, err))
		}
	}
	return result
}

func executePlatformMacFilter(ctx context.Context, ifaces []string, blockedMACs []string, runner platformCmdRunner) error {
	if len(ifaces) == 0 {
		return errors.New("no wireless interfaces available")
	}
	// Snapshot every interface before the first write; unreadable policy must
	// never be replaced with an invented disabled-filter default.
	snaps := make([]macFilterSnapshot, 0, len(ifaces))
	for _, iface := range ifaces {
		snap, err := snapshotInterfaceMacFilter(ctx, iface, runner)
		if err != nil {
			return fmt.Errorf("snapshot %s: %w", iface, err)
		}
		// ASUS allow lists have different semantics and must retain ownership.
		if snap.mode == "2" {
			return fmt.Errorf("refusing to replace ASUS allow list on %s", iface)
		}
		snaps = append(snaps, snap)
	}
	desired := append([]string(nil), blockedMACs...)
	sort.Strings(desired)
	mode := "0"
	if len(desired) > 0 {
		mode = "1"
	}
	for i, iface := range ifaces {
		target := macFilterSnapshot{iface: iface, mode: mode, macList: desired}
		if err := writeInterfaceMacFilter(ctx, target, runner); err != nil {
			return errors.Join(fmt.Errorf("apply MAC filter on %s: %w", iface, err), rollbackMacFilter(snaps[:i+1], runner))
		}
	}
	// Disconnect clients only after all interfaces have verified the policy.
	for _, iface := range ifaces {
		for _, mac := range desired {
			_, _ = runner(ctx, "wl", "-i", iface, "deauthenticate", mac)
		}
	}
	return nil
}

func startPlatformMacFilter(blockedMACs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), platformCommandTimeout)
	defer cancel()

	ifaces := getPlatformWirelessInterfaces()
	err := executePlatformMacFilter(ctx, ifaces, blockedMACs, defaultPlatformCmdRunner)
	if err != nil {
		logEvent("[unifi-bridge] MAC filter update failed: %v", err)
	}
	return err
}
