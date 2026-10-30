package broadcom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
type sampleFileCacheKey struct{}
type sampleFileCache struct {
	values map[string][]byte
	errors map[string]error
}
type sampleParentCacheKey struct{}
type sampleParentCache struct {
	done       bool
	chassis    string
	portID     string
	remotePort int
	cached     bool
	err        error
}
type sampleCarrierCacheKey struct{}
type sampleCarrierCache struct {
	values map[string]struct {
		value int
		known bool
	}
}
type commandResult struct {
	output string
	err    error
}
type sampleCommandCache struct {
	mu      sync.Mutex
	results map[string]commandResult
}

type AsusProvider struct {
	runner            CmdRunner
	compactRadioNames bool
	expectedUplinkMAC string
	radioNames        map[string]string
	previousCPU       cpuSample
	retryBaselines    map[string]retryCounterBaseline
	vapCounterState   *vapCounterTracker
	cacheMu           sync.Mutex
	staticCache       map[string]staticCacheEntry
	now               func() time.Time
	readFile          func(string) ([]byte, error)
	readDir           func(string) ([]os.DirEntry, error)
	parentCache       *verifiedParent
	staIncompleteMu   sync.Mutex
	stationIncomplete bool
}

type verifiedParent struct {
	localInterface string
	chassisMAC     string
	portID         string
	remotePort     int
	verifiedAt     time.Time
}

// AsusProviderOptions controls startup-only identity behavior.
// CompactRadioNames requires a controller identity migration on adopted APs.
type AsusProviderOptions struct {
	CompactRadioNames bool
	// ExpectedUplinkMAC permits a locally-scoped LLDP chassis ID only when it
	// exactly matches this explicit configured parent identity.
	ExpectedUplinkMAC string
}

const parentCacheTTL = 2 * time.Minute

type retryCounterBaseline struct {
	packets        int64
	retries        int64
	source         string
	packetSource   string
	uptime         int64
	uptimePresent  bool
	packetsPresent bool
	retriesPresent bool
	at             time.Time
}

func NewAsusProvider(runner CmdRunner) *AsusProvider {
	return NewAsusProviderWithOptions(runner, AsusProviderOptions{})
}

func NewAsusProviderWithOptions(runner CmdRunner, options AsusProviderOptions) *AsusProvider {
	if runner == nil {
		runner = defaultCmdRunner
	}
	expectedUplinkMAC, _ := inform.NormalizeMAC(options.ExpectedUplinkMAC)
	return &AsusProvider{runner: runner, compactRadioNames: options.CompactRadioNames, expectedUplinkMAC: expectedUplinkMAC, staticCache: make(map[string]staticCacheEntry), retryBaselines: make(map[string]retryCounterBaseline), vapCounterState: newVAPCounterTracker(), now: time.Now, readFile: os.ReadFile, readDir: os.ReadDir}
}

func (p *AsusProvider) markStationIncomplete(iface, mac string) {
	p.staIncompleteMu.Lock()
	defer p.staIncompleteMu.Unlock()
	p.stationIncomplete = true
}

func (p *AsusProvider) isStationSampleIncomplete() bool {
	p.staIncompleteMu.Lock()
	defer p.staIncompleteMu.Unlock()
	return p.stationIncomplete
}

func (p *AsusProvider) resetStationSampleIncomplete() bool {
	p.staIncompleteMu.Lock()
	defer p.staIncompleteMu.Unlock()
	val := p.stationIncomplete
	p.stationIncomplete = false
	return val
}

func (p *AsusProvider) Collect(ctx context.Context, mapping map[string]string) (*providers.Sample, error) {
	p.resetStationSampleIncomplete()
	ctx = context.WithValue(ctx, sampleCommandCacheKey{}, &sampleCommandCache{results: make(map[string]commandResult)})
	ctx = context.WithValue(ctx, sampleFileCacheKey{}, &sampleFileCache{values: make(map[string][]byte), errors: make(map[string]error)})
	ctx = context.WithValue(ctx, sampleParentCacheKey{}, &sampleParentCache{})
	ctx = context.WithValue(ctx, sampleCarrierCacheKey{}, &sampleCarrierCache{values: make(map[string]struct {
		value int
		known bool
	})})
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
	p.applyRetryInterval(vaps, started)
	p.vapCounterState.Observe(vaps, started)
	sample := &providers.Sample{Radios: radios, VAPs: vaps, StartedAt: started, StageErrors: make(map[string]string)}
	if p.resetStationSampleIncomplete() {
		sample.StageErrors["stations"] = "incomplete station details"
	}
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
	// Enrich local diagnostics only after every required sample stage has
	// completed, so optional driver commands cannot starve core telemetry.
	p.enrichCCAInterferenceContext(ctx, sample.RadioStats)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sample.CompletedAt = time.Now()
	return sample, nil
}

func (p *AsusProvider) readSampleFile(ctx context.Context, path string) ([]byte, error) {
	reader := p.readFile
	if reader == nil {
		reader = os.ReadFile
	}
	cache, ok := ctx.Value(sampleFileCacheKey{}).(*sampleFileCache)
	if !ok {
		return reader(path)
	}
	if value, exists := cache.values[path]; exists {
		return append([]byte(nil), value...), nil
	}
	if err, exists := cache.errors[path]; exists {
		return nil, err
	}
	value, err := reader(path)
	if err != nil {
		cache.errors[path] = err
		return nil, err
	}
	cache.values[path] = append([]byte(nil), value...)
	return value, nil
}

func (p *AsusProvider) readCarrier(ctx context.Context, iface string) (int, bool) {
	if cache, ok := ctx.Value(sampleCarrierCacheKey{}).(*sampleCarrierCache); ok {
		if result, exists := cache.values[iface]; exists {
			return result.value, result.known
		}
		value, known := readSysfsIntWith(p.readFile, "/sys/class/net/"+iface+"/carrier")
		known = known && (value == 0 || value == 1)
		cache.values[iface] = struct {
			value int
			known bool
		}{value, known}
		return value, known
	}
	value, known := readSysfsIntWith(p.readFile, "/sys/class/net/"+iface+"/carrier")
	return value, known && (value == 0 || value == 1)
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
			if p.compactRadioNames {
				radioName += strconv.Itoa(bandCounts[band])
			} else {
				radioName += "-" + strconv.Itoa(bandCounts[band])
			}
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
		cuTotal, cuSelfRx, cuSelfTx, _, err := p.parseChanimStatsStrict(ctx, iface)
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
			Is11ac:         hasVHT,
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
	var netDev map[string]netDevCounters
	if raw, err := p.readSampleFile(ctx, "/proc/net/dev"); err == nil {
		netDev = parseNetDev(string(raw))
	}

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

		bssidRaw, err := p.wlCmdContext(ctx, iface, "bssid")
		if err != nil {
			return nil, fmt.Errorf("read %s BSSID: %w", iface, err)
		}
		bssid, err := inform.NormalizeMAC(bssidRaw)
		if err != nil {
			return nil, fmt.Errorf("read %s BSSID: invalid value", iface)
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

		var vapTxBytes, vapRxBytes, vapTxPackets, vapRxPackets *int64
		if counters, ok := netDev[iface]; ok {
			vapTxBytes, vapRxBytes = int64ptr(counters.txBytes), int64ptr(counters.rxBytes)
			vapTxPackets, vapRxPackets = int64ptr(counters.txPackets), int64ptr(counters.rxPackets)
		}

		vap := telemetry.VAP{
			Name:          vapName,
			InterfaceName: iface,
			Radio:         band,
			RadioName:     radioName,
			MAC:           bssid,
			BSSID:         bssid,
			ESSID:         ssid,
			Channel:       channel,
			State:         "RUN",
			Up:            true,
			CountersKnown: vapTxBytes != nil,
			IsGuest:       false,
			NumSta:        len(stations),
			TxBytes:       vapTxBytes,
			RxBytes:       vapRxBytes,
			TxPackets:     vapTxPackets,
			RxPackets:     vapRxPackets,
			StaTable:      stations,
		}

		if wlanID, ok := wlanMapping[ssid]; ok && telemetry.ValidLegacyWlanID(wlanID) {
			vap.WlanID = wlanID
		}

		vaps = append(vaps, vap)
	}

	return vaps, nil
}

func int64ptr(v int64) *int64 { return &v }

func (p *AsusProvider) applyRetryInterval(vaps []telemetry.VAP, sampledAt time.Time) {
	if p.retryBaselines == nil {
		p.retryBaselines = make(map[string]retryCounterBaseline)
	}
	seen := make(map[string]struct{})
	for vi := range vaps {
		for si := range vaps[vi].StaTable {
			sta := &vaps[vi].StaTable[si]
			ifName := vaps[vi].InterfaceName
			if ifName == "" {
				ifName = vaps[vi].Name
			}
			key := ifName + "|" + strings.ToLower(vaps[vi].BSSID) + "|" + strings.ToLower(sta.MAC)
			seen[key] = struct{}{}
			previous, exists := p.retryBaselines[key]
			current := retryCounterBaseline{packets: sta.TxPackets, retries: sta.TxRetries, source: sta.RetryCounterSource, packetSource: sta.TxPacketCounterSource, uptime: sta.Uptime, uptimePresent: sta.UptimePresent, at: sampledAt}
			compatible := exists && sta.TxPacketsPresent && sta.TxRetriesPresent && sta.UptimePresent && previous.uptimePresent && sta.TxPacketCounterSource == "tx_ucast_pkts" && previous.packetSource == current.packetSource && sta.RetryCounterSource == "tx_pkts_retries" && previous.source == current.source && previous.at.Before(sampledAt) && previous.packetsPresent && previous.retriesPresent
			if compatible && sta.UptimePresent && previous.uptimePresent && sta.Uptime < previous.uptime {
				compatible = false
			}
			if compatible && (sta.TxPackets < previous.packets || sta.TxRetries < previous.retries) {
				compatible = false
			}
			if compatible {
				deltaPackets := sta.TxPackets - previous.packets
				deltaRetries := sta.TxRetries - previous.retries
				if deltaPackets > 0 && sta.Signal != 0 {
					sta.Satisfaction = CalculateSatisfaction(sta.Signal, sta.RSSI, deltaPackets, deltaRetries)
				}
			}
			current.packetsPresent = sta.TxPacketsPresent
			current.retriesPresent = sta.TxRetriesPresent
			p.retryBaselines[key] = current
		}
	}
	for key := range p.retryBaselines {
		if _, ok := seen[key]; !ok {
			delete(p.retryBaselines, key)
		}
	}
}

func (p *AsusProvider) getStationsForIface(iface, vapName, bssid, ssid string, channel int, band string) []telemetry.Station {
	stations, _ := p.getStationsForIfaceContext(context.Background(), iface, vapName, bssid, ssid, channel, band)
	return stations
}

func isStationDepartedOutput(err error, output string) bool {
	var msg string
	if err != nil {
		msg = strings.ToLower(err.Error())
	}
	out := strings.ToLower(output)
	return strings.Contains(msg, "bad address") ||
		strings.Contains(msg, "not associated") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such device") ||
		strings.Contains(out, "bad address") ||
		strings.Contains(out, "not associated") ||
		strings.Contains(out, "not found")
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

	rawMACs := []string{}
	for _, field := range strings.Fields(assocRaw) {
		if field == "assoclist" {
			continue
		}
		rawMACs = append(rawMACs, field)
	}

	for _, rawMAC := range rawMACs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		normMAC, err := inform.NormalizeMAC(rawMAC)
		if err != nil {
			return nil, fmt.Errorf("parse %s association list: malformed station identifier %q", iface, rawMAC)
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
		if err != nil || len(infoRaw) == 0 {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Check if the driver explicitly indicates client is no longer associated
			if isStationDepartedOutput(err, infoRaw) {
				// Client departed between assoclist and sta_info read.
				continue
			}
			// Bounded recheck: confirm if client is still in assoclist
			recheckRaw, recheckErr := p.wlCmdContext(ctx, iface, "assoclist")
			if recheckErr == nil && !strings.Contains(recheckRaw, rawMAC) {
				// Confirmed departed on assoclist recheck
				continue
			}
			// Station is still associated or driver command had a generic failure/timeout.
			// Retain station in sample to prevent false disconnect/departure events,
			// but mark station sample as incomplete.
			p.markStationIncomplete(iface, rawMAC)
			stations = append(stations, sta)
			continue
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
	cuTotal, cuSelfRx, cuSelfTx, _, _ = p.parseChanimStatsStrict(ctx, iface)
	return
}

func (p *AsusProvider) parseChanimStatsStrict(ctx context.Context, iface string) (cuTotal int, cuSelfRx int, cuSelfTx int, details telemetry.ChanimDetails, resultErr error) {
	raw, err := p.wlCmdContext(ctx, iface, "chanim_stats")
	if err != nil {
		return 0, 0, 0, details, err
	}
	if len(strings.TrimSpace(raw)) == 0 {
		return 0, 0, 0, details, fmt.Errorf("empty response")
	}
	details = telemetry.ParseChanimDetails(raw)
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
				return 0, 0, 0, details, fmt.Errorf("malformed utilization row")
			}
			cuSelfTx, err = strconv.Atoi(fields[txIdx])
			if err != nil {
				return 0, 0, 0, details, fmt.Errorf("invalid tx utilization: %w", err)
			}
			cuSelfRx, err = strconv.Atoi(fields[rxIdx])
			if err != nil {
				return 0, 0, 0, details, fmt.Errorf("invalid inbss utilization: %w", err)
			}
			idle, err := strconv.Atoi(fields[idleIdx])
			if err != nil || idle < 0 || idle > 100 {
				return 0, 0, 0, details, fmt.Errorf("invalid idle utilization %q", fields[idleIdx])
			}
			cuTotal = 100 - idle
			return cuTotal, cuSelfRx, cuSelfTx, details, nil
		}
	}
	return 0, 0, 0, details, fmt.Errorf("unrecognized response")
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
		channel, width := ParseChanspec(chanspecRaw)
		if channel <= 0 || width <= 0 {
			return nil, fmt.Errorf("read %s chanspec: invalid value %q", iface, strings.TrimSpace(chanspecRaw))
		}
		band := BandForChannel(channel)
		radioName := p.radioNames[iface]
		if radioName == "" {
			radioName = "wifi-" + band
		}

		cuTotal, cuSelfRx, cuSelfTx, rfDetails, err := p.parseChanimStatsStrict(ctx, iface)
		if err != nil {
			return nil, fmt.Errorf("read %s channel utilization: %w", iface, err)
		}

		stats = append(stats, telemetry.RadioStat{
			Bandwidth: width,
			Name:      radioName,
			Channel:   channel,
			NumSta:    0, // Populated dynamically by bridge daemon based on active VAPs
			CuTotal:   cuTotal,
			CuSelfRx:  cuSelfRx,
			CuSelfTx:  cuSelfTx,
			CuOBSS:    rfDetails.OBSSPercent,
			CuNoCat:   rfDetails.NoCatPercent,
			CuNoPkt:   rfDetails.NoPktPercent,
			NoiseDBm:  rfDetails.NoiseDBm,
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
	valid     bool
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
		if iface == "" || len(fields) < 16 {
			continue
		}
		parsed := make([]int64, 16)
		valid := true
		for i, field := range fields[:16] {
			value, err := strconv.ParseUint(field, 10, 63)
			if err != nil {
				valid = false
				break
			}
			parsed[i] = int64(value)
		}
		if valid {
			rxB, rxP, rxE, rxD := parsed[0], parsed[1], parsed[2], parsed[3]
			txB, txP, txE, txD := parsed[8], parsed[9], parsed[10], parsed[11]
			res[iface] = netDevCounters{
				rxBytes:   rxB,
				rxPackets: rxP,
				rxErrors:  rxE,
				rxDropped: rxD,
				txBytes:   txB,
				txPackets: txP,
				txErrors:  txE,
				txDropped: txD,
				valid:     true,
			}
		}
	}
	return res
}

func configuredGatewayIP(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	ipv4 := ip.To4()
	if ipv4 == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ipv4.Equal(net.IPv4bcast) {
		return ""
	}
	return ipv4.String()
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
	chassisMAC, portMAC, remotePort, _, err = p.resolveLLDPNeighborContext(ctx)
	return
}

func (p *AsusProvider) resolveLLDPNeighborContext(ctx context.Context) (chassisMAC string, portID string, remotePort int, cached bool, err error) {
	if sample, ok := ctx.Value(sampleParentCacheKey{}).(*sampleParentCache); ok {
		if sample.done {
			return sample.chassis, sample.portID, sample.remotePort, sample.cached, sample.err
		}
		defer func() {
			sample.done = true
			sample.chassis, sample.portID, sample.remotePort, sample.cached, sample.err = chassisMAC, portID, remotePort, cached, err
		}()
	}
	out, err := p.runCommand(ctx, "lldpcli", "-f", "json", "show", "neighbors")
	if err != nil || len(out) == 0 {
		if ctx.Err() != nil {
			return "", "", 0, false, ctx.Err()
		}
		if err == nil {
			err = fmt.Errorf("empty lldpcli response")
		}
		return p.cachedParent(err)
	}

	var data lldpCliOutput
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return p.cachedParent(fmt.Errorf("json unmarshal failed: %w", err))
	}

	ifaceData, ok := data.LLDP.Interface["eth0"]
	if !ok || len(ifaceData.Chassis) == 0 {
		// A successful, valid response without an eth0 neighbor is authoritative.
		p.parentCache = nil
		return "", "", 0, false, nil
	}
	if len(ifaceData.Chassis) != 1 {
		return p.cachedParent(fmt.Errorf("ambiguous eth0 neighbor"))
	}
	var neighborID struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	for _, item := range ifaceData.Chassis {
		neighborID = item.ID
	}
	chassisMAC, err = inform.NormalizeMAC(neighborID.Value)
	if err != nil || !isUnicastMAC(chassisMAC) {
		return p.cachedParent(fmt.Errorf("invalid chassis MAC"))
	}
	localChassisID := isLocalIDSubtype(neighborID.Type)
	if !isMACIDSubtype(neighborID.Type) && !localChassisID {
		return p.cachedParent(fmt.Errorf("unsupported chassis ID subtype %q", neighborID.Type))
	}
	portID = strings.TrimSpace(ifaceData.Port.ID.Value)
	portType := strings.ToLower(strings.TrimSpace(ifaceData.Port.ID.Type))
	if portType == "mac" || portType == "mac address" {
		portID, err = inform.NormalizeMAC(portID)
		if err != nil || !isUnicastMAC(portID) {
			return p.cachedParent(fmt.Errorf("invalid port MAC"))
		}
	} else if portID != "" && !isPortNameSubtype(portType) && !isLocalIDSubtype(portType) {
		return p.cachedParent(fmt.Errorf("unsupported port ID subtype %q", portType))
	}
	descr := strings.TrimSpace(ifaceData.Port.Descr)
	idName, idNameKnown := "", false
	if isPortNameSubtype(portType) {
		idName, idNameKnown = normalizeEthernetIfname(ifaceData.Port.ID.Value)
	}
	descrName, descrNameKnown := normalizeEthernetIfname(descr)
	if idNameKnown && descrNameKnown && idName != descrName {
		return p.cachedParent(fmt.Errorf("conflicting remote port identifiers"))
	}
	remotePort = remotePortIndex(idName)
	if !idNameKnown {
		remotePort = remotePortIndex(descrName)
	}
	if portID == "" {
		portID = descr
	}
	if portID == "" {
		return p.cachedParent(fmt.Errorf("empty port ID"))
	}
	if localChassisID && (p.expectedUplinkMAC == "" || chassisMAC != p.expectedUplinkMAC || remotePort == 0) {
		return p.cachedParent(fmt.Errorf("unverified local chassis ID"))
	}
	verified := p.now()
	p.parentCache = &verifiedParent{localInterface: "eth0", chassisMAC: chassisMAC, portID: portID, remotePort: remotePort, verifiedAt: verified}
	return chassisMAC, portID, remotePort, false, nil
}

func (p *AsusProvider) cachedParent(cause error) (string, string, int, bool, error) {
	parent := p.parentCache
	if parent == nil {
		return "", "", 0, false, cause
	}
	now := p.now()
	if now.Before(parent.verifiedAt) || now.Sub(parent.verifiedAt) >= parentCacheTTL || parent.localInterface != "eth0" {
		p.parentCache = nil
		return "", "", 0, false, cause
	}
	return parent.chassisMAC, parent.portID, parent.remotePort, true, nil
}

func isMACIDSubtype(subtype string) bool {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "mac", "mac address":
		return true
	default:
		return false
	}
}

func isPortNameSubtype(subtype string) bool {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "interface name", "ifname", "interface name (ifname)":
		return true
	default:
		return false
	}
}

func isLocalIDSubtype(subtype string) bool {
	return strings.EqualFold(strings.TrimSpace(subtype), "local")
}

func isUnicastMAC(mac string) bool {
	if len(mac) != 17 {
		return false
	}
	first, err := strconv.ParseUint(mac[:2], 16, 8)
	return err == nil && first&1 == 0 && mac != "00:00:00:00:00:00"
}

func parseRemoteEthernetPort(subtype, value string) (int, bool) {
	if !isPortNameSubtype(subtype) {
		return 0, false
	}
	name, ok := normalizeEthernetIfname(value)
	if !ok {
		return 0, false
	}
	port := remotePortIndex(name)
	return port, port != 0
}

func normalizeEthernetIfname(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "eth") || len(value) <= 3 {
		return "", false
	}
	number := value[3:]
	for _, r := range number {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	n, err := strconv.Atoi(number)
	if err != nil {
		return "", false
	}
	return "eth" + strconv.Itoa(n), true
}

func remotePortIndex(ifname string) int {
	if len(ifname) != 4 || !strings.HasPrefix(ifname, "eth") || ifname[3] < '0' || ifname[3] > '3' {
		return 0
	}
	return int(ifname[3]-'0') + 1
}

func (p *AsusProvider) GetLLDPTable() ([]telemetry.LLDPEntry, error) {
	return p.GetLLDPTableContext(context.Background())
}

func (p *AsusProvider) GetLLDPTableContext(ctx context.Context) ([]telemetry.LLDPEntry, error) {
	if carrier, known := p.readCarrier(ctx, "eth0"); known && carrier == 0 {
		p.parentCache = nil
		return nil, nil
	}
	chassisMAC, portMAC, _, _, err := p.resolveLLDPNeighborContext(ctx)
	if err == nil && chassisMAC != "" {
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

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, err
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

func readSysfsIntWith(readFile func(string) ([]byte, error), path string) (int, bool) {
	if readFile == nil {
		return 0, false
	}
	data, err := readFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return value, true
}

func readSysfsStringWith(readFile func(string) ([]byte, error), path string) string {
	if readFile == nil {
		return ""
	}
	data, err := readFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysfsString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

type arpRecord struct {
	ip  string
	mac string
	dev string
}

func parseArpTableEntries(data string) map[string]arpRecord {
	arpMap := make(map[string]arpRecord)
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			// Require completed or permanent entry (Flags ATF_COM 0x2 or ATF_PERM 0x4).
			// Incomplete or failed entries (Flags 0x0) must be ignored.
			flags, err := strconv.ParseInt(strings.TrimPrefix(fields[2], "0x"), 16, 64)
			if err != nil || (flags&0x02) == 0 {
				continue
			}
			ip := fields[0]
			mac := strings.ToLower(fields[3])
			norm, err := inform.NormalizeMAC(mac)
			if err != nil || !isUnicastMAC(norm) || strings.HasPrefix(norm, "00:00:00") {
				continue
			}
			dev := ""
			if len(fields) >= 6 {
				dev = fields[5]
			}
			arpMap[norm] = arpRecord{
				ip:  ip,
				mac: norm,
				dev: dev,
			}
		}
	}
	return arpMap
}

func parseArpTable(data string) map[string]string {
	entries := parseArpTableEntries(data)
	arpMap := make(map[string]string, len(entries))
	for mac, entry := range entries {
		arpMap[mac] = entry.ip
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

		if isLocal == "yes" {
			continue
		}
		mac, err := inform.NormalizeMAC(mac)
		if err != nil || !isUnicastMAC(mac) {
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

		ageF, err := strconv.ParseFloat(ageStr, 64)
		if err != nil || math.IsNaN(ageF) || math.IsInf(ageF, 0) || ageF < 0 || ageF > float64(math.MaxInt) {
			continue
		}
		ageSec := int(ageF)
		duplicate := false
		for _, entry := range macsByPort[portIdx] {
			if entry.MAC == mac {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
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

type bridgeInventory struct {
	name          string
	ports         map[int]string
	vlanFiltering bool
	vlanByIf      map[string][]int
}

func getBridgePortMap() map[int]string {
	return getBridgePortMapWith(os.ReadDir, os.ReadFile)
}

func getBridgePortMapWith(readDir func(string) ([]os.DirEntry, error), readFile func(string) ([]byte, error)) map[int]string {
	bridges := discoverBridgeInventory(context.Background(), readDir, readFile, nil)
	merged := make(map[int]string)
	for _, bridge := range bridges {
		for idx, ifname := range bridge.ports {
			merged[idx] = ifname
		}
	}
	return merged
}

func discoverBridgeInventory(ctx context.Context, readDir func(string) ([]os.DirEntry, error), readFile func(string) ([]byte, error), run func(context.Context, string, ...string) (string, error)) map[string]bridgeInventory {
	if readDir == nil {
		readDir = os.ReadDir
	}
	if readFile == nil {
		readFile = os.ReadFile
	}
	bridges := make(map[string]bridgeInventory)
	netEntries, err := readDir("/sys/class/net")
	if err != nil {
		return bridges
	}
	for _, netEntry := range netEntries {
		if ctx.Err() != nil {
			return bridges
		}
		name := netEntry.Name()
		brifPath := filepath.Join("/sys/class/net", name, "brif")
		members, err := readDir(brifPath)
		if err != nil {
			continue
		}
		inv := bridgeInventory{name: name, ports: make(map[int]string), vlanByIf: make(map[string][]int)}
		for _, member := range members {
			portNoPath := filepath.Join(brifPath, member.Name(), "port_no")
			data, err := readFile(portNoPath)
			if err != nil {
				continue
			}
			n := strings.TrimPrefix(strings.TrimSpace(string(data)), "0x")
			port, err := strconv.ParseInt(n, 16, 32)
			if err == nil && port > 0 {
				inv.ports[int(port)] = member.Name()
			}
		}
		filtering, err := readFile(filepath.Join("/sys/class/net", name, "bridge", "vlan_filtering"))
		inv.vlanFiltering = err == nil && strings.TrimSpace(string(filtering)) == "1"
		if inv.vlanFiltering && run != nil {
			for _, member := range members {
				if ctx.Err() != nil {
					return bridges
				}
				if out, err := run(ctx, "bridge", "vlan", "show", "dev", member.Name()); err == nil {
					for iface, vlans := range parseBridgeVLANShow(out) {
						inv.vlanByIf[iface] = vlans
					}
				}
			}
		}
		bridges[name] = inv
	}
	return bridges
}

func parseBridgeVLANShow(data string) map[string][]int {
	result := make(map[string][]int)
	current := ""
	seen := make(map[string]map[int]bool)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if fields[0] == "port" || fields[0] == "dev" {
				current = ""
				continue
			}
			current = fields[0]
		}
		if current == "" {
			continue
		}
		values := fields[1:]
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			values = fields
		}
		for _, field := range values {
			field = strings.TrimSuffix(field, "*")
			if strings.Contains(field, "-") {
				bounds := strings.Split(field, "-")
				if len(bounds) != 2 {
					addVLAN(seen, current, 0)
					continue
				}
				first, errFirst := strconv.Atoi(bounds[0])
				last, errLast := strconv.Atoi(bounds[1])
				if errFirst != nil || errLast != nil || first < 1 || last > 4094 || first > last {
					addVLAN(seen, current, 0)
					continue
				}
				for vlan := first; vlan <= last; vlan++ {
					addVLAN(seen, current, vlan)
				}
				continue
			}
			v, err := strconv.Atoi(field)
			if err != nil || v < 1 || v > 4094 {
				continue
			}
			addVLAN(seen, current, v)
		}
	}
	for iface, ids := range seen {
		for id := range ids {
			result[iface] = append(result[iface], id)
		}
		sort.Ints(result[iface])
	}
	return result
}

func addVLAN(seen map[string]map[int]bool, iface string, vlan int) {
	if seen[iface] == nil {
		seen[iface] = make(map[int]bool)
	}
	seen[iface][vlan] = true
}

func parseVLANFromName(name string) int {
	if strings.HasPrefix(name, "wl") || strings.HasPrefix(name, "ath") || strings.HasPrefix(name, "wlan") || strings.HasPrefix(name, "ra") {
		return 0
	}
	dot := strings.LastIndex(name, ".")
	if dot == -1 || dot == len(name)-1 {
		return 0
	}
	vid, err := strconv.Atoi(name[dot+1:])
	if err != nil || vid < 1 || vid > 4094 {
		return 0
	}
	return vid
}

func parseBridgeVLAN(name string) int {
	lower := strings.ToLower(name)
	if lower == "br0" {
		return 1
	}
	prefixes := []string{"br-vlan", "br_vlan", "br-lan-", "vlan", "br."}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			if vid, err := strconv.Atoi(strings.TrimPrefix(lower, p)); err == nil && vid >= 1 && vid <= 4094 {
				return vid
			}
		}
	}
	if strings.HasPrefix(lower, "br") {
		rest := strings.TrimPrefix(lower, "br")
		if vid, err := strconv.Atoi(rest); err == nil && vid >= 1 && vid <= 4094 {
			return vid
		}
	}
	return 0
}

func effectiveVLAN(inv bridgeInventory, iface string) int {
	if inv.vlanFiltering {
		v := inv.vlanByIf[iface]
		if len(v) != 1 {
			return 0
		}
		return v[0]
	}

	if vid := parseVLANFromName(iface); vid > 0 {
		return vid
	}
	for _, member := range inv.ports {
		if vid := parseVLANFromName(member); vid > 0 {
			return vid
		}
	}
	if vid := parseBridgeVLAN(inv.name); vid > 0 {
		return vid
	}
	return 0
}

func bridgeNameForInterface(bridges map[string]bridgeInventory, iface string) (string, bool) {
	for name, inv := range bridges {
		for _, member := range inv.ports {
			if member == iface {
				return name, true
			}
		}
	}
	return "", false
}

func assignPortVLANs(entries map[int][]telemetry.PortMacEntry, portMap map[int]string, inv bridgeInventory) {
	for portIdx, macs := range entries {
		vlan := effectiveVLAN(inv, portMap[portIdx])
		if vlan == 0 {
			continue
		}
		for i := range macs {
			macs[i].Vlan = vlan
		}
		entries[portIdx] = macs
	}
}

func physicalPortMap(bridgePorts map[int]string) map[int]string {
	indexes := map[string]int{"eth0": 1, "eth1": 2, "eth2": 3, "eth3": 4}
	result := make(map[int]string)
	for _, ifname := range bridgePorts {
		if idx := indexes[ifname]; idx != 0 {
			result[idx] = ifname
		}
	}
	return result
}

func (p *AsusProvider) GetPortTable() ([]telemetry.Port, []telemetry.EthernetPort, *telemetry.Uplink, error) {
	return p.GetPortTableContext(context.Background())
}

func (p *AsusProvider) GetPortTableContext(ctx context.Context) ([]telemetry.Port, []telemetry.EthernetPort, *telemetry.Uplink, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	ifaces := []struct {
		idx      int
		dev      string
		name     string
		isUplink bool
		defMedia string
	}{
		{1, "eth0", "Port 1", true, "2.5GE"},
		{2, "eth1", "Port 2", false, "GE"},
		{3, "eth2", "Port 3", false, "GE"},
		{4, "eth3", "Port 4", false, "GE"},
	}
	carrierState := struct {
		value int
		known bool
	}{}
	carriers := make(map[string]struct {
		value int
		known bool
	}, len(ifaces))
	for _, spec := range ifaces {
		value, known := p.readCarrier(ctx, spec.dev)
		carriers[spec.dev] = struct {
			value int
			known bool
		}{value, known}
		if spec.isUplink {
			carrierState = carriers[spec.dev]
		}
	}
	if carrierState.known && carrierState.value == 0 {
		p.parentCache = nil
	}
	readFile := p.readFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	netDevRaw, _ := p.readSampleFile(ctx, "/proc/net/dev")
	counters := parseNetDev(string(netDevRaw))

	hostMAC, err := p.nvramGetContext(ctx, "lan_hwaddr")
	if err != nil {
		return nil, nil, nil, err
	}
	if norm, err := inform.NormalizeMAC(hostMAC); err == nil {
		hostMAC = norm
	}

	bridges := discoverBridgeInventory(ctx, p.readDir, readFile, p.runCommand)
	arpData, _ := readFile("/proc/net/arp")
	arpEntries := parseArpTableEntries(string(arpData))
	arpMap := make(map[string]string, len(arpEntries))
	for mac, entry := range arpEntries {
		arpMap[mac] = entry.ip
	}
	macsByPort := make(map[int][]telemetry.PortMacEntry)
	for bridgeName, inv := range bridges {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		showmacsOut, cmdErr := p.runCommand(ctx, "brctl", "showmacs", bridgeName)
		if cmdErr != nil {
			if ctx.Err() != nil {
				return nil, nil, nil, ctx.Err()
			}
			continue
		}
		bridgeMacs, _ := parseBrctlShowmacs(showmacsOut, inv.ports, arpMap)
		assignPortVLANs(bridgeMacs, physicalPortMap(inv.ports), inv)
		for portIdx, entries := range bridgeMacs {
			for _, entry := range entries {
				duplicate := false
				for _, existing := range macsByPort[portIdx] {
					if existing.MAC == entry.MAC {
						duplicate = true
						break
					}
				}
				if !duplicate {
					macsByPort[portIdx] = append(macsByPort[portIdx], entry)
				}
			}
		}
	}
	portDevMap := map[int]string{1: "eth0", 2: "eth1", 3: "eth2", 4: "eth3"}
	lastConnByPort := make(map[int]*telemetry.PortLastConnection)
	for portIdx, entries := range macsByPort {
		if len(entries) != 1 {
			continue
		}
		entry := entries[0]
		portDev := portDevMap[portIdx]
		bridgeName, _ := bridgeNameForInterface(bridges, portDev)
		clientIP := ""
		if arp, ok := arpEntries[entry.MAC]; ok {
			// Validate that the ARP entry was learned on the bridge or device corresponding to this physical port.
			// This prevents cross-bridge IP poisoning when an AP has multiple isolated VLAN bridges.
			if (bridgeName != "" && arp.dev == bridgeName) || arp.dev == portDev || bridgeName == "" {
				clientIP = arp.ip
			}
		}
		lastConnByPort[portIdx] = &telemetry.PortLastConnection{
			Connected: true,
			MAC:       entry.MAC,
			IP:        clientIP,
			LastSeen:  time.Now().Unix() - int64(entry.Age),
		}
	}

	chassisMAC, _, remotePort, parentCached, err := "", "", 0, false, error(nil)
	if !carrierState.known || carrierState.value == 1 {
		chassisMAC, _, remotePort, parentCached, err = p.resolveLLDPNeighborContext(ctx)
		if ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
	}
	gwMAC := chassisMAC
	uplinkSource := ""
	if chassisMAC != "" && parentCached {
		uplinkSource = "lldp_uplink_cached"
	} else if chassisMAC != "" {
		uplinkSource = "lldp_uplink"
	}
	if gwMAC == "" && !(carrierState.known && carrierState.value == 0) {
		remotePort = 0
	}
	lanGateway, err := p.nvramGetContext(ctx, "lan_gateway")
	if err != nil {
		return nil, nil, nil, err
	}
	lanGateway = configuredGatewayIP(lanGateway)
	lanIP, err := p.nvramGetContext(ctx, "lan_ipaddr")
	if err != nil {
		return nil, nil, nil, err
	}

	ports := make([]telemetry.Port, 0, len(ifaces))
	for _, spec := range ifaces {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		carrier := carriers[spec.dev]
		speed, _ := readSysfsIntWith(readFile, fmt.Sprintf("/sys/class/net/%s/speed", spec.dev))
		duplexStr := readSysfsStringWith(readFile, fmt.Sprintf("/sys/class/net/%s/duplex", spec.dev))

		up := carrier.known && carrier.value == 1
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
		stpState := ""
		stpCost := 0
		if bridgeName, ok := bridgeNameForInterface(bridges, spec.dev); ok {
			stpState = stpStates[readSysfsStringWith(readFile, filepath.Join("/sys/class/net", bridgeName, "brif", spec.dev, "state"))]
			stpCost, _ = readSysfsIntWith(readFile, filepath.Join("/sys/class/net", bridgeName, "brif", spec.dev, "path_cost"))
		}
		if stpCost < 0 {
			stpCost = 0
		}

		c := counters[spec.dev]

		portMacs := macsByPort[spec.idx]
		lastConn := lastConnByPort[spec.idx]
		if carrier.known && !up {
			portMacs, lastConn = nil, nil
		}
		if spec.isUplink && up && gwMAC != "" && lastConn == nil {
			lastSeen := int64(0)
			if strings.HasPrefix(uplinkSource, "lldp_uplink") && p.parentCache != nil {
				lastSeen = p.parentCache.verifiedAt.Unix()
			}
			lastConn = &telemetry.PortLastConnection{
				Connected: up,
				MAC:       gwMAC,
				IP:        lanGateway,
				LastSeen:  lastSeen,
			}
		}

		ports = append(ports, telemetry.Port{
			IfName:         spec.dev,
			PortIdx:        spec.idx,
			Name:           spec.name,
			Media:          media,
			Up:             up,
			LinkKnown:      carrier.known,
			CountersKnown:  c.valid,
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
			Up:         pt.Up,
			LinkKnown:  pt.LinkKnown,
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
		UplinkSource:     uplinkSource,
		UplinkMAC:        gwMAC,
		UplinkRemotePort: remotePort,
		LinkKnown:        carrierState.known,
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
