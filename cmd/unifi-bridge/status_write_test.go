package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteStatusFileReportsEachFailureStageWithoutReplacingPublishedStatus(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "status.js")
	if err := os.WriteFile(final, []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		fail string
		want string
	}{
		{"mkdir", "mkdir", "mkdir status directory"},
		{"write", "write", "write temporary status file"},
		{"rename", "rename", "rename status file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := osStatusFileOps
			sentinel := errors.New("permission denied")
			switch tc.fail {
			case "mkdir":
				ops.mkdirAll = func(string, os.FileMode) error { return sentinel }
			case "write":
				ops.writeFile = func(string, []byte, os.FileMode) error { return sentinel }
			case "rename":
				ops.rename = func(string, string) error { return sentinel }
			}
			err := writeStatusFile(dir, []byte("new"), ops)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want stage %q wrapping injected error", err, tc.want)
			}
			got, readErr := os.ReadFile(final)
			if readErr != nil || string(got) != "previous" {
				t.Fatalf("published status changed on failure: %q, %v", got, readErr)
			}
			if _, statErr := os.Stat(filepath.Join(dir, "status.js.tmp")); !os.IsNotExist(statErr) {
				t.Fatalf("temporary status file remains after failure: %v", statErr)
			}
		})
	}
}

func TestWriteStatusFilePublishesAtomically(t *testing.T) {
	dir := t.TempDir()
	if err := writeStatusFile(dir, []byte("window.status = {};\n"), osStatusFileOps); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "status.js"))
	if err != nil || string(got) != "window.status = {};\n" {
		t.Fatalf("published status = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "status.js.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains after success: %v", err)
	}
}

func TestStatusDiagnosticReporterBoundsAndRateLimitsWithoutLogRecursion(t *testing.T) {
	reporter := statusDiagnosticReporter{interval: time.Minute}
	var messages []string
	emit := func(format string, args ...any) { messages = append(messages, fmt.Sprintf(format, args...)) }
	start := time.Unix(100, 0)
	for i := 0; i < 100; i++ {
		reporter.report(start.Add(time.Duration(i)*time.Second), "rename status file", errors.New(strings.Repeat("x", 500)), emit)
	}
	if len(messages) != 2 {
		t.Fatalf("emitted %d diagnostics during 100 seconds, want at most one per minute", len(messages))
	}
	if len(messages[0]) > 260 || !strings.Contains(messages[0], "rename status file") || !strings.Contains(messages[0], "suppressed 0") {
		t.Fatalf("first diagnostic is not useful and bounded: %d bytes: %q", len(messages[0]), messages[0])
	}
	if !strings.Contains(messages[1], "suppressed 59") {
		t.Fatalf("rate-limited failures were not summarized: %q", messages[1])
	}
	reporter.report(start.Add(2*time.Minute), "mkdir status directory", errors.New("permission denied"), emit)
	if len(messages) != 3 || !strings.Contains(messages[2], "permission denied") || !strings.Contains(messages[2], "suppressed 39") {
		t.Fatalf("suppressed failures were not summarized: %#v", messages)
	}
}
