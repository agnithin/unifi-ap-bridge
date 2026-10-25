package broadcom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

		cuTotal, cuSelfRx, cuSelfTx := p.parseChanimStats(iface)

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
			CuTotal:        cuTotal,
			CuSelfRx:       cuSelfRx,
			CuSelfTx:       cuSelfTx,
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

type netDevCounters struct {
	rxBytes   int64
	rxPackets int64
	rxErrors  int64
	rxDropped int64
	txBytes   int64
	txPackets int64
	txErrors  int64
	txDropped int64
}

func parseNetDev(content string) map[string]netDevCounters {
	res := make(map[string]netDevCounters)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) >= 16 {
			rxB, _ := strconv.ParseInt(fields[0], 10, 64)
			rxP, _ := strconv.ParseInt(fields[1], 10, 64)
			rxE, _ := strconv.ParseInt(fields[2], 10, 64)
			rxD, _ := strconv.ParseInt(fields[3], 10, 64)
			txB, _ := strconv.ParseInt(fields[8], 10, 64)
			txP, _ := strconv.ParseInt(fields[9], 10, 64)
			txE, _ := strconv.ParseInt(fields[10], 10, 64)
			txD, _ := strconv.ParseInt(fields[11], 10, 64)
			res[iface] = netDevCounters{
				rxBytes:   rxB,
				rxPackets: rxP,
				rxErrors:  rxE,
				rxDropped: rxD,
				txBytes:   txB,
				txPackets: txP,
				txErrors:  txE,
				txDropped: txD,
			}
		}
	}
	return res
}

func normalizeGatewayMAC(mac string) string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	if len(mac) == 17 && strings.HasPrefix(mac, "6e:") {
		// Cloud Gateway Max hardware device MAC is 6c:63:f8:30:9a:05
		if strings.HasPrefix(mac, "6e:63:f8:30:9a:") {
			return "6c:63:f8:30:9a:05"
		}
		b0, err := strconv.ParseUint(mac[:2], 16, 8)
		if err == nil && (b0&0x02) != 0 {
			b0 &^= 0x02
			return fmt.Sprintf("%02x%s", b0, mac[2:])
		}
	}
	return mac
}

func parseArpGatewayMAC(gwIP string) string {
	if gwIP == "" {
		return ""
	}
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == gwIP {
			mac := strings.ToLower(strings.TrimSpace(fields[3]))
			if mac != "00:00:00:00:00:00" && len(mac) == 17 {
				return normalizeGatewayMAC(mac)
			}
		}
	}
	return ""
}

type lldpCliOutput struct {
	LLDP struct {
		Interface map[string]struct {
			Chassis map[string]struct {
				ID struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"id"`
			} `json:"chassis"`
			Port struct {
				ID struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"id"`
				Descr string `json:"descr"`
			} `json:"port"`
		} `json:"interface"`
	} `json:"lldp"`
}

func (p *AsusProvider) getLLDPNeighbor() (chassisMAC string, portMAC string, remotePort int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := p.runner(ctx, "lldpcli", "-f", "json", "show", "neighbors")
	if err != nil || len(out) == 0 {
		return "", "", 0, fmt.Errorf("lldpcli unavailable: %v", err)
	}

	var data lldpCliOutput
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return "", "", 0, fmt.Errorf("json unmarshal failed: %v", err)
	}

	ifaceData, ok := data.LLDP.Interface["eth0"]
	if !ok {
		for _, v := range data.LLDP.Interface {
			ifaceData = v
			ok = true
			break
		}
	}
	if !ok {
		return "", "", 0, fmt.Errorf("no lldp interface found")
	}

	for _, c := range ifaceData.Chassis {
		if c.ID.Value != "" {
			chassisMAC = strings.ToLower(strings.TrimSpace(c.ID.Value))
			break
		}
	}

	portMAC = strings.ToLower(strings.TrimSpace(ifaceData.Port.ID.Value))
	descr := strings.ToLower(strings.TrimSpace(ifaceData.Port.Descr))

	remotePort = 1
	if strings.HasPrefix(descr, "eth") {
		numStr := strings.TrimPrefix(descr, "eth")
		if n, e := strconv.Atoi(numStr); e == nil {
			remotePort = n + 1 // eth0 -> Port 1, eth2 -> Port 3
		}
	} else if len(portMAC) == 17 && len(chassisMAC) == 17 {
		cLast, _ := strconv.ParseInt(chassisMAC[15:17], 16, 64)
		pLast, _ := strconv.ParseInt(portMAC[15:17], 16, 64)
		if pLast >= cLast && (pLast-cLast) < 10 {
			remotePort = int(pLast - cLast + 1)
		}
	}

	if chassisMAC == "" {
		return "", "", 0, fmt.Errorf("empty chassis mac")
	}
	return chassisMAC, portMAC, remotePort, nil
}

func (p *AsusProvider) GetLLDPTable() ([]telemetry.LLDPEntry, error) {
	chassisMAC, portMAC, _, err := p.getLLDPNeighbor()
	if err == nil && chassisMAC != "" {
		if portMAC == "" {
			portMAC = chassisMAC
		}
		return []telemetry.LLDPEntry{
			{
				ChassisID:     chassisMAC,
				PortID:        portMAC,
				LocalPortName: "eth0",
				LocalPortIdx:  1,
				IsWired:       true,
			},
		}, nil
	}

	gwIP := p.nvramGet("lan_gateway")
	if gwIP == "" {
		gwIP = "10.100.1.1"
	}
	gwMAC := parseArpGatewayMAC(gwIP)
	if gwMAC != "" {
		return []telemetry.LLDPEntry{
			{
				ChassisID:     gwMAC,
				PortID:        gwMAC,
				LocalPortName: "eth0",
				LocalPortIdx:  1,
				IsWired:       true,
			},
		}, nil
	}

	return nil, nil
}

func readSysfsInt(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return v
}

func readSysfsString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseArpTable(data string) map[string]string {
	arpMap := make(map[string]string)
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			ip := fields[0]
			mac := strings.ToLower(fields[3])
			if len(mac) == 17 && !strings.HasPrefix(mac, "00:00:00") {
				arpMap[mac] = ip
			}
		}
	}
	return arpMap
}

func parseBrctlShowmacs(data string, bridgePortMap map[int]string, arpMap map[string]string) (map[int][]telemetry.PortMacEntry, map[int]*telemetry.PortLastConnection) {
	macsByPort := make(map[int][]telemetry.PortMacEntry)
	lastConnByPort := make(map[int]*telemetry.PortLastConnection)

	// Map ifname to physical port index
	// eth0 = Port 1 (WAN/Uplink)
	// eth1 = Port 2 (LAN 1)
	// eth2 = Port 3 (LAN 2)
	// eth3 = Port 4 (LAN 3)
	ifaceToPortIdx := map[string]int{
		"eth1": 2,
		"eth2": 3,
		"eth3": 4,
	}

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pNumStr := fields[0]
		mac := strings.ToLower(fields[1])
		isLocal := strings.ToLower(fields[2])
		ageStr := fields[3]

		if isLocal == "yes" || len(mac) != 17 {
			continue
		}

		pNum, err := strconv.Atoi(pNumStr)
		if err != nil {
			continue
		}

		ifname, ok := bridgePortMap[pNum]
		if !ok {
			continue
		}

		portIdx, ok := ifaceToPortIdx[ifname]
		if !ok {
			continue
		}

		ageSec := 0
		if ageF, err := strconv.ParseFloat(ageStr, 64); err == nil {
			ageSec = int(ageF)
		}

		macsByPort[portIdx] = append(macsByPort[portIdx], telemetry.PortMacEntry{
			MAC:  mac,
			Age:  ageSec,
			Vlan: 1,
		})

		ip := arpMap[mac]
		lastConnByPort[portIdx] = &telemetry.PortLastConnection{
			Connected:   true,
			MAC:         mac,
			IP:          ip,
			ConnectedAt: time.Now().Unix() - int64(ageSec),
			LastSeen:    time.Now().Unix(),
		}
	}
	return macsByPort, lastConnByPort
}

func getBridgePortMap() map[int]string {
	m := make(map[int]string)
	entries, err := os.ReadDir("/sys/class/net/br0/brif")
	if err != nil {
		return m
	}
	for _, entry := range entries {
		ifname := entry.Name()
		portNoPath := filepath.Join("/sys/class/net/br0/brif", ifname, "port_no")
		data, err := os.ReadFile(portNoPath)
		if err != nil {
			continue
		}
		str := strings.TrimSpace(string(data))
		str = strings.TrimPrefix(str, "0x")
		if val, err := strconv.ParseInt(str, 16, 64); err == nil {
			m[int(val)] = ifname
		}
	}
	return m
}

func (p *AsusProvider) GetPortTable() ([]telemetry.Port, []telemetry.EthernetPort, *telemetry.Uplink, error) {
	netDevRaw, _ := os.ReadFile("/proc/net/dev")
	counters := parseNetDev(string(netDevRaw))

	hostMAC := p.nvramGet("lan_hwaddr")
	if norm, err := inform.NormalizeMAC(hostMAC); err == nil {
		hostMAC = norm
	}

	brPortMap := getBridgePortMap()
	arpData, _ := os.ReadFile("/proc/net/arp")
	arpMap := parseArpTable(string(arpData))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	showmacsOut, _ := p.runner(ctx, "brctl", "showmacs", "br0")
	macsByPort, lastConnByPort := parseBrctlShowmacs(showmacsOut, brPortMap, arpMap)

	chassisMAC, _, remotePort, err := p.getLLDPNeighbor()
	gwMAC := chassisMAC
	if err != nil || gwMAC == "" {
		gwIP := p.nvramGet("lan_gateway")
		if gwIP == "" {
			gwIP = "10.100.1.1"
		}
		gwMAC = parseArpGatewayMAC(gwIP)
		remotePort = 1
	}

	// Physical port mapping for Asuswrt (XT8 / BCM4908 / BCM53134):
	// eth0 = 2.5G WAN/Uplink
	// eth1 = 1G LAN 1
	// eth2 = 1G LAN 2
	// eth3 = 1G LAN 3
	ifaces := []struct {
		idx      int
		dev      string
		name     string
		isUplink bool
		defMedia string
	}{
		{idx: 1, dev: "eth0", name: "Port 1", isUplink: true, defMedia: "2.5GE"},
		{idx: 2, dev: "eth1", name: "Port 2", isUplink: false, defMedia: "GE"},
		{idx: 3, dev: "eth2", name: "Port 3", isUplink: false, defMedia: "GE"},
		{idx: 4, dev: "eth3", name: "Port 4", isUplink: false, defMedia: "GE"},
	}

	ports := make([]telemetry.Port, 0, len(ifaces))
	for _, spec := range ifaces {
		carrier := readSysfsInt(fmt.Sprintf("/sys/class/net/%s/carrier", spec.dev))
		speed := readSysfsInt(fmt.Sprintf("/sys/class/net/%s/speed", spec.dev))
		duplexStr := readSysfsString(fmt.Sprintf("/sys/class/net/%s/duplex", spec.dev))

		up := carrier == 1
		fullDuplex := duplexStr == "full"
		media := spec.defMedia
		if speed >= 2500 {
			media = "2.5GE"
		} else if speed >= 1000 {
			media = "GE"
		} else if speed >= 100 {
			media = "FE"
		} else if !up {
			speed = 0
		}

		stpState := "disabled"
		stpCost := 200000
		if up {
			stpState = "forwarding"
			if speed >= 2500 {
				stpCost = 8000
			} else if speed >= 1000 {
				stpCost = 20000
			}
		}

		c := counters[spec.dev]

		portMacs := macsByPort[spec.idx]
		lastConn := lastConnByPort[spec.idx]
		if spec.isUplink && gwMAC != "" && lastConn == nil {
			lastConn = &telemetry.PortLastConnection{
				Connected: up,
				MAC:       gwMAC,
				IP:        p.nvramGet("lan_gateway"),
				LastSeen:  time.Now().Unix(),
			}
		}

		ports = append(ports, telemetry.Port{
			PortIdx:        spec.idx,
			Name:           spec.name,
			Media:          media,
			Up:             up,
			Speed:          speed,
			FullDuplex:     fullDuplex,
			Enable:         true,
			Enabled:        true,
			IsUplink:       spec.isUplink,
			OpMode:         "switch",
			Forward:        "all",
			Autoneg:        true,
			PortPoe:        false,
			StpState:       stpState,
			StpPathcost:    stpCost,
			RxBytes:        c.rxBytes,
			TxBytes:        c.txBytes,
			RxPackets:      c.rxPackets,
			TxPackets:      c.txPackets,
			RxErrors:       c.rxErrors,
			TxErrors:       c.txErrors,
			RxDropped:      c.rxDropped,
			TxDropped:      c.txDropped,
			MacTable:       portMacs,
			LastConnection: lastConn,
		})
	}

	eth0Port := ports[0]
	ethTable := make([]telemetry.EthernetPort, 0, len(ports))
	for _, pt := range ports {
		devName := fmt.Sprintf("eth%d", pt.PortIdx-1)
		ethTable = append(ethTable, telemetry.EthernetPort{
			Name:       devName,
			MAC:        hostMAC,
			PortIdx:    pt.PortIdx,
			Speed:      pt.Speed,
			FullDuplex: pt.FullDuplex,
			NumPort:    len(ports),
		})
	}

	uplink := &telemetry.Uplink{
		Name:             "eth0",
		MAC:              hostMAC,
		IP:               p.nvramGet("lan_ipaddr"),
		Type:             "wire",
		Speed:            eth0Port.Speed,
		FullDuplex:       eth0Port.FullDuplex,
		Up:               eth0Port.Up,
		NumPort:          1,
		PortIdx:          1,
		Media:            "2.5GE",
		MaxSpeed:         2500,
		UplinkSource:     "lldp_uplink",
		UplinkMAC:        gwMAC,
		UplinkRemotePort: remotePort,
		RxBytes:          eth0Port.RxBytes,
		TxBytes:          eth0Port.TxBytes,
		RxPackets:        eth0Port.RxPackets,
		TxPackets:        eth0Port.TxPackets,
		RxErrors:         eth0Port.RxErrors,
		TxErrors:         eth0Port.TxErrors,
		RxDropped:        eth0Port.RxDropped,
	}

	return ports, ethTable, uplink, nil
}

func (p *AsusProvider) GetSystemStats() (*telemetry.SystemStats, *telemetry.SysStats, error) {
	var uptime int64
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if sec, err := strconv.ParseFloat(fields[0], 64); err == nil {
				uptime = int64(sec)
			}
		}
	}

	// Memory from /proc/meminfo
	var memTotal, memFree, buffers, cached, used int64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(strings.TrimSuffix(parts[1], "kB"))
			fields := strings.Fields(valStr)
			if len(fields) > 0 {
				val, _ := strconv.ParseInt(fields[0], 10, 64)
				switch key {
				case "MemTotal":
					memTotal = val
				case "MemFree":
					memFree = val
				case "Buffers":
					buffers = val
				case "Cached":
					cached = val
				}
			}
		}
	}

	memPct := 0.0
	if memTotal > 0 {
		used = memTotal - (memFree + buffers + cached)
		if used < 0 {
			used = 0
		}
		memPct = math.Round((float64(used)/float64(memTotal)*100.0)*10) / 10
	}

	// CPU & load averages from /proc/loadavg
	cpuPct := 0.0
	var load1Str, load5Str, load15Str string
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			load1Str = fields[0]
			load5Str = fields[1]
			load15Str = fields[2]
			if load1, err := strconv.ParseFloat(load1Str, 64); err == nil {
				numCores := runtime.NumCPU()
				if numCores <= 0 {
					numCores = 4
				}
				pct := (load1 / float64(numCores)) * 100.0
				if pct > 100.0 {
					pct = 100.0
				}
				cpuPct = math.Round(pct*10) / 10
			}
		}
	}

	sysStats := &telemetry.SysStats{
		LoadAvg1:  load1Str,
		LoadAvg5:  load5Str,
		LoadAvg15: load15Str,
		MemUsed:   used * 1024,
		MemTotal:  memTotal * 1024,
		MemBuffer: buffers * 1024,
	}

	return &telemetry.SystemStats{
		CPU:    cpuPct,
		Mem:    memPct,
		Uptime: uptime,
	}, sysStats, nil
}
