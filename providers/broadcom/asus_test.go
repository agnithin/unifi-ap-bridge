package broadcom

import (
	"context"
	"strings"
	"testing"
)

func TestAsusProvider(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) (string, error) {
		full := name + " " + strings.Join(args, " ")
		switch {
		case strings.Contains(full, "nvram get lan_hwaddr"):
			return "00:11:22:33:44:55", nil
		case strings.Contains(full, "nvram get lan_ipaddr"):
			return "10.100.1.10", nil
		case strings.Contains(full, "nvram get computer_name"):
			return "XT8-Test", nil
		case strings.Contains(full, "nvram get wl_ifnames"):
			return "eth4 eth6", nil
		case strings.Contains(full, "wl -i eth4 ssid"):
			return "Current SSID: \"blueberry-prime\"", nil
		case strings.Contains(full, "wl -i eth6 ssid"):
			return "Current SSID: \"blueberry-prime\"", nil
		case strings.Contains(full, "wl -i eth4 chanspec"):
			return "6", nil
		case strings.Contains(full, "wl -i eth6 chanspec"):
			return "161/80", nil
		case strings.Contains(full, "wl -i eth4 bssid"):
			return "00:11:22:33:44:56", nil
		case strings.Contains(full, "wl -i eth6 bssid"):
			return "00:11:22:33:44:57", nil
		case strings.Contains(full, "wl -i eth6 assoclist"):
			return "assoclist 16:ae:f6:a7:ee:2b", nil
		case strings.Contains(full, "wl -i eth6 sta_info"):
			return "flags: 0x1\nrates: 1201 Mbps (11ax 80MHz 2ss)\nrssi: -71\nnoise: -90", nil
		default:
			return "", nil
		}
	}

	provider := NewAsusProvider(mockRunner)

	host, err := provider.GetHostInfo()
	if err != nil {
		t.Fatalf("GetHostInfo failed: %v", err)
	}
	if host.Hostname != "XT8-Test" || host.IP != "10.100.1.10" {
		t.Errorf("host info mismatch: %+v", host)
	}

	radios, err := provider.GetRadios()
	if err != nil {
		t.Fatalf("GetRadios failed: %v", err)
	}
	if len(radios) != 2 {
		t.Errorf("expected 2 radios, got %d", len(radios))
	}

	wlanMap := map[string]string{
		"blueberry-prime": "6adb775a27327fe7fefdc22c",
	}

	vaps, err := provider.GetVAPs(wlanMap)
	if err != nil {
		t.Fatalf("GetVAPs failed: %v", err)
	}
	if len(vaps) != 2 {
		t.Fatalf("expected 2 vaps, got %d", len(vaps))
	}

	// Verify 5 GHz VAP station
	vap5G := vaps[1]
	if len(vap5G.StaTable) != 1 {
		t.Fatalf("expected 1 station on 5G VAP, got %d", len(vap5G.StaTable))
	}
	sta := vap5G.StaTable[0]
	if sta.MAC != "16:ae:f6:a7:ee:2b" {
		t.Errorf("station MAC mismatch: %s", sta.MAC)
	}
	if sta.RadioProto != "ax" {
		t.Errorf("expected Wi-Fi 6 ax station, got %s", sta.RadioProto)
	}
	if sta.Signal != -71 || sta.RSSI != 19 {
		t.Errorf("signal/rssi mismatch: signal=%d, rssi=%d", sta.Signal, sta.RSSI)
	}
}
