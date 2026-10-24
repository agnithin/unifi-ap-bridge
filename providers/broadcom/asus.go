package broadcom

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
	"github.com/agnithin/unifi-ap-bridge/providers"
)

type CmdRunner func(ctx context.Context, name string, args ...string) (string, error)

func defaultCmdRunner(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(bytes.TrimSpace(out)), err
}

type AsusProvider struct {
	runner CmdRunner
}

func NewAsusProvider(runner CmdRunner) *AsusProvider {
	if runner == nil {
		runner = defaultCmdRunner
	}
	return &AsusProvider{runner: runner}
}

func (p *AsusProvider) Name() string {
	return "broadcom_asuswrt"
}

func (p *AsusProvider) nvramGet(key string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := p.runner(ctx, "nvram", "get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (p *AsusProvider) wlCmd(iface string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fullArgs := append([]string{"-i", iface}, args...)
	return p.runner(ctx, "wl", fullArgs...)
}

func (p *AsusProvider) GetHostInfo() (*providers.HostInfo, error) {
	mac := p.nvramGet("lan_hwaddr")
	if mac == "" {
		mac = p.nvramGet("et0macaddr")
	}
	normMAC, err := inform.NormalizeMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("failed to get valid lan mac: %w", err)
	}

	ip := p.nvramGet("lan_ipaddr")
	hostname := p.nvramGet("lan_hostname")
	if hostname == "" {
		hostname = p.nvramGet("computer_name")
	}
	if hostname == "" {
		hostname = "Asus-XT8"
	}

	// Read uptime directly from /proc/uptime (sub-microsecond, 0 subprocesses)
	var uptime int64 = 3600
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if sec, err := strconv.ParseFloat(fields[0], 64); err == nil && sec > 0 {
				uptime = int64(sec)
			}
		}
	}

	return &providers.HostInfo{
		Hostname: hostname,
		MAC:      normMAC,
		IP:       ip,
		Uptime:   uptime,
	}, nil
}

func (p *AsusProvider) getRadioInterfaces() []string {
	raw := p.nvramGet("wl_ifnames")
	if raw == "" {
		return []string{"eth4", "eth6"}
	}
	all := strings.Fields(raw)
	// On tri-band XT8 (eth4=2.4G, eth5=5G-1, eth6=5G-2), client traffic is on eth4 and eth6
	if len(all) >= 3 && all[0] == "eth4" && all[2] == "eth6" {
		return []string{"eth4", "eth6"}
	}
	return all
}

func (p *AsusProvider) GetRadios() ([]telemetry.Radio, error) {
	ifaces := p.getRadioInterfaces()
	radios := make([]telemetry.Radio, 0, len(ifaces))

	for _, iface := range ifaces {
		chanspecRaw, _ := p.wlCmd(iface, "chanspec")
		channel, _ := ParseChanspec(chanspecRaw)
		band := BandForChannel(channel)
		radioName := "wifi-" + band

		ht := "20"
		nss := 2
		maxTx := 22
		minTx := 6
		hasVHT := false
		if band == "na" {
			ht = "80"
			nss = 4
			maxTx = 26
			minTx = 12
			hasVHT = true
		}

		radios = append(radios, telemetry.Radio{
			Name:           radioName,
			Radio:          band,
			Channel:        channel,
			HT:             ht,
			Nss:            nss,
			MaxTxPower:     maxTx,
			MinTxPower:     minTx,
			BuiltinAntenna: true,
			Is11ax:         true,
			HasHE:          true,
			HasVHT:         hasVHT,
			TxPowerMode:    "auto",
		})
	}
	return radios, nil
}

func (p *AsusProvider) GetVAPs(wlanMapping map[string]string) ([]telemetry.VAP, error) {
	candidates := []string{
		"eth4", "eth5", "eth6",
		"wl0.1", "wl0.2", "wl0.3",
		"wl1.1", "wl1.2", "wl1.3",
		"wl2.1", "wl2.2", "wl2.3",
	}

	vaps := make([]telemetry.VAP, 0, len(candidates))
	vapIndex := 0

	for _, iface := range candidates {
		ssidRaw, err := p.wlCmd(iface, "ssid")
		if err != nil {
			continue
		}
		var ssid string
		parts := strings.Split(ssidRaw, "\"")
		if len(parts) >= 2 {
			ssid = parts[1]
		}
		if ssid == "" {
			continue
		}

		bssidRaw, _ := p.wlCmd(iface, "bssid")
		bssid, err := inform.NormalizeMAC(bssidRaw)
		if err != nil {
			bssid = bssidRaw
		}

		chanspecRaw, _ := p.wlCmd(iface, "chanspec")
		channel, _ := ParseChanspec(chanspecRaw)
		if channel == 0 {
			if strings.HasPrefix(iface, "wl0") || iface == "eth4" {
				channel = 6
			} else if strings.HasPrefix(iface, "wl1") || iface == "eth5" {
				channel = 36
			} else {
				channel = 161
			}
		}

		band := BandForChannel(channel)
		radioName := "wifi-" + band

		vapName := fmt.Sprintf("ath%d", vapIndex)

		// Get associated clients for this VAP
		stations := p.getStationsForIface(iface, vapName, bssid, ssid, channel, band)

		// Skip unused intermediate backhaul interfaces if they have 0 clients
		if iface == "eth5" && len(stations) == 0 {
			continue
		}

		vapIndex++

		var vapTxBytes, vapRxBytes, vapTxPackets, vapRxPackets int64
		for _, s := range stations {
			vapTxBytes += s.TxBytes
			vapRxBytes += s.RxBytes
			vapTxPackets += s.TxPackets
			vapRxPackets += s.RxPackets
		}

		vap := telemetry.VAP{
			Name:      vapName,
			Radio:     band,
			RadioName: radioName,
			MAC:       bssid,
			BSSID:     bssid,
			ESSID:     ssid,
			Channel:   channel,
			State:     "RUN",
			Up:        true,
			IsGuest:   false,
			NumSta:    len(stations),
			TxBytes:   vapTxBytes,
			RxBytes:   vapRxBytes,
			TxPackets: vapTxPackets,
			RxPackets: vapRxPackets,
			StaTable:  stations,
		}

		if wlanID, ok := wlanMapping[ssid]; ok && len(wlanID) == 24 {
			vap.WlanID = wlanID
		}

		vaps = append(vaps, vap)
	}

	return vaps, nil
}

func (p *AsusProvider) getStationsForIface(iface, vapName, bssid, ssid string, channel int, band string) []telemetry.Station {
	stations := []telemetry.Station{}
	assocRaw, err := p.wlCmd(iface, "assoclist")
	if err != nil || strings.TrimSpace(assocRaw) == "" {
		return stations
	}

	lines := strings.Split(assocRaw, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		rawMAC := fields[0]
		if len(fields) >= 2 && fields[0] == "assoclist" {
			rawMAC = fields[1]
		}

		normMAC, err := inform.NormalizeMAC(rawMAC)
		if err != nil {
			continue
		}

		sta := telemetry.Station{
			MAC:        normMAC,
			Radio:      band,
			VAPName:    vapName,
			BSSID:      bssid,
			ESSID:      ssid,
			Channel:    channel,
			Authorized: true,
			IsGuest:    false,
		}

		// Enrich with sta_info (RSSI, generation, rates)
		infoRaw, err := p.wlCmd(iface, "sta_info", rawMAC)
		if err == nil && len(infoRaw) > 0 {
			parsed := ParseStaInfo(infoRaw, band)
			EnrichStation(&sta, parsed)
		} else {
			// Fallback: rssi only
			rssiRaw, _ := p.wlCmd(iface, "rssi", rawMAC)
			if rssiVal, err := strconv.Atoi(strings.TrimSpace(rssiRaw)); err == nil {
				sta.Signal = rssiVal
				sta.Noise = -90
				sta.RSSI = rssiVal - (-90)
			}
			if band == "na" {
				sta.RadioProto = "ac"
				sta.Is11ac = true
			} else {
				sta.RadioProto = "ng"
				sta.Is11n = true
			}
		}

		stations = append(stations, sta)
	}

	return stations
}

func (p *AsusProvider) parseChanimStats(iface string) (cuTotal int, cuSelfRx int, cuSelfTx int) {
	raw, err := p.wlCmd(iface, "chanim_stats")
	if err != nil || len(raw) == 0 {
		return 0, 0, 0
	}
	lines := strings.Split(raw, "\n")
	var headerIdx map[string]int
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "chanspec" {
			headerIdx = make(map[string]int)
			for i, h := range fields {
				headerIdx[h] = i
			}
			continue
		}
		if headerIdx != nil && strings.HasPrefix(fields[0], "0x") {
			if txIdx, ok := headerIdx["tx"]; ok && txIdx < len(fields) {
				if v, err := strconv.Atoi(fields[txIdx]); err == nil {
					cuSelfTx = v
				}
			}
			if rxIdx, ok := headerIdx["inbss"]; ok && rxIdx < len(fields) {
				if v, err := strconv.Atoi(fields[rxIdx]); err == nil {
					cuSelfRx = v
				}
			}
			if idleIdx, ok := headerIdx["idle"]; ok && idleIdx < len(fields) {
				if v, err := strconv.Atoi(fields[idleIdx]); err == nil {
					cuTotal = 100 - v
					if cuTotal < 0 {
						cuTotal = 0
					} else if cuTotal > 100 {
						cuTotal = 100
					}
				}
			}
			break
		}
	}
	return cuTotal, cuSelfRx, cuSelfTx
}

func (p *AsusProvider) GetRadioStats() ([]telemetry.RadioStat, error) {
	ifaces := p.getRadioInterfaces()
	stats := make([]telemetry.RadioStat, 0, len(ifaces))

	for _, iface := range ifaces {
		chanspecRaw, _ := p.wlCmd(iface, "chanspec")
		channel, _ := ParseChanspec(chanspecRaw)
		band := BandForChannel(channel)
		radioName := "wifi-" + band

		cuTotal, cuSelfRx, cuSelfTx := p.parseChanimStats(iface)

		stats = append(stats, telemetry.RadioStat{
			Name:     radioName,
			Channel:  channel,
			NumSta:   0, // Populated dynamically by bridge daemon based on active VAPs
			CuTotal:  cuTotal,
			CuSelfRx: cuSelfRx,
			CuSelfTx: cuSelfTx,
		})
	}
	return stats, nil
}
