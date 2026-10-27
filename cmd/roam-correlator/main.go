package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

const (
	maxAPFiles        = 8
	maxStatusBytes    = 256 * 1024
	maxStatusEvents   = 64
	maxInputEvents    = maxAPFiles * maxStatusEvents
	defaultRoamWindow = 90 * time.Second
)

var apNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)
var apMACPattern = regexp.MustCompile(`(?i)^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$`)

type apFileFlag []string

func (a *apFileFlag) String() string { return strings.Join(*a, ",") }
func (a *apFileFlag) Set(value string) error {
	*a = append(*a, value)
	return nil
}

type apInput struct {
	name string
	path string
}

type observedEvent struct {
	ap    string
	event telemetry.StationEvent
}

type statusSnapshot struct {
	APMAC         string          `json:"mac"`
	StationEvents json.RawMessage `json:"station_events"`
}

type outputDocument struct {
	EvidenceScope string         `json:"evidence_scope"`
	Window        string         `json:"correlation_window"`
	Pairs         []redactedPair `json:"pairs"`
}

type redactedPair struct {
	Type                     string    `json:"type"`
	ClientTag                string    `json:"client_tag"`
	SourceAP                 string    `json:"source_ap"`
	DestinationAP            string    `json:"destination_ap"`
	SourceRadio              string    `json:"source_radio,omitempty"`
	DestinationRadio         string    `json:"destination_radio,omitempty"`
	SourcePhysicalRadio      string    `json:"source_physical_radio,omitempty"`
	DestinationPhysicalRadio string    `json:"destination_physical_radio,omitempty"`
	SourceBand               string    `json:"source_band,omitempty"`
	DestinationBand          string    `json:"destination_band,omitempty"`
	SourceBSSID              string    `json:"source_bssid,omitempty"`
	DestinationBSSID         string    `json:"destination_bssid,omitempty"`
	DepartureObservedAt      time.Time `json:"departure_observed_at"`
	DeparturePreviousAt      time.Time `json:"departure_previous_observed_at,omitempty"`
	ArrivalObservedAt        time.Time `json:"arrival_observed_at"`
	ArrivalPreviousAt        time.Time `json:"arrival_previous_observed_at,omitempty"`
	SourcePollGapSeconds     int64     `json:"source_poll_gap_seconds,omitempty"`
	DestinationPollGapSec    int64     `json:"destination_poll_gap_seconds,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

// run reads bounded, explicitly supplied AP status snapshots, correlates only
// their station_events in memory, and writes pseudonymized JSON to stdout.
// It performs no network access, changes no AP files, and logs no client MAC.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("roam-correlator", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var apSpecs apFileFlag
	fs.Var(&apSpecs, "ap", "AP label=status.js snapshot path (repeat for each AP)")
	window := fs.Duration("window", defaultRoamWindow, "maximum observation timestamp difference, from 5s to 10m")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: roam-correlator -ap AP_LABEL=status.js [-ap AP_LABEL=status.js] [-window 90s]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if len(apSpecs) < 2 || len(apSpecs) > maxAPFiles {
		return fmt.Errorf("provide between 2 and %d AP status files", maxAPFiles)
	}
	if *window < 5*time.Second || *window > 10*time.Minute {
		return errors.New("correlation window must be between 5 seconds and 10 minutes")
	}
	inputs, err := parseAPInputs(apSpecs)
	if err != nil {
		return err
	}
	if err := validateDistinctSnapshotFiles(inputs); err != nil {
		return err
	}
	correlator := telemetry.NewCrossAPCorrelator(*window, maxInputEvents, maxInputEvents)
	seenAPMACs := make(map[string]string, len(inputs))
	count := 0
	observations := make([]observedEvent, 0, maxInputEvents)
	for _, input := range inputs {
		snapshot, err := readSnapshot(input.path)
		if err != nil {
			return fmt.Errorf("read AP status snapshot %q: %w", input.path, err)
		}
		if len(snapshot.StationEvents) == 0 {
			return fmt.Errorf("AP status snapshot for %q does not expose station_events", input.name)
		}
		var events []telemetry.StationEvent
		if string(snapshot.StationEvents) != "null" {
			if err := json.Unmarshal(snapshot.StationEvents, &events); err != nil {
				return fmt.Errorf("AP status snapshot for %q has invalid station_events", input.name)
			}
		}
		if len(events) > maxStatusEvents {
			return fmt.Errorf("AP status snapshot %q exceeds %d station events", input.path, maxStatusEvents)
		}
		apMAC := strings.ToLower(strings.TrimSpace(snapshot.APMAC))
		if !apMACPattern.MatchString(apMAC) {
			return fmt.Errorf("AP status snapshot for %q is missing a valid AP MAC identity", input.name)
		}
		if previous := seenAPMACs[apMAC]; previous != "" {
			return fmt.Errorf("AP labels %q and %q identify the same AP", previous, input.name)
		}
		seenAPMACs[apMAC] = input.name
		count += len(events)
		if count > maxInputEvents {
			return fmt.Errorf("input exceeds %d station events", maxInputEvents)
		}
		for _, event := range events {
			observations = append(observations, observedEvent{ap: input.name, event: event})
		}
	}
	// The tracker prunes against each observation time. Feeding one AP's full
	// timeline before another's could discard a valid earlier pair when the
	// per-AP polling histories have different latest timestamps.
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		if !left.event.ObservedAt.Equal(right.event.ObservedAt) {
			return left.event.ObservedAt.Before(right.event.ObservedAt)
		}
		if left.ap != right.ap {
			return left.ap < right.ap
		}
		return left.event.Type < right.event.Type
	})
	for _, observation := range observations {
		correlator.Observe(observation.ap, observation.event)
	}
	var salt [32]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return errors.New("could not initialize per-run client pseudonyms")
	}
	output := outputDocument{
		EvidenceScope: "paired AP-local station observations only; does not prove a roam, exact timing, interruption, or disconnect reason",
		Window:        window.String(),
		Pairs:         make([]redactedPair, 0),
	}
	for _, pair := range correlator.Events() {
		output.Pairs = append(output.Pairs, redactPair(salt[:], pair))
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func validateDistinctSnapshotFiles(inputs []apInput) error {
	paths := make(map[string]string, len(inputs))
	files := make([]os.FileInfo, 0, len(inputs))
	for _, input := range inputs {
		absolute, err := filepath.Abs(input.path)
		if err != nil {
			return errors.New("invalid AP status snapshot path")
		}
		if previous := paths[filepath.Clean(absolute)]; previous != "" {
			return fmt.Errorf("AP labels %q and %q refer to the same status path", previous, input.name)
		}
		paths[filepath.Clean(absolute)] = input.name
		info, err := os.Stat(input.path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("AP status snapshot for %q is unavailable or not a regular file", input.name)
		}
		for i, prior := range files {
			if os.SameFile(prior, info) {
				return fmt.Errorf("AP labels %q and %q refer to the same status file", inputs[i].name, input.name)
			}
		}
		files = append(files, info)
	}
	return nil
}

func parseAPInputs(specs []string) ([]apInput, error) {
	inputs := make([]apInput, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		name, path, ok := strings.Cut(spec, "=")
		name, path = strings.TrimSpace(name), strings.TrimSpace(path)
		if !ok || !apNamePattern.MatchString(name) || path == "" {
			return nil, errors.New("each -ap value must be AP_LABEL=STATUS_PATH")
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate AP label %q", name)
		}
		seen[name] = true
		inputs = append(inputs, apInput{name: name, path: path})
	}
	return inputs, nil
}

func readSnapshot(path string) (statusSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return statusSnapshot{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return statusSnapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxStatusBytes {
		return statusSnapshot{}, errors.New("snapshot must be a regular file no larger than 256 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStatusBytes+1))
	if err != nil {
		return statusSnapshot{}, err
	}
	if len(data) > maxStatusBytes {
		return statusSnapshot{}, errors.New("snapshot exceeds 256 KiB")
	}
	data = unwrapStatusScript(data)
	var snapshot statusSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return statusSnapshot{}, errors.New("invalid AP status JSON")
	}
	return snapshot, nil
}

func unwrapStatusScript(data []byte) []byte {
	trimmed := strings.TrimSpace(string(data))
	const prefix = "window.unifiBridgeStatus ="
	if strings.HasPrefix(trimmed, prefix) {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		trimmed = strings.TrimSuffix(trimmed, ";")
		trimmed = strings.TrimSpace(trimmed)
	}
	return []byte(trimmed)
}

func redactPair(salt []byte, pair telemetry.CrossAPObservation) redactedPair {
	mac := strings.ToLower(strings.TrimSpace(pair.ClientMAC))
	macHash := hmac.New(sha256.New, salt)
	_, _ = macHash.Write([]byte(mac))
	tag := "c_" + hex.EncodeToString(macHash.Sum(nil)[:8])
	return redactedPair{
		Type:                     pair.Type,
		ClientTag:                tag,
		SourceAP:                 pair.SourceAP,
		DestinationAP:            pair.DestinationAP,
		SourceRadio:              pair.SourceRadio,
		DestinationRadio:         pair.DestinationRadio,
		SourcePhysicalRadio:      pair.SourcePhysicalRadio,
		DestinationPhysicalRadio: pair.DestinationPhysicalRadio,
		SourceBand:               pair.SourceBand,
		DestinationBand:          pair.DestinationBand,
		SourceBSSID:              pair.SourceBSSID,
		DestinationBSSID:         pair.DestinationBSSID,
		DepartureObservedAt:      pair.DepartureObservedAt,
		DeparturePreviousAt:      pair.DeparturePreviousAt,
		ArrivalObservedAt:        pair.ArrivalObservedAt,
		ArrivalPreviousAt:        pair.ArrivalPreviousAt,
		SourcePollGapSeconds:     pair.SourcePollGapSeconds,
		DestinationPollGapSec:    pair.DestinationPollGapSec,
	}
}
