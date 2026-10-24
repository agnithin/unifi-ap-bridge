package broadcom

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

var (
	rssiRegex  = regexp.MustCompile(`(?i)(?:rssi|signal):\s*(-?\d+)`)
	noiseRegex = regexp.MustCompile(`(?i)noise:\s*(-?\d+)`)
	rateRegex  = regexp.MustCompile(`(?i)(?:rate|rate_mbps):\s*(\d+)`)
)

type ParsedStaInfo struct {
	Signal     int
	Noise      int
	RSSI       int
	RadioProto string
	Is11ax     bool
	Is11ac     bool
	Is11n      bool
	TxRate     int
	RxRate     int
	Uptime     int
}

// ParseStaInfo parses the text output of `wl sta_info <mac>`.
func ParseStaInfo(output string, fallbackBand string) ParsedStaInfo {
	info := ParsedStaInfo{
		Noise: -90, // Standard default noise floor
	}

	lower := strings.ToLower(output)

	// Wi-Fi Generation Detection:
	// Prioritize highest generation first: 11ax -> 11ac -> 11n -> fallback
	switch {
	case strings.Contains(lower, "11ax") || strings.Contains(lower, "he_cap") || strings.Contains(lower, "he caps") || strings.Contains(lower, "he mcs") || strings.Contains(lower, "he set") || strings.Contains(lower, " he "):
		info.RadioProto = "ax"
		info.Is11ax = true
	case strings.Contains(lower, "11ac") || strings.Contains(lower, "vht_cap") || strings.Contains(lower, "vht caps") || strings.Contains(lower, "vht set") || strings.Contains(lower, "vht80") || strings.Contains(lower, "vht160") || strings.Contains(lower, " vht "):
		info.RadioProto = "ac"
		info.Is11ac = true
	case strings.Contains(lower, "11n") || strings.Contains(lower, "n_cap") || strings.Contains(lower, "ht caps") || strings.Contains(lower, "ht20") || strings.Contains(lower, "ht40") || strings.Contains(lower, "mcs set") || strings.Contains(lower, "mcs index") || strings.Contains(lower, " ht "):
		if fallbackBand == "na" {
			info.RadioProto = "na"
		} else {
			info.RadioProto = "ng"
		}
		info.Is11n = true
	default:
		if fallbackBand == "na" {
			info.RadioProto = "ac"
			info.Is11ac = true
		} else {
			info.RadioProto = "ng"
			info.Is11n = true
		}
	}

	if match := rssiRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.Signal = val
		}
	}

	if match := noiseRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.Noise = val
		}
	}

	// Calculate SNR (RSSI in UniFi is positive SNR = Signal - Noise)
	if info.Signal != 0 {
		snr := info.Signal - info.Noise
		if snr < 0 {
			snr = 0
		}
		info.RSSI = snr
	}

	if match := rateRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.TxRate = val * 1000 // Convert Mbps to kbps
		}
	}

	return info
}

// EnrichStation populates telemetry fields from parsed station info.
func EnrichStation(sta *telemetry.Station, info ParsedStaInfo) {
	if info.Signal != 0 {
		sta.Signal = info.Signal
		sta.Noise = info.Noise
		sta.RSSI = info.RSSI
	}
	sta.RadioProto = info.RadioProto
	sta.Is11ax = info.Is11ax
	sta.Is11ac = info.Is11ac
	sta.Is11n = info.Is11n
	sta.Authorized = true
	sta.IsGuest = false
}
