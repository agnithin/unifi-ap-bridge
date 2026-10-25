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
			return "flags: 0x1\nrates: 1201 Mbps (11ax 80MHz 2ss)\nrssi: -71\nnoise: -90\ntx ucast bytes: 1048576\nrx ucast bytes: 524288\ntx ucast pkts: 1000\nrx ucast pkts: 500", nil
		case strings.Contains(full, "wl -i eth4 chanim_stats"):
			return "version: 3\nchanspec tx inbss obss nocat nopkt doze txop goodtx badtx glitch badplcp knoise idle timestamp\n0x1006 12 2 35 11 6 0 28 1 4 1317 42 -91 36 -1193366231", nil
		case strings.Contains(full, "wl -i eth6 chanim_stats"):
			return "version: 3\nchanspec tx inbss obss nocat nopkt doze txop goodtx badtx glitch badplcp knoise idle timestamp\n0xe39b 5 3 2 1 1 0 80 1 1 10 5 -90 75 -1172933938", nil
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
	if radios[0].CuTotal != 64 || radios[0].CuSelfTx != 12 || radios[0].CuSelfRx != 2 {
		t.Errorf("radio 0 CuTotal mismatch: %+v", radios[0])
	}
	if radios[1].CuTotal != 25 || radios[1].CuSelfTx != 5 || radios[1].CuSelfRx != 3 {
		t.Errorf("radio 1 CuTotal mismatch: %+v", radios[1])
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
	if sta.TxBytes != 1048576 || sta.RxBytes != 524288 || sta.TxPackets != 1000 || sta.RxPackets != 500 {
		t.Errorf("sta traffic counters mismatch: %+v", sta)
	}
	if vap5G.TxBytes != 1048576 || vap5G.RxBytes != 524288 || vap5G.TxPackets != 1000 || vap5G.RxPackets != 500 {
		t.Errorf("vap traffic counters mismatch: %+v", vap5G)
	}

	// Verify Radio Stats (RF Channel Utilization)
	rStats, err := provider.GetRadioStats()
	if err != nil {
		t.Fatalf("GetRadioStats failed: %v", err)
	}
	if len(rStats) != 2 {
		t.Fatalf("expected 2 radio stats, got %d", len(rStats))
	}
	// eth4 (2.4 GHz, Ch 6): idle=36 -> cu_total=64, cu_self_tx=12, cu_self_rx=2
	if rStats[0].CuTotal != 64 || rStats[0].CuSelfTx != 12 || rStats[0].CuSelfRx != 2 {
		t.Errorf("2.4G radio stats mismatch: %+v", rStats[0])
	}
	// eth6 (5 GHz, Ch 161): idle=75 -> cu_total=25, cu_self_tx=5, cu_self_rx=3
	if rStats[1].CuTotal != 25 || rStats[1].CuSelfTx != 5 || rStats[1].CuSelfRx != 3 {
		t.Errorf("5G radio stats mismatch: %+v", rStats[1])
	}
}

func TestParseNetDev(t *testing.T) {
	raw := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 1000 10 1 2 0 0 0 0 2000 20 3 4 0 0 0 0
  eth3: 5000 50 0 0 0 0 0 0 6000 60 0 0 0 0 0 0`

	m := parseNetDev(raw)
	if len(m) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(m))
	}
	eth0 := m["eth0"]
	if eth0.rxBytes != 1000 || eth0.rxPackets != 10 || eth0.rxErrors != 1 || eth0.rxDropped != 2 {
		t.Errorf("eth0 rx counters mismatch: %+v", eth0)
	}
	if eth0.txBytes != 2000 || eth0.txPackets != 20 || eth0.txErrors != 3 || eth0.txDropped != 4 {
		t.Errorf("eth0 tx counters mismatch: %+v", eth0)
	}

	eth3 := m["eth3"]
	if eth3.rxBytes != 5000 || eth3.txBytes != 6000 {
		t.Errorf("eth3 counters mismatch: %+v", eth3)
	}
}

func TestParseArpTable(t *testing.T) {
	raw := `IP address       HW type     Flags       HW address            Mask     Device
10.100.1.8       0x1         0x2         10:2c:b1:73:3d:00     *        br0
10.100.1.99      0x1         0x2         90:3c:1d:58:12:30     *        br0
10.100.1.200     0x1         0x0         00:00:00:00:00:00     *        br0`

	arp := parseArpTable(raw)
	if len(arp) != 2 {
		t.Fatalf("expected 2 valid arp entries, got %d", len(arp))
	}
	if arp["10:2c:b1:73:3d:00"] != "10.100.1.8" {
		t.Errorf("arp ip mismatch: %s", arp["10:2c:b1:73:3d:00"])
	}
	if arp["90:3c:1d:58:12:30"] != "10.100.1.99" {
		t.Errorf("arp ip mismatch: %s", arp["90:3c:1d:58:12:30"])
	}
}

func TestParseBrctlShowmacs(t *testing.T) {
	raw := `port no	mac addr		is local?	ageing timer
  3	10:2c:b1:73:3d:00	no		   5.61
 10	00:11:32:cf:04:f0	no		   0.45
  1	fc:34:97:4f:ac:60	yes		   0.00
  2	a0:b1:c2:d3:e4:f5	no		  12.30`

	brPortMap := map[int]string{
		1:  "eth1", // Port 2 (LAN 1)
		2:  "eth2", // Port 3 (LAN 2)
		3:  "eth3", // Port 4 (LAN 3)
		10: "eth0", // Port 1 (WAN / Uplink)
	}

	arpMap := map[string]string{
		"10:2c:b1:73:3d:00": "10.100.1.8",
		"a0:b1:c2:d3:e4:f5": "10.100.1.50",
	}

	macsByPort, lastConnByPort := parseBrctlShowmacs(raw, brPortMap, arpMap)

	// Port 4 (eth3) should have 10:2c:b1:73:3d:00
	p4Macs := macsByPort[4]
	if len(p4Macs) != 1 {
		t.Fatalf("expected 1 mac on port 4, got %d", len(p4Macs))
	}
	if p4Macs[0].MAC != "10:2c:b1:73:3d:00" || p4Macs[0].Age != 5 {
		t.Errorf("unexpected port 4 mac: %+v", p4Macs[0])
	}
	if lastConnByPort[4] == nil || lastConnByPort[4].IP != "10.100.1.8" || lastConnByPort[4].MAC != "10:2c:b1:73:3d:00" {
		t.Errorf("unexpected port 4 last connection: %+v", lastConnByPort[4])
	}

	// Port 3 (eth2) should have a0:b1:c2:d3:e4:f5
	p3Macs := macsByPort[3]
	if len(p3Macs) != 1 || p3Macs[0].MAC != "a0:b1:c2:d3:e4:f5" {
		t.Errorf("unexpected port 3 mac: %+v", p3Macs)
	}

	// Port 1 (eth0) is uplink, should be ignored
	if len(macsByPort[1]) != 0 {
		t.Errorf("expected 0 macs on uplink port 1, got %d", len(macsByPort[1]))
	}
}
