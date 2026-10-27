package broadcom

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const validCCAOutput = "Using channel 6\nSummaries:\nchan dur ibss obss interf num seconds\n6 1004 8% Low 22% Medium 4% Low 1\n"

func TestParseCCAInterference(t *testing.T) {
	got, err := parseCCAInterference(validCCAOutput, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != 4 {
		t.Fatalf("interference = %v, want 4", got)
	}
	zero := strings.Replace(validCCAOutput, "4% Low", "0% Low", 1)
	got, err = parseCCAInterference(zero, 6)
	if err != nil || got == nil || *got != 0 {
		t.Fatalf("zero sample = %v, err %v; want measured zero", got, err)
	}
}

func TestOptionalCCAQueryTimeoutPreservesCompletedCoreRadioStats(t *testing.T) {
	var ccaCalls int
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "nvram get wl_ifnames":
			return "eth4", nil
		case "wl -i eth4 chanspec":
			return "6", nil
		case "wl -i eth4 chanim_stats":
			return "version: 3\nchanspec tx inbss obss nocat nopkt doze txop goodtx badtx glitch badplcp knoise idle timestamp\n0x1006 12 2 35 11 6 0 28 1 4 1317 42 -91 36 -1193366231", nil
		case "wl -i eth4 cca_get_stats -n -s 1":
			ccaCalls++
			<-ctx.Done()
			return "", ctx.Err()
		default:
			return "", fmt.Errorf("unexpected command %s", command)
		}
	}
	p := NewAsusProvider(runner)
	p.radioNames = map[string]string{"eth4": "wifi-ng"}
	stats, err := p.GetRadioStatsContext(context.Background())
	if err != nil || len(stats) != 1 || stats[0].CuTotal != 64 {
		t.Fatalf("core radio stats failed before optional CCA query: stats=%+v err=%v", stats, err)
	}
	start := time.Now()
	p.enrichCCAInterferenceContext(context.Background(), stats)
	if ccaCalls != 1 || len(stats) != 1 || stats[0].CuTotal != 64 || stats[0].CCAInterference != nil {
		t.Fatalf("unexpected optional-timeout result: calls=%d stats=%+v", ccaCalls, stats)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("optional CCA query exceeded its tight bound: %s", elapsed)
	}
}

func TestGetRadioStatsAddsMeasuredCCAInterference(t *testing.T) {
	runner := func(_ context.Context, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "nvram get wl_ifnames":
			return "eth4", nil
		case "wl -i eth4 chanspec":
			return "6", nil
		case "wl -i eth4 chanim_stats":
			return "version: 3\nchanspec tx inbss obss nocat nopkt doze txop goodtx badtx glitch badplcp knoise idle timestamp\n0x1006 12 2 35 11 6 0 28 1 4 1317 42 -91 36 -1193366231", nil
		case "wl -i eth4 cca_get_stats -n -s 1":
			return validCCAOutput, nil
		default:
			return "", fmt.Errorf("unexpected command %s", command)
		}
	}
	p := NewAsusProvider(runner)
	p.radioNames = map[string]string{"eth4": "wifi-ng"}
	stats, err := p.GetRadioStatsContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.enrichCCAInterferenceContext(context.Background(), stats)
	if len(stats) != 1 || stats[0].CCAInterference == nil || *stats[0].CCAInterference != 4 {
		t.Fatalf("CCA radio stats = %+v", stats)
	}
}

func TestGetRadioStatsSkipsCCAWhenSampleDeadlineIsNear(t *testing.T) {
	var ccaCalls int
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "nvram get wl_ifnames":
			return "eth4", nil
		case "wl -i eth4 chanspec":
			return "6", nil
		case "wl -i eth4 chanim_stats":
			return "version: 3\nchanspec tx inbss obss nocat nopkt doze txop goodtx badtx glitch badplcp knoise idle timestamp\n0x1006 12 2 35 11 6 0 28 1 4 1317 42 -91 36 -1193366231", nil
		case "wl -i eth4 cca_get_stats -n -s 1":
			ccaCalls++
			return validCCAOutput, nil
		default:
			return "", errors.New("unexpected command")
		}
	}
	p := NewAsusProvider(runner)
	p.radioNames = map[string]string{"eth4": "wifi-ng"}
	ctx, cancel := context.WithTimeout(context.Background(), ccaQueryTimeout+ccaDeadlineReserve)
	defer cancel()
	stats, err := p.GetRadioStatsContext(ctx)
	if err != nil {
		t.Fatalf("near-deadline core stats failed: %v", err)
	}
	p.enrichCCAInterferenceContext(ctx, stats)
	if ccaCalls != 0 || len(stats) != 1 || stats[0].CuTotal != 64 || stats[0].CCAInterference != nil {
		t.Fatalf("optional CCA should be skipped near sample deadline: calls=%d stats=%+v", ccaCalls, stats)
	}
}

func TestParseCCAInterferenceRejectsInvalidSamples(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ch   int
	}{
		{"empty", "", 6},
		{"wrong channel", validCCAOutput, 11},
		{"out of range", strings.Replace(validCCAOutput, "4% Low", "101% High", 1), 6},
		{"zero duration", strings.Replace(validCCAOutput, "6 1004", "6 0", 1), 6},
		{"zero samples", strings.TrimSuffix(validCCAOutput, "1\n") + "0\n", 6},
		{"multiple requested samples", strings.TrimSuffix(validCCAOutput, "1\n") + "2\n", 6},
		{"missing row", "Summaries:\nchan dur ibss obss interf num seconds\n", 6},
		{"multiple rows", validCCAOutput + "6 1004 8% Low 22% Medium 4% Low 1\n", 6},
		{"bad header", strings.Replace(validCCAOutput, "chan dur ibss obss interf num seconds", "chan duration ibss obss interf num seconds", 1), 6},
		{"too large", strings.Repeat("x", maxCCAStatsOutput+1), 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := parseCCAInterference(tc.raw, tc.ch); err == nil || got != nil {
				t.Fatalf("got %v, err %v; want omitted sample and error", got, err)
			}
		})
	}
}
