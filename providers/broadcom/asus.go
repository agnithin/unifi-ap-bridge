package broadcom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
	"github.com/agnithin/unifi-ap-bridge/providers"
)

type CmdRunner func(ctx context.Context, name string, args ...string) (string, error)

func defaultCmdRunner(ctx context.Context, name string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 300 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return string(bytes.TrimSpace(out)), ctx.Err()
	}
	if cmd.Process != nil {
		// The command leader can exit while background descendants survive,
		// whether or not they inherited its output pipes.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return string(bytes.TrimSpace(out)), err
}

const commandTimeout = 2 * time.Second
const stableCacheTTL = 5 * time.Minute

type staticCacheEntry struct {
	value   string
	expires time.Time
}

type sampleCommandCacheKey struct{}
type commandResult struct {
	output string
	err    error
}
type sampleCommandCache struct {
	mu      sync.Mutex
	results map[string]commandResult
}

type AsusProvider struct {
	runner      CmdRunner
	radioNames  map[string]string
	previousCPU cpuSample
	cacheMu     sync.Mutex
	staticCache map[string]staticCacheEntry
	now         func() time.Time
}

func NewAsusProvider(runner CmdRunner) *AsusProvider {
	if runner == nil {
		runner = defaultCmdRunner
	}
	return &AsusProvider{runner: runner, staticCache: make(map[string]staticCacheEntry), now: time.Now}
}

func (p *AsusProvider) Collect(ctx context.Context, mapping map[string]string) (*providers.Sample, error) {
	ctx = context.WithValue(ctx, sampleCommandCacheKey{}, &sampleCommandCache{results: make(map[string]commandResult)})
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	radios, err := p.GetRadiosContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect radios: %w", err)
	}
	vaps, err := p.GetVAPsContext(ctx, mapping)
	if err != nil {
		return nil, fmt.Errorf("collect vaps: %w", err)
	}
	sample := &providers.Sample{Radios: radios, VAPs: vaps, StartedAt: started, StageErrors: make(map[string]string)}
	setStageError := func(stage string, err error) error {
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		message := "unavailable"
		if errors.Is(err, context.DeadlineExceeded) {
			message = "deadline exceeded"
		}
		if errors.Is(err, context.Canceled) {
			message = "canceled"
		}
		sample.StageErrors[stage] = message
		return nil
	}
	if sample.RadioStats, err = p.GetRadioStatsContext(ctx); err != nil {
		if fatal := setStageError("radio_stats", err); fatal != nil {
			return nil, fatal
		}
	}
	if sample.Ports, sample.Ethernet, sample.Uplink, err = p.GetPortTableContext(ctx); err != nil {
		if fatal := setStageError("ports", err); fatal != nil {
			return nil, fatal
		}
		sample.Ports, sample.Ethernet, sample.Uplink = nil, nil, nil
	}
	if sample.LLDP, err = p.GetLLDPTableContext(ctx); err != nil {
		if fatal := setStageError("lldp", err); fatal != nil {
			return nil, fatal
		}
		sample.LLDP = nil
	}
	if sample.System, sample.LegacySystem, err = p.GetSystemStatsContext(ctx); err != nil {
		if fatal := setStageError("system", err); fatal != nil {
			return nil, fatal
		}
		sample.System, sample.LegacySystem = nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sample.CompletedAt = time.Now()
	return sample, nil
}

func (p *AsusProvider) Name() string {
	return "broadcom_asuswrt"
}

func (p *AsusProvider) nvramGet(key string) string {
	out, _ := p.nvramGetContext(context.Background(), key)
	return out
}

func (p *AsusProvider) runCommand(ctx context.Context, name string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if cache, ok := ctx.Value(sampleCommandCacheKey{}).(*sampleCommandCache); ok {
		key := name + "\x00" + strings.Join(args, "\x00")
		cache.mu.Lock()
		result, found := cache.results[key]
		cache.mu.Unlock()
		if found {
			return result.output, result.err
		}
		cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
		out, err := p.runner(cmdCtx, name, args...)
		cancel()
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			cache.mu.Lock()
			cache.results[key] = commandResult{output: out, err: err}
			cache.mu.Unlock()
		}
		return out, err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	return p.runner(cmdCtx, name, args...)
}

func (p *AsusProvider) wlCmd(iface string, args ...string) (string, error) {
	return p.wlCmdContext(context.Background(), iface, args...)
}

func (p *AsusProvider) wlCmdContext(ctx context.Context, iface string, args ...string) (string, error) {
	fullArgs := append([]string{"-i", iface}, args...)
	return p.runCommand(ctx, "wl", fullArgs...)
}

func (p *AsusProvider) nvramGetContext(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	stable := key == "wl_ifnames" || strings.HasPrefix(key, "wl") && strings.HasSuffix(key, "_vifnames")
	if stable {
		p.cacheMu.Lock()
		entry, ok := p.staticCache["nvram:"+key]
		if ok && p.now().Before(entry.expires) {
			p.cacheMu.Unlock()
			return entry.value, nil
		}
		p.cacheMu.Unlock()
	}
	out, err := p.runCommand(ctx, "nvram", "get", key)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if stable {
		p.cacheMu.Lock()
		p.staticCache["nvram:"+key] = staticCacheEntry{value: out, expires: p.now().Add(stableCacheTTL)}
		p.cacheMu.Unlock()
	}
	return out, nil
}

func (p *AsusProvider) GetHostInfo() (*providers.HostInfo, error) {
	return p.GetHostInfoContext(context.Background())
}

func (p *AsusProvider) GetHostInfoContext(ctx context.Context) (*providers.HostInfo, error) {
	mac, _ := p.nvramGetContext(ctx, "lan_hwaddr")
	if mac == "" {
		mac, _ = p.nvramGetContext(ctx, "et0macaddr")
	}
	normMAC, err := inform.NormalizeMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("failed to get valid lan mac: %w", err)
	}

	ip, _ := p.nvramGetContext(ctx, "lan_ipaddr")
	hostname, _ := p.nvramGetContext(ctx, "lan_hostname")
	if hostname == "" {
		hostname, _ = p.nvramGetContext(ctx, "computer_name")
	}
	if hostname == "" {
		hostname = "Asus-XT8"
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
	interfaces, _ := p.getRadioInterfacesContext(context.Background())
	return interfaces
}

func (p *AsusProvider) getRadioInterfacesContext(ctx context.Context) ([]string, error) {
	raw, err := p.nvramGetContext(ctx, "wl_ifnames")
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return []string{"eth4", "eth6"}, nil
	}
	return strings.Fields(raw), nil
}

func (p *AsusProvider) GetRadios() ([]telemetry.Radio, error) {
	return p.GetRadiosContext(context.Background())
}

func (p *AsusProvider) GetRadiosContext(ctx context.Context) ([]telemetry.Radio, error) {
	ifaces, err := p.getRadioInterfacesContext(ctx)
	if err != nil {
		return nil, err
	}
	radios := make([]telemetry.Radio, 0, len(ifaces))
	p.radioNames = make(map[string]string)
	bandCounts := make(map[string]int)

	for _, iface := range ifaces {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chanspecRaw, err := p.wlCmdContext(ctx, iface, "chanspec")
		if err != nil {
			return nil, fmt.Errorf("read %s chanspec: %w", iface, err)
		}
		channel, width := ParseChanspec(chanspecRaw)
		if channel <= 0 || width <= 0 {
			return nil, fmt.Errorf("read %s chanspec: invalid value %q", iface, strings.TrimSpace(chanspecRaw))
		}
		band := BandForChannel(channel)
		radioName := "wifi-" + band
		if bandCounts[band] > 0 {
			radioName += "-" + strconv.Itoa(bandCounts[band])
		}
		bandCounts[band]++
		p.radioNames[iface] = radioName

		ht := strconv.Itoa(width)
		nss := 2
		maxTx := 22
		minTx := 6
		hasVHT := false
		if band == "na" {
			nss = 4
			maxTx = 26
			minTx = 12
			hasVHT = true
		}

		if chainRaw, e := p.wlCmdContext(ctx, iface, "txchain"); e == nil && len(strings.Fields(chainRaw)) > 0 {
			if mask, e := strconv.ParseUint(strings.Fields(chainRaw + " ")[0], 0, 32); e == nil && mask != 0 {
				nss = bits.OnesCount32(uint32(mask))
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cuTotal, cuSelfRx, cuSelfTx, err := p.parseChanimStatsStrict(ctx, iface)
		if err != nil {
			return nil, fmt.Errorf("read %s channel utilization: %w", iface, err)
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
			CuTotal:        cuTotal,
			CuSelfRx:       cuSelfRx,
			CuSelfTx:       cuSelfTx,
		})
	}
	return radios, nil
}

func (p *AsusProvider) GetVAPs(wlanMapping map[string]string) ([]telemetry.VAP, error) {
	return p.GetVAPsContext(context.Background(), wlanMapping)
}

func (p *AsusProvider) GetVAPsContext(ctx context.Context, wlanMapping map[string]string) ([]telemetry.VAP, error) {
	if p.radioNames == nil {
		if _, err := p.GetRadiosContext(ctx); err != nil {
			return nil, err
		}
	}
	physical, err := p.getRadioInterfacesContext(ctx)
	if err != nil {
		return nil, err
	}
	candidates := append([]string(nil), physical...)
	parent := make(map[string]string)
	units := make(map[string]int)
	for unit, iface := range physical {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		parent[iface] = iface
		units[iface] = unit
		virtualRaw, err := p.nvramGetContext(ctx, fmt.Sprintf("wl%d_vifnames", unit))
		if err != nil {
			return nil, err
		}
		virtual := strings.Fields(virtualRaw)
		if len(virtual) == 0 {
			for sub := 1; sub <= 3; sub++ {
				virtual = append(virtual, fmt.Sprintf("wl%d.%d", unit, sub))
			}
		}
		for _, vif := range virtual {
			parent[vif] = iface
			units[vif] = unit
		}
		candidates = append(candidates, virtual...)
	}

	vaps := make([]telemetry.VAP, 0, len(candidates))

	for _, iface := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nvramPrefix := iface
		if !strings.HasPrefix(iface, "wl") {
			nvramPrefix = fmt.Sprintf("wl%d", units[iface])
		}
		bssEnabled, err := p.nvramGetContext(ctx, nvramPrefix+"_bss_enabled")
		if err != nil {
			return nil, err
		}
		if bssEnabled == "0" {
			continue
		}
		ssidRaw, err := p.wlCmdContext(ctx, iface, "ssid")
		if err != nil {
			return nil, fmt.Errorf("read %s SSID: %w", iface, err)
		}
		var ssid string
		parts := strings.Split(ssidRaw, "\"")
		if len(parts) >= 2 {
			ssid = parts[1]
		}
		if ssid == "" {
			continue
		}

		bssidRaw, _ := p.wlCmdContext(ctx, iface, "bssid")
		bssid, err := inform.NormalizeMAC(bssidRaw)
		if err != nil {
			bssid = bssidRaw
		}

		chanspecRaw, err := p.wlCmdContext(ctx, iface, "chanspec")
		if err != nil {
			return nil, fmt.Errorf("read %s chanspec: %w", iface, err)
		}
		channel, _ := ParseChanspec(chanspecRaw)
		if channel == 0 {
			return nil, fmt.Errorf("read %s chanspec: invalid value %q", iface, strings.TrimSpace(chanspecRaw))
		}

		band := BandForChannel(channel)
		radioName := p.radioNames[parent[iface]]

		sub := 0
		if dot := strings.IndexByte(iface, '.'); dot >= 0 {
			sub, _ = strconv.Atoi(iface[dot+1:])
		}
		vapName := fmt.Sprintf("ath%d", units[iface]*16+sub)

		// Get associated clients for this VAP
		stations, err := p.getStationsForIfaceContext(ctx, iface, vapName, bssid, ssid, channel, band)
		if err != nil {
			return nil, err
		}

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
	stations, _ := p.getStationsForIfaceContext(context.Background(), iface, vapName, bssid, ssid, channel, band)
	return stations
}

func (p *AsusProvider) getStationsForIfaceContext(ctx context.Context, iface, vapName, bssid, ssid string, channel int, band string) ([]telemetry.Station, error) {
	stations := []telemetry.Station{}
	assocRaw, err := p.wlCmdContext(ctx, iface, "assoclist")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(assocRaw) == "" {
		return stations, nil
	}

	lines := strings.Split(assocRaw, "\n")

	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		infoRaw, err := p.wlCmdContext(ctx, iface, "sta_info", rawMAC)
		if err != nil {
			return nil, fmt.Errorf("read %s station %s details: %w", iface, normMAC, err)
		}
		if len(infoRaw) == 0 {
			return nil, fmt.Errorf("read %s station %s details: empty response", iface, normMAC)
		}
		parsed := ParseStaInfo(infoRaw, band)
		EnrichStation(&sta, parsed)

		stations = append(stations, sta)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return stations, nil
}

func (p *AsusProvider) parseChanimStats(iface string) (cuTotal int, cuSelfRx int, cuSelfTx int) {
	return p.parseChanimStatsContext(context.Background(), iface)
}

func (p *AsusProvider) parseChanimStatsContext(ctx context.Context, iface string) (cuTotal int, cuSelfRx int, cuSelfTx int) {
	cuTotal, cuSelfRx, cuSelfTx, _ = p.parseChanimStatsStrict(ctx, iface)
	return
}

func (p *AsusProvider) parseChanimStatsStrict(ctx context.Context, iface string) (cuTotal int, cuSelfRx int, cuSelfTx int, resultErr error) {
	raw, err := p.wlCmdContext(ctx, iface, "chanim_stats")
	if err != nil {
		return 0, 0, 0, err
	}
	if len(strings.TrimSpace(raw)) == 0 {
		return 0, 0, 0, fmt.Errorf("empty response")
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
			txIdx, hasTx := headerIdx["tx"]
			rxIdx, hasRx := headerIdx["inbss"]
			idleIdx, hasIdle := headerIdx["idle"]
			if !hasTx || !hasRx || !hasIdle || txIdx >= len(fields) || rxIdx >= len(fields) || idleIdx >= len(fields) {
				return 0, 0, 0, fmt.Errorf("malformed utilization row")
			}
			cuSelfTx, err = strconv.Atoi(fields[txIdx])
			if err != nil {
				return 0, 0, 0, fmt.Errorf("invalid tx utilization: %w", err)
			}
			cuSelfRx, err = strconv.Atoi(fields[rxIdx])
			if err != nil {
				return 0, 0, 0, fmt.Errorf("invalid inbss utilization: %w", err)
			}
			idle, err := strconv.Atoi(fields[idleIdx])
			if err != nil || idle < 0 || idle > 100 {
				return 0, 0, 0, fmt.Errorf("invalid idle utilization %q", fields[idleIdx])
			}
			cuTotal = 100 - idle
			return cuTotal, cuSelfRx, cuSelfTx, nil
		}
	}
	return 0, 0, 0, fmt.Errorf("unrecognized response")
}

func (p *AsusProvider) GetRadioStats() ([]telemetry.RadioStat, error) {
	return p.GetRadioStatsContext(context.Background())
}

func (p *AsusProvider) GetRadioStatsContext(ctx context.Context) ([]telemetry.RadioStat, error) {
	if p.radioNames == nil {
		if _, err := p.GetRadiosContext(ctx); err != nil {
			return nil, err
		}
	}
	ifaces, err := p.getRadioInterfacesContext(ctx)
	if err != nil {
		return nil, err
	}
	stats := make([]telemetry.RadioStat, 0, len(ifaces))

	for _, iface := range ifaces {
		chanspecRaw, err := p.wlCmdContext(ctx, iface, "chanspec")
		if err != nil {
			return nil, fmt.Errorf("read %s chanspec: %w", iface, err)
		}
		channel, _ := ParseChanspec(chanspecRaw)
		if channel <= 0 {
			return nil, fmt.Errorf("read %s chanspec: invalid value %q", iface, strings.TrimSpace(chanspecRaw))
		}
		band := BandForChannel(channel)
		radioName := p.radioNames[iface]
		if radioName == "" {
			radioName = "wifi-" + band
		}

		cuTotal, cuSelfRx, cuSelfTx, err := p.parseChanimStatsStrict(ctx, iface)
		if err != nil {
			return nil, fmt.Errorf("read %s channel utilization: %w", iface, err)
		}

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
	return p.getLLDPNeighborContext(context.Background())
}

func (p *AsusProvider) getLLDPNeighborContext(ctx context.Context) (chassisMAC string, portMAC string, remotePort int, err error) {
	out, err := p.runCommand(ctx, "lldpcli", "-f", "json", "show", "neighbors")
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

	remotePort = 0
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
	return p.GetLLDPTableContext(context.Background())
}

func (p *AsusProvider) GetLLDPTableContext(ctx context.Context) ([]telemetry.LLDPEntry, error) {
	chassisMAC, portMAC, _, err := p.getLLDPNeighborContext(ctx)
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

	gwIP, _ := p.nvramGetContext(ctx, "lan_gateway")
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

	if err := ctx.Err(); err != nil {
		return nil, err
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
			MAC: mac,
			Age: ageSec,
		})

		ip := arpMap[mac]
		lastConnByPort[portIdx] = &telemetry.PortLastConnection{
			Connected: true,
			MAC:       mac,
			IP:        ip,
			// FDB age is last-seen age, not connection uptime.
			ConnectedAt: 0,
			LastSeen:    time.Now().Unix() - int64(ageSec),
		}
	}
	for port, macs := range macsByPort {
		if len(macs) != 1 {
			delete(lastConnByPort, port)
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
	return p.GetPortTableContext(context.Background())
}

func (p *AsusProvider) GetPortTableContext(ctx context.Context) ([]telemetry.Port, []telemetry.EthernetPort, *telemetry.Uplink, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	netDevRaw, _ := os.ReadFile("/proc/net/dev")
	counters := parseNetDev(string(netDevRaw))

	hostMAC, err := p.nvramGetContext(ctx, "lan_hwaddr")
	if err != nil {
		return nil, nil, nil, err
	}
	if norm, err := inform.NormalizeMAC(hostMAC); err == nil {
		hostMAC = norm
	}

	brPortMap := getBridgePortMap()
	arpData, _ := os.ReadFile("/proc/net/arp")
	arpMap := parseArpTable(string(arpData))

	showmacsOut, cmdErr := p.runCommand(ctx, "brctl", "showmacs", "br0")
	if cmdErr != nil && ctx.Err() != nil {
		return nil, nil, nil, ctx.Err()
	}
	macsByPort, lastConnByPort := parseBrctlShowmacs(showmacsOut, brPortMap, arpMap)

	chassisMAC, _, remotePort, err := p.getLLDPNeighborContext(ctx)
	gwMAC := chassisMAC
	if err != nil || gwMAC == "" {
		gwIP, err := p.nvramGetContext(ctx, "lan_gateway")
		if err != nil {
			return nil, nil, nil, err
		}
		if gwIP == "" {
			gwIP = "10.100.1.1"
		}
		gwMAC = parseArpGatewayMAC(gwIP)
		remotePort = 0
	}
	lanGateway, err := p.nvramGetContext(ctx, "lan_gateway")
	if err != nil {
		return nil, nil, nil, err
	}
	lanIP, err := p.nvramGetContext(ctx, "lan_ipaddr")
	if err != nil {
		return nil, nil, nil, err
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
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
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

		stpStates := map[string]string{"0": "disabled", "1": "listening", "2": "learning", "3": "forwarding", "4": "blocking"}
		stpState := stpStates[readSysfsString("/sys/class/net/br0/brif/"+spec.dev+"/state")]
		stpCost := readSysfsInt("/sys/class/net/br0/brif/" + spec.dev + "/path_cost")
		if stpCost < 0 {
			stpCost = 0
		}

		c := counters[spec.dev]

		portMacs := macsByPort[spec.idx]
		lastConn := lastConnByPort[spec.idx]
		if spec.isUplink && gwMAC != "" && lastConn == nil {
			lastConn = &telemetry.PortLastConnection{
				Connected: up,
				MAC:       gwMAC,
				IP:        lanGateway,
				LastSeen:  time.Now().Unix(),
			}
		}

		ports = append(ports, telemetry.Port{
			IfName:         spec.dev,
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
		IP:               lanIP,
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

	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	return ports, ethTable, uplink, nil
}

func (p *AsusProvider) GetSystemStats() (*telemetry.SystemStats, *telemetry.SysStats, error) {
	return p.GetSystemStatsContext(context.Background())
}

func (p *AsusProvider) GetSystemStatsContext(ctx context.Context) (*telemetry.SystemStats, *telemetry.SysStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
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
	var memTotal, memFree, memAvailable, buffers, cached, used int64
	hasAvailable := false
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
				case "MemAvailable":
					memAvailable = val
					hasAvailable = true
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
		if hasAvailable {
			used = memTotal - memAvailable
		}
		if used < 0 {
			used = 0
		}
		memPct = math.Round((float64(used)/float64(memTotal)*100.0)*10) / 10
	}

	// Load is a run-queue metric, not CPU utilization. Keep it separately.
	cpuPct := -1.0
	if raw, err := os.ReadFile("/proc/stat"); err == nil {
		current := parseCPUSample(string(raw))
		cpuPct = cpuBusyPercent(p.previousCPU, current)
		p.previousCPU = current
	}
	var load1Str, load5Str, load15Str string
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			load1Str = fields[0]
			load5Str = fields[1]
			load15Str = fields[2]

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

	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return &telemetry.SystemStats{
		CPU: func() *float64 {
			if cpuPct < 0 {
				return nil
			}
			return &cpuPct
		}(),
		Mem:    memPct,
		Uptime: uptime,
	}, sysStats, nil
}
