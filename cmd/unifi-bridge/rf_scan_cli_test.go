package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPassiveScanCLIRequiresOptInAndValidChannelsBeforeRunner(t *testing.T) {
	dir := t.TempDir()
	called := false
	runner := func(context.Context, int, string, ...string) (string, bool, error) {
		called = true
		return "", false, nil
	}
	if _, err := runPassiveScanCLI("eth4", "1,6", false, dir, runner); err == nil {
		t.Fatal("expected explicit opt-in to be required")
	}
	for _, channels := range []string{"", "1,,6", "1,abc", "1,2,3,4,5,6,7,8,9,10,11,12,13,14,32,36,40"} {
		if _, err := runPassiveScanCLI("eth4", channels, true, dir, runner); err == nil {
			t.Fatalf("invalid channel list accepted: %q", channels)
		}
	}
	if called {
		t.Fatal("blocked or invalid request invoked the scan runner")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("blocked request created output: entries=%v err=%v", entries, err)
	}
}

func TestRunPassiveScanCLIWritesCappedPrivateJSONArtifact(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	runner := func(_ context.Context, outputLimit int, executable string, args ...string) (string, bool, error) {
		if executable != "wl" {
			t.Fatalf("unexpected executable: %q", executable)
		}
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			return "accepted", false, nil
		}
		if outputLimit != rfScanOutputLimit-len("accepted") {
			t.Fatalf("remaining output limit=%d", outputLimit)
		}
		return strings.Repeat("x", rfScanOutputLimit), true, nil
	}
	path, err := runPassiveScanCLI("eth4", "1,6,36", true, dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("artifact escaped output directory: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("artifact mode=%#o, want 0600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Channels         []int  `json:"channels"`
		ResultsFreshness string `json:"results_freshness"`
		OutputTruncated  bool   `json:"output_truncated"`
		ScanOutput       string `json:"scan_output"`
		ResultsOutput    string `json:"results_output"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON artifact: %v", err)
	}
	if len(result.Channels) != 3 || result.ResultsFreshness != "unknown" || !result.OutputTruncated || result.ScanOutput != "accepted" || len(result.ScanOutput)+len(result.ResultsOutput) != rfScanOutputLimit {
		t.Fatalf("incorrect bounded artifact content: %+v", result)
	}
	if len(calls) != 2 || calls[0][4] != "passive" || calls[0][len(calls[0])-1] != "1,6,36" {
		t.Fatalf("unexpected passive command calls: %#v", calls)
	}
}
