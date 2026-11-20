package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

func TestDiagnosticInformDoesNotChangeWirePayload(t *testing.T) {
	original := telemetry.InformPayload{XAuthKey: "private-key-fixture", InformURL: "https://user:password@controller/inform?token=secret"}
	dump, err := marshalDiagnosticInform(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"private-key-fixture", "password", "token", "secret"} {
		if bytes.Contains(dump, []byte(value)) {
			t.Fatal("unredacted diagnostics")
		}
	}
	if original.XAuthKey != "private-key-fixture" || !bytes.Contains(dump, []byte("https://controller/inform")) {
		t.Fatal("missing endpoint or modified wire payload")
	}
}

func TestClearAdoptionRequiresLockAndClearsBothGenerations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-state.json")
	for _, name := range []string{path, path + ".bak"} {
		if err := os.WriteFile(name, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lock := filepath.Join(dir, "mutation")
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK", lock)
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK_HELD", "0")
	if err := clearAdoptionUnderLock(path); err == nil {
		t.Fatal("cleared without startup ownership")
	}
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte("123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK_HELD", "1")
	t.Setenv("UNIFI_BRIDGE_MUTATION_LOCK_OWNER", "123")
	if err := clearAdoptionUnderLock(path); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + ".bak"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("adoption file remains: %v", err)
		}
	}
}

func TestRoamRefreshWaitsForSpacingAndCoalescesWithFullSample(t *testing.T) {
	now := time.Unix(100, 0)
	spacing := 2 * time.Second
	if delay, schedule := roamRefreshDelay(now, now.Add(-time.Second), now.Add(10*time.Second), 500*time.Millisecond, spacing); !schedule || delay != time.Second {
		t.Fatalf("roam refresh scheduled with wrong delay: %s / %t", delay, schedule)
	}
	if _, schedule := roamRefreshDelay(now, now.Add(-time.Second), now.Add(2*time.Second), 500*time.Millisecond, spacing); schedule {
		t.Fatal("unnecessary wireless refresh immediately before full sample")
	}
}
