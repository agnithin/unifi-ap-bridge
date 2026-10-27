package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

func TestRunCorrelatesAndRedactsClientIdentifiers(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	mainPath := writeSnapshotFixture(t, dir, "main.json", []telemetry.StationEvent{{Type: "observed_disassociation", MAC: "02:11:22:33:44:55", ObservedAt: base.Add(20 * time.Second), PreviousObserved: base.Add(10 * time.Second), PollGapSeconds: 10}})
	nodePath := writeSnapshotFixture(t, dir, "node.json", []telemetry.StationEvent{{Type: "observed_association", MAC: "02:11:22:33:44:55", ObservedAt: base.Add(15 * time.Second), PreviousObserved: base, PollGapSeconds: 15}})
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-window", "30s", "-ap", "main=" + mainPath, "-ap", "node=" + nodePath}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v stderr=%s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "02:11:22:33:44:55") || strings.Contains(stdout.String(), `"client_mac"`) {
		t.Fatalf("raw client MAC leaked: %s", stdout.String())
	}
	var output struct {
		EvidenceScope string `json:"evidence_scope"`
		Pairs         []struct {
			Type                  string    `json:"type"`
			ClientTag             string    `json:"client_tag"`
			SourceAP              string    `json:"source_ap"`
			DestinationAP         string    `json:"destination_ap"`
			DepartureObservedAt   time.Time `json:"departure_observed_at"`
			ArrivalObservedAt     time.Time `json:"arrival_observed_at"`
			SourcePollGapSeconds  int64     `json:"source_poll_gap_seconds"`
			DestinationPollGapSec int64     `json:"destination_poll_gap_seconds"`
		} `json:"pairs"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if len(output.Pairs) != 1 {
		t.Fatalf("expected one correlation, got %+v", output.Pairs)
	}
	pair := output.Pairs[0]
	if pair.Type != "correlated_ap_observations" || !strings.HasPrefix(pair.ClientTag, "c_") || pair.SourceAP != "main" || pair.DestinationAP != "node" || pair.DepartureObservedAt != base.Add(20*time.Second) || pair.ArrivalObservedAt != base.Add(15*time.Second) || pair.SourcePollGapSeconds != 10 || pair.DestinationPollGapSec != 15 {
		t.Fatalf("incorrect redacted correlation: %+v", pair)
	}
	if !strings.Contains(output.EvidenceScope, "does not prove a roam") {
		t.Fatalf("missing evidence limitation: %q", output.EvidenceScope)
	}
}

func TestRunGloballyOrdersObservationsAcrossAPs(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	mainPath := writeSnapshotFixture(t, dir, "main.json", []telemetry.StationEvent{
		{Type: "observed_disassociation", MAC: "02:11:22:33:44:55", ObservedAt: base.Add(10 * time.Second)},
		{Type: "observed_association", MAC: "02:11:22:33:44:55", ObservedAt: base.Add(100 * time.Second)},
	})
	nodePath := writeSnapshotFixture(t, dir, "node.json", []telemetry.StationEvent{
		{Type: "observed_association", MAC: "02:11:22:33:44:55", ObservedAt: base.Add(11 * time.Second)},
	})
	type pair struct {
		Type          string    `json:"type"`
		SourceAP      string    `json:"source_ap"`
		DestinationAP string    `json:"destination_ap"`
		DepartureAt   time.Time `json:"departure_observed_at"`
		ArrivalAt     time.Time `json:"arrival_observed_at"`
	}
	decode := func(args ...string) []pair {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err != nil {
			t.Fatalf("run failed: %v stderr=%s", err, stderr.String())
		}
		var output struct {
			Pairs []pair `json:"pairs"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			t.Fatalf("output is not JSON: %v", err)
		}
		return output.Pairs
	}
	mainFirst := decode("-window", "30s", "-ap", "main="+mainPath, "-ap", "node="+nodePath)
	nodeFirst := decode("-window", "30s", "-ap", "node="+nodePath, "-ap", "main="+mainPath)
	want := []pair{{Type: "correlated_ap_observations", SourceAP: "main", DestinationAP: "node", DepartureAt: base.Add(10 * time.Second), ArrivalAt: base.Add(11 * time.Second)}}
	if !reflect.DeepEqual(mainFirst, want) || !reflect.DeepEqual(nodeFirst, want) {
		t.Fatalf("AP argument order changed retained pairs: main-first=%+v node-first=%+v", mainFirst, nodeFirst)
	}
}

func TestRunRejectsInvalidOrOversizedInputsWithoutClientDataInErrors(t *testing.T) {
	dir := t.TempDir()
	good := writeSnapshotFixture(t, dir, "good.json", nil)
	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"-ap", "one=" + good, "-ap", "two=" + good, "-ap", "bad label=" + good},
		{"-ap", "one=" + good, "-ap", "one=" + good},
		{"-ap", "one=" + good, "-ap", "two=" + good},
		{"-window", "1h", "-ap", "one=" + good, "-ap", "two=" + good},
	} {
		stdout.Reset()
		stderr.Reset()
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatalf("invalid args accepted or emitted output: args=%q out=%q err=%v", args, stdout.String(), err)
		}
	}
	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, bytes.Repeat([]byte(" "), maxStatusBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSnapshot(large); err == nil {
		t.Fatal("oversized file accepted")
	}
	badJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badJSON, []byte(`{"station_events":[{"mac":"02:11:22:33:44:55"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSnapshot(badJSON); err == nil || strings.Contains(err.Error(), "02:11:22:33:44:55") {
		t.Fatalf("bad JSON error exposed client content: %v", err)
	}
	oldNode := filepath.Join(dir, "old-node.json")
	if err := os.WriteFile(oldNode, []byte(`{"mac":"02:00:00:00:00:02"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mainStatus := writeSnapshotFixture(t, dir, "main-status.json", nil)
	stdout.Reset()
	if err := run([]string{"-ap", "main=" + mainStatus, "-ap", "node=" + oldNode}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "does not expose station_events") || stdout.Len() != 0 {
		t.Fatalf("pre-timeline status was not rejected clearly: out=%q err=%v", stdout.String(), err)
	}
}

func TestRunLimitsEventsPerSnapshot(t *testing.T) {
	dir := t.TempDir()
	events := make([]telemetry.StationEvent, maxStatusEvents+1)
	path := writeSnapshotFixture(t, dir, "too-many.json", events)
	var stdout, stderr bytes.Buffer
	err := run([]string{"-ap", "main=" + path, "-ap", "node=" + path}, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("oversized event list was accepted: out=%q err=%v", stdout.String(), err)
	}
}

func TestRunRejectsSnapshotsWithSameAPIdentity(t *testing.T) {
	dir := t.TempDir()
	first := writeSnapshotFixture(t, dir, "first.json", nil)
	second := writeSnapshotFixture(t, dir, "second.json", nil)
	var stdout, stderr bytes.Buffer
	err := run([]string{"-ap", "main=" + first, "-ap", "node=" + second}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "same AP") || stdout.Len() != 0 {
		t.Fatalf("duplicate AP identity accepted: out=%q err=%v", stdout.String(), err)
	}
}

func writeSnapshotFixture(t *testing.T, dir, name string, events []telemetry.StationEvent) string {
	t.Helper()
	apMAC := "02:00:00:00:00:01"
	if strings.Contains(name, "node") {
		apMAC = "02:00:00:00:00:02"
	}
	data, err := json.Marshal(map[string]any{"mac": apMAC, "station_events": events, "unrelated_status": "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	script := append([]byte("window.unifiBridgeStatus = "), data...)
	script = append(script, []byte(";\n")...)
	if err := os.WriteFile(path, script, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
