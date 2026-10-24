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
