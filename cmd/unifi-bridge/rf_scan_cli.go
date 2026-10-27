package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agnithin/unifi-ap-bridge/providers/broadcom"
)

const (
	rfScanOutputLimit = 16 * 1024
	rfScanTimeout     = 10 * time.Second
)

type passiveRFScanArtifact struct {
	Interface        string    `json:"interface"`
	Channels         []int     `json:"channels"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
	ScanOutput       string    `json:"scan_output"`
	ResultsOutput    string    `json:"results_output"`
	OutputTruncated  bool      `json:"output_truncated"`
	ResultsFreshness string    `json:"results_freshness"`
}

// runPassiveScanCLI runs one explicit passive scan only after the caller has
// supplied the disruptive-operation opt-in. The JSON artifact can contain
// nearby network identifiers, so it is created privately and never logged.
func runPassiveScanCLI(interfaceName, channelList string, allow bool, outputDir string, runner broadcom.PassiveScanRunner) (string, error) {
	if !allow {
		return "", errors.New("passive RF scan requires --allow-disruptive-rf-scan")
	}
	channels, err := parseRFScanChannels(channelList)
	if err != nil {
		return "", err
	}
	if outputDir == "" {
		return "", errors.New("passive RF scan output directory is required")
	}
	info, err := os.Stat(outputDir)
	if err != nil {
		return "", fmt.Errorf("inspect passive RF scan output directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("passive RF scan output path is not a directory")
	}
	result, err := broadcom.RunPassiveScan(context.Background(), broadcom.PassiveScanRequest{
		Interface:       interfaceName,
		Channels:        channels,
		PassiveDwell:    100 * time.Millisecond,
		HomeDwell:       100 * time.Millisecond,
		Timeout:         rfScanTimeout,
		MaxOutputBytes:  rfScanOutputLimit,
		AllowDisruption: true,
	}, runner)
	if err != nil {
		return "", err
	}
	artifact := passiveRFScanArtifact{Interface: result.Interface, Channels: result.Channels, StartedAt: result.StartedAt, CompletedAt: result.CompletedAt, ScanOutput: result.ScanOutput, ResultsOutput: result.ResultsOutput, OutputTruncated: result.OutputTruncated, ResultsFreshness: result.ResultsFreshness}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode passive RF scan result: %w", err)
	}
	file, err := os.CreateTemp(outputDir, "unifi-bridge-rf-scan-*.json")
	if err != nil {
		return "", fmt.Errorf("create private passive RF scan result: %w", err)
	}
	path := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("protect passive RF scan result: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write passive RF scan result: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("sync passive RF scan result: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close passive RF scan result: %w", err)
	}
	keep = true
	return path, nil
}

func parseRFScanChannels(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	if len(parts) == 0 || len(parts) > 16 {
		return nil, errors.New("passive RF scan requires 1 to 16 comma-separated channels")
	}
	channels := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("passive RF scan channel list contains an empty channel")
		}
		channel, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid passive RF scan channel %q", part)
		}
		channels = append(channels, channel)
	}
	return channels, nil
}
