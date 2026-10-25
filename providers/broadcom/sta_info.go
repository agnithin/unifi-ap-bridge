package broadcom

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

var (
	rssiRegex         = regexp.MustCompile(`(?i)(?:rssi|signal):\s*(-?\d+)`)
	noiseRegex        = regexp.MustCompile(`(?i)noise(?:\s*floor)?:\s*(-?\d+)`)
	txRateKbpsRegex   = regexp.MustCompile(`(?i)rate of last tx pkt:\s*(\d+)\s*kbps`)
	rxRateKbpsRegex   = regexp.MustCompile(`(?i)rate of last rx pkt:\s*(\d+)\s*kbps`)
	legacyRateRegex   = regexp.MustCompile(`(?i)legacy rate\s*(\d+)\s*mbps`)
	genericRateRegex  = regexp.MustCompile(`(?i)(?:rate|rate_mbps):\s*(\d+)`)
	txUcastBytesRegex = regexp.MustCompile(`(?i)tx ucast bytes:\s*(\d+)`)
	txTotalBytesRegex = regexp.MustCompile(`(?i)tx total bytes:\s*(\d+)`)
	rxUcastBytesRegex = regexp.MustCompile(`(?i)rx ucast bytes:\s*(\d+)`)
	rxDataBytesRegex  = regexp.MustCompile(`(?i)rx (?:data|total) bytes:\s*(\d+)`)
	txUcastPktsRegex  = regexp.MustCompile(`(?i)tx ucast pkts:\s*(\d+)`)
	txTotalPktsRegex  = regexp.MustCompile(`(?i)tx total pkts:\s*(\d+)`)
	rxUcastPktsRegex  = regexp.MustCompile(`(?i)rx ucast pkts:\s*(\d+)`)
	rxDataPktsRegex   = regexp.MustCompile(`(?i)rx (?:data|total) pkts:\s*(\d+)`)
	txRetriesRegex    = regexp.MustCompile(`(?i)tx (?:pkts retries|data pkts retried):\s*(\d+)`)
	uptimeRegex       = regexp.MustCompile(`(?i)in network\s*(\d+)\s*seconds`)
	idleRegex         = regexp.MustCompile(`(?i)idle\s*(\d+)\s*seconds`)
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
	Uptime     int64
	IdleTime   int64
	TxBytes    int64
	RxBytes    int64
	TxPackets  int64
	RxPackets  int64
	TxRetries  int64
}

// ParseStaInfo parses the text output of `wl sta_info <mac>`.
func ParseStaInfo(output string, fallbackBand string) ParsedStaInfo {
	info := ParsedStaInfo{}

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
	if info.Signal != 0 && info.Noise != 0 {
		snr := info.Signal - info.Noise
		if snr < 0 {
			snr = 0
		}
		info.RSSI = snr
	}

	// Rates
	if match := txRateKbpsRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.TxRate = val
		}
	} else if match := genericRateRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.TxRate = val * 1000 // Convert Mbps to kbps
		}
	}

	if match := rxRateKbpsRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.RxRate = val
		}
	} else if match := legacyRateRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			info.RxRate = val * 1000 // Convert Mbps to kbps
		}
	}

	// Traffic & Packet Counters
	if match := txUcastBytesRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.TxBytes = val
		}
	} else if match := txTotalBytesRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.TxBytes = val
		}
	}

	if match := rxUcastBytesRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.RxBytes = val
		}
	} else if match := rxDataBytesRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.RxBytes = val
		}
	}

	if match := txUcastPktsRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.TxPackets = val
		}
	} else if match := txTotalPktsRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.TxPackets = val
		}
	}

	if match := rxUcastPktsRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.RxPackets = val
		}
	} else if match := rxDataPktsRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.RxPackets = val
		}
	}
	if match := txRetriesRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.TxRetries = val
		}
	}
	if match := uptimeRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.Uptime = val
		}
	}
	if match := idleRegex.FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			info.IdleTime = val
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
	sta.TxBytes = info.TxBytes
	sta.RxBytes = info.RxBytes
	sta.TxPackets = info.TxPackets
	sta.RxPackets = info.RxPackets
	sta.TxRetries = info.TxRetries
	sta.TxRate = info.TxRate
	sta.RxRate = info.RxRate
	sta.Uptime = info.Uptime
	sta.IdleTime = info.IdleTime
	sta.Satisfaction = CalculateSatisfaction(sta.Signal, sta.RSSI, sta.TxPackets, sta.TxRetries)
}

// CalculateSatisfaction computes a 0-100 UniFi WiFi Experience health score
// based on RSSI/signal strength and transmit packet retry rates.
func CalculateSatisfaction(signal, rssi int, txPackets, txRetries int64) int {
	if signal == 0 {
		return 0
	} // Missing measurements must not imply perfect experience.
	score := 100

	if signal != 0 {
		if signal < -80 {
			score -= 25
		} else if signal < -75 {
			score -= 15
		} else if signal < -67 {
			score -= 5
		}
	}

	if txPackets > 0 && txRetries > 0 {
		ratio := float64(txRetries) / float64(txPackets+txRetries)
		penalty := int(ratio * 100 * 2) // e.g. 5% retries = 10% penalty
		if penalty > 35 {
			penalty = 35
		}
		score -= penalty
	}

	if score < 20 {
		score = 20
	} else if score > 100 {
		score = 100
	}
	return score
}
