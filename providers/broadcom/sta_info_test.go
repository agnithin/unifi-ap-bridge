package broadcom

import (
	"testing"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

func TestParseStaInfo_Generations(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		fallbackBand string
		wantProto    string
		want11ax     bool
		want11ac     bool
		want11n      bool
	}{
		{
			name: "Sonoff ESP32 on 2.4GHz (802.11n)",
			output: `[VER 8] STA 78:1C:3C:86:6E:8B:
	 chanspec 6 (0x1006)
	 flags 0x1e038: N_CAP AMPDU AMSDU
	 HT caps 0x112c:  SGI20 STBC-Rx
	 mcs index 7 stf mode 2 auto`,
			fallbackBand: "ng",
			wantProto:    "ng",
			want11ax:     false,
			want11ac:     false,
			want11n:      true,
		},
		{
			name: "5GHz 802.11ax client",
			output: `[VER 8] STA E8:C9:13:7D:11:CC:
	 flags 0xd1e13a: WME PS N_CAP VHT_CAP HE_CAP AMPDU AMSDU GBL_RCLASS
	 HE caps 0x26b9: LDPC HE-HTC SU-BFR SU&MU-BFE
	 he mcs 5 Nss 2 Tx Exp 2 bw80`,
			fallbackBand: "na",
			wantProto:    "ax",
			want11ax:     true,
			want11ac:     false,
			want11n:      false,
		},
		{
			name: "5GHz 802.11ac client",
			output: `[VER 8] STA 50:14:79:D7:7C:77:
	 flags 0xd1e13a: WME PS N_CAP VHT_CAP AMPDU AMSDU GBL_RCLASS
	 VHT caps 0x17b: LDPC SGI80 STBC-Tx STBC-Rx
	 vht mcs 9 Nss 2`,
			fallbackBand: "na",
			wantProto:    "ac",
			want11ax:     false,
			want11ac:     true,
			want11n:      false,
		},
		{
			name: "Fallback 2.4GHz client without caps",
			output: `[VER 8] STA 11:22:33:44:55:66:
	 rateset [ 1 2 5.5 6 9 11 12 18 24 36 48 54 ]`,
			fallbackBand: "ng",
			wantProto:    "ng",
			want11ax:     false,
			want11ac:     false,
			want11n:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ParseStaInfo(tc.output, tc.fallbackBand)
			if parsed.RadioProto != tc.wantProto {
				t.Errorf("RadioProto got %q, want %q", parsed.RadioProto, tc.wantProto)
			}
			if parsed.Is11ax != tc.want11ax {
				t.Errorf("Is11ax got %v, want %v", parsed.Is11ax, tc.want11ax)
			}
			if parsed.Is11ac != tc.want11ac {
				t.Errorf("Is11ac got %v, want %v", parsed.Is11ac, tc.want11ac)
			}
			if parsed.Is11n != tc.want11n {
				t.Errorf("Is11n got %v, want %v", parsed.Is11n, tc.want11n)
			}

			sta := telemetry.Station{MAC: "dummy"}
			EnrichStation(&sta, parsed)
			if sta.RadioProto != tc.wantProto || sta.Is11ax != tc.want11ax || sta.Is11ac != tc.want11ac || sta.Is11n != tc.want11n {
				t.Errorf("EnrichStation mismatch: %+v", sta)
			}
		})
	}
}

func TestParseStaInfo_Counters(t *testing.T) {
	raw := `[VER 8] STA CA:9D:36:FB:0B:53:
	 chanspec 161/80 (0xe39b)
	 aid:11 
	 rateset [ 6 9 12 18 24 36 48 54 ]
	 idle 4 seconds
	 in network 5595 seconds
	 state: AUTHENTICATED ASSOCIATED AUTHORIZED
	 flags 0x51e03b: BRCM WME N_CAP VHT_CAP HE_CAP AMPDU AMSDU
	 HE caps 0xf629: LDPC HE-HTC SU&MU-BFE
	 tx total pkts: 3022160
	 tx total bytes: 698409158
	 tx ucast pkts: 99951
	 tx ucast bytes: 111671620
	 tx failures: 22
	 rx data pkts: 162193
	 rx data bytes: 100968148
	 rx ucast pkts: 161988
	 rx ucast bytes: 100933896
	 rate of last tx pkt: 816660 kbps - 408330 kbps
	 rate of last rx pkt: 720580 kbps
	 tx pkts retries: 16301
	 per antenna rssi of last rx data frame: -68 -71 -68 -67
	 per antenna noise floor: -90 -89 -88 -90
smoothed rssi: -67`

	parsed := ParseStaInfo(raw, "na")
	if parsed.TxBytes != 111671620 {
		t.Errorf("TxBytes got %d, want %d", parsed.TxBytes, 111671620)
	}
	if parsed.RxBytes != 100933896 {
		t.Errorf("RxBytes got %d, want %d", parsed.RxBytes, 100933896)
	}
	if parsed.TxPackets != 99951 {
		t.Errorf("TxPackets got %d, want %d", parsed.TxPackets, 99951)
	}
	if parsed.RxPackets != 161988 {
		t.Errorf("RxPackets got %d, want %d", parsed.RxPackets, 161988)
	}
	if parsed.TxRetries != 16301 {
		t.Errorf("TxRetries got %d, want %d", parsed.TxRetries, 16301)
	}
	if !parsed.TxPacketsPresent || parsed.TxPacketSource != "tx_ucast_pkts" || !parsed.TxRetriesPresent || parsed.RetriesSource != "tx_pkts_retries" || !parsed.UptimePresent {
		t.Errorf("counter presence/source metadata missing: %+v", parsed)
	}
	if parsed.TxRate != 816660 {
		t.Errorf("TxRate got %d, want %d", parsed.TxRate, 816660)
	}
	if parsed.RxRate != 720580 {
		t.Errorf("RxRate got %d, want %d", parsed.RxRate, 720580)
	}
	if parsed.Uptime != 5595 {
		t.Errorf("Uptime got %d, want %d", parsed.Uptime, 5595)
	}
	if parsed.IdleTime != 4 {
		t.Errorf("IdleTime got %d, want %d", parsed.IdleTime, 4)
	}

	sta := telemetry.Station{MAC: "ca:9d:36:fb:0b:53"}
	EnrichStation(&sta, parsed)

	if sta.TxBytes != 111671620 || sta.RxBytes != 100933896 || sta.TxPackets != 99951 || sta.RxPackets != 161988 {
		t.Errorf("EnrichStation traffic counter mismatch: %+v", sta)
	}
	if sta.TxRate != 816660 || sta.RxRate != 720580 {
		t.Errorf("EnrichStation rate mismatch: tx=%d, rx=%d", sta.TxRate, sta.RxRate)
	}
	if sta.Uptime != 5595 || sta.IdleTime != 4 {
		t.Errorf("EnrichStation time mismatch: uptime=%d, idle=%d", sta.Uptime, sta.IdleTime)
	}
	if sta.Satisfaction != 0 {
		t.Errorf("a single cumulative sample must leave interval satisfaction unknown, got %d", sta.Satisfaction)
	}
}

func TestParseStaInfo_RetryScoreSourcePair(t *testing.T) {
	raw := `in network 100 seconds
tx ucast pkts: 200
tx pkts retries: 8`
	parsed := ParseStaInfo(raw, "na")
	if !parsed.TxPacketsPresent || parsed.TxPacketSource != "tx_ucast_pkts" || parsed.TxPackets != 200 {
		t.Fatalf("unicast packet source not captured: %+v", parsed)
	}
	if !parsed.TxRetriesPresent || parsed.RetriesSource != "tx_pkts_retries" || parsed.TxRetries != 8 {
		t.Fatalf("data retry source not captured: %+v", parsed)
	}
}

func TestParseStaInfoPrefersCanonicalRetryLabelAndKeepsExactCounters(t *testing.T) {
	// Sanitized dual-label observation: canonical host-requested unicast retry
	// count follows the broader data retry count in the textual payload.
	raw := `tx data pkts retried: 2
 tx pkts retries: 71
 tx total pkts sent: 2795
 tx pkts retry exhausted: 1
 in network 600 seconds`
	parsed := ParseStaInfo(raw, "na")
	if parsed.TxRetries != 71 || !parsed.TxRetriesPresent || parsed.RetriesSource != "tx_pkts_retries" {
		t.Fatalf("canonical retry source did not win: %+v", parsed)
	}
	if parsed.TxSuccessPackets != 2795 || !parsed.TxSuccessPresent || parsed.TxSuccessSource != "tx_total_pkts_sent" {
		t.Fatalf("exact success counter missing: %+v", parsed)
	}
	if parsed.TxRetryExhausted != 1 || !parsed.TxExhaustedPresent || parsed.TxExhaustedSource != "tx_pkts_retry_exhausted" {
		t.Fatalf("exact exhausted counter missing: %+v", parsed)
	}
	fallback := ParseStaInfo("tx data pkts retried: 9", "na")
	if fallback.TxRetries != 9 || !fallback.TxRetriesPresent || fallback.RetriesSource != "tx_data_pkts_retried" {
		t.Fatalf("secondary retry source fallback failed: %+v", fallback)
	}
	sta := telemetry.Station{}
	EnrichStation(&sta, parsed)
	if sta.TxSuccessPackets != 2795 || sta.TxRetryExhausted != 1 || !sta.TxSuccessPacketsPresent || !sta.TxRetryExhaustedPresent {
		t.Fatalf("station lost private parsed counters: %+v", sta)
	}
}

func TestCalculateSatisfaction(t *testing.T) {
	// Excellent client (-55 dBm, 0 retries) -> 100
	if s := CalculateSatisfaction(-55, 35, 1000, 0); s != 100 {
		t.Errorf("got %d, want 100", s)
	}
	// Weak signal client (-82 dBm, 10% retries)
	if s := CalculateSatisfaction(-82, 10, 900, 100); s > 70 {
		t.Errorf("got %d, want <= 70", s)
	}
}
