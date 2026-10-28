package broadcom

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
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
	provider.readFile = func(path string) ([]byte, error) {
		if path == "/proc/net/dev" {
			return []byte("Inter-| Receive | Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n eth6: 1 2 0 0 0 0 0 0 1048576 1000 0 0 0 0 0 0\n"), nil
		}
		return nil, errors.New("not in fixture")
	}

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
	if vap5G.TxBytes == nil || vap5G.RxBytes == nil || vap5G.TxPackets == nil || vap5G.RxPackets == nil || *vap5G.TxBytes != 1048576 || *vap5G.RxBytes != 1 || *vap5G.TxPackets != 1000 || *vap5G.RxPackets != 2 || !vap5G.CountersKnown {
		t.Errorf("expected measured VAP-interface counters: %+v", vap5G)
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
	if rStats[0].CuOBSS == nil || *rStats[0].CuOBSS != 35 || rStats[0].CuNoCat == nil || *rStats[0].CuNoCat != 11 || rStats[0].CuNoPkt == nil || *rStats[0].CuNoPkt != 6 || rStats[0].NoiseDBm == nil || *rStats[0].NoiseDBm != -91 {
		t.Errorf("expected measured passive CCA/noise fields: %+v", rStats[0])
	}
	// eth6 (5 GHz, Ch 161): idle=75 -> cu_total=25, cu_self_tx=5, cu_self_rx=3
	if rStats[1].CuTotal != 25 || rStats[1].CuSelfTx != 5 || rStats[1].CuSelfRx != 3 {
		t.Errorf("5G radio stats mismatch: %+v", rStats[1])
	}
}

func TestCommandRunnerHonorsParentDeadlineAndPreCancellation(t *testing.T) {
	var calls atomic.Int32
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		calls.Add(1)
		<-ctx.Done()
		return "", ctx.Err()
	}
	p := NewAsusProvider(runner)
	preCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.runCommand(preCanceled, "slow"); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled command error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("pre-canceled context started %d commands", calls.Load())
	}

	parent, stop := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer stop()
	start := time.Now()
	if _, err := p.runCommand(parent, "slow"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline command error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("command exceeded parent deadline by too much: %s", elapsed)
	}
}

func TestDefaultRunnerKillsCommandProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are Unix-specific")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	output, err := defaultCmdRunner(ctx, "sh", "-c", "sleep 30 & echo $!; wait")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runner error = %v", err)
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		t.Fatalf("child PID missing from output %q", output)
	}
	child, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("invalid child PID %q", fields[0])
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(child, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d survived command deadline", child)
}

func TestDefaultRunnerKillsDescendantAfterCommandExits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are Unix-specific")
	}
	output, err := defaultCmdRunner(context.Background(), "sh", "-c", "sleep 30 & echo $!")
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("runner error = %v, want bounded pipe wait", err)
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		t.Fatalf("child PID missing from output %q", output)
	}
	child, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("invalid child PID %q", fields[0])
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(child, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived parent exit", child)
}

func TestDefaultRunnerKillsDetachedDescendantAfterCommandExits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are Unix-specific")
	}
	output, err := defaultCmdRunner(context.Background(), "sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $!")
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		t.Fatalf("child PID missing from output %q", output)
	}
	child, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("invalid child PID %q", fields[0])
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !testProcessAlive(child) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("detached descendant process %d survived parent exit", child)
}

func processStateFromStat(stat string) byte {
	closeParen := strings.LastIndex(stat, ")")
	if closeParen < 0 || closeParen+2 >= len(stat) {
		return 0
	}
	return stat[closeParen+2]
}

func testProcessAlive(pid int) bool {
	if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil && processStateFromStat(string(stat)) == 'Z' {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func TestRadioCollectionRejectsFailedRequiredReads(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		full := name + " " + strings.Join(args, " ")
		switch full {
		case "nvram get wl_ifnames":
			return "eth4", nil
		case "wl -i eth4 chanspec":
			return "6", nil
		case "wl -i eth4 chanim_stats":
			return "", errors.New("command timed out")
		default:
			return "", nil
		}
	}
	if _, err := NewAsusProvider(runner).GetRadiosContext(context.Background()); err == nil || !strings.Contains(err.Error(), "channel utilization") {
		t.Fatalf("failed channel-utilization read became fresh radio data: %v", err)
	}
}

func TestLLDPValidationAndVerifiedParentCache(t *testing.T) {
	now := time.Unix(1_000, 0)
	valid := `{"lldp":{"interface":{"eth0":{"chassis":{"1":{"id":{"type":"mac","value":"6c:63:f8:30:9a:05"}}},"port":{"id":{"type":"mac","value":"6c:63:f8:30:9a:06"},"descr":"eth2"}}}}}`
	response := valid
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "lldpcli" {
			return response, nil
		}
		return "", nil
	}
	p := NewAsusProvider(runner)
	p.now = func() time.Time { return now }
	chassis, portID, remote, cached, err := p.resolveLLDPNeighborContext(context.Background())
	if err != nil || cached || chassis != "6c:63:f8:30:9a:05" || portID != "6c:63:f8:30:9a:06" || remote != 3 {
		t.Fatalf("valid LLDP neighbor parsed incorrectly: %q %q %d cached=%v err=%v", chassis, portID, remote, cached, err)
	}
	response = strings.Replace(valid, `"descr":"eth2"`, `"descr":"uplink"`, 1)
	now = now.Add(10 * time.Second)
	_, _, remote, cached, err = p.resolveLLDPNeighborContext(context.Background())
	if err != nil || cached || remote != 0 || p.parentCache.remotePort != 0 {
		t.Fatalf("new neighbor with unknown remote port retained stale port: remote=%d cached=%v parent=%+v err=%v", remote, cached, p.parentCache, err)
	}
	response = `{invalid json`
	now = now.Add(time.Minute)
	_, _, remote, cached, err = p.resolveLLDPNeighborContext(context.Background())
	if err != nil || !cached || remote != 0 {
		t.Fatalf("brief LLDP failure did not use verified parent: remote=%d cached=%v err=%v", remote, cached, err)
	}
	response = `{"lldp":{"interface":{"eth1":{}}}}`
	_, _, _, cached, err = p.resolveLLDPNeighborContext(context.Background())
	if err != nil || cached || p.parentCache != nil {
		t.Fatalf("authoritative eth1-only response retained eth0 parent: cached=%v parent=%+v err=%v", cached, p.parentCache, err)
	}
	response = valid
	_, _, _, _, err = p.resolveLLDPNeighborContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response = `{invalid json`
	now = now.Add(parentCacheTTL)
	if _, _, _, _, err = p.resolveLLDPNeighborContext(context.Background()); err == nil || p.parentCache != nil {
		t.Fatal("LLDP parent was reused at the exact cache expiry boundary")
	}
	response = valid
	_, _, _, _, err = p.resolveLLDPNeighborContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response = `{invalid json`
	now = time.Unix(1_005, 0)
	if _, _, _, _, err = p.resolveLLDPNeighborContext(context.Background()); err == nil || p.parentCache != nil {
		t.Fatal("clock rollback extended or retained cached LLDP parent")
	}

	response = strings.Replace(valid, `"type":"mac","value":"6c:63:f8:30:9a:05"`, `"type":"local","value":"gateway"`, 1)
	p.parentCache = nil
	if _, _, _, _, err = p.resolveLLDPNeighborContext(context.Background()); err == nil {
		t.Fatal("non-MAC chassis subtype was accepted")
	}
	response = strings.Replace(valid, `"descr":"eth2"`, `"descr":"eth3"`, 1)
	response = strings.Replace(response, `"value":"6c:63:f8:30:9a:06"`, `"value":"eth2"`, 1)
	response = strings.Replace(response, `"type":"mac","value":"eth2"`, `"type":"interface name","value":"eth2"`, 1)
	if _, _, _, _, err = p.resolveLLDPNeighborContext(context.Background()); err == nil {
		t.Fatal("conflicting remote port identifiers were accepted")
	}
	for _, value := range []string{"eth0", "eth2", "eth3"} {
		if _, ok := parseRemoteEthernetPort("interface name", value); !ok {
			t.Errorf("complete interface name %q was rejected", value)
		}
	}
	for _, value := range []string{"eth4", "eth12", "3", "port 3"} {
		if port, ok := parseRemoteEthernetPort("interface name", value); ok || port != 0 {
			t.Errorf("unsupported remote port %q was guessed as %d", value, port)
		}
	}
}

func TestLLDPLocalChassisRequiresConfiguredMACAndUnambiguousPort(t *testing.T) {
	const expectedMAC = "6c:63:f8:30:9a:05"
	base := `{"lldp":{"interface":{"eth0":{"chassis":{"1":{"id":{"type":"local","value":"6c:63:f8:30:9a:05"}}},"port":{"id":{"type":"interface name","value":"eth2"},"descr":"eth2"}}}}}`
	mainObserved := `{"lldp":{"interface":{"eth0":{"chassis":{"1":{"id":{"type":"local","value":"6c:63:f8:30:9a:05"}}},"port":{"id":{"type":"mac","value":"6c:63:f8:30:9a:06"},"descr":"eth0"}}}}}`
	nodeObserved := `{"lldp":{"interface":{"eth0":{"chassis":{"1":{"id":{"type":"local","value":"6c:63:f8:30:9a:05"}}},"port":{"id":{"type":"mac","value":"6c:63:f8:30:9a:07"},"descr":"eth2"}}}}}`
	makeResponse := func(chassis, portID, descr string) string {
		response := strings.Replace(base, `"value":"6c:63:f8:30:9a:05"`, `"value":"`+chassis+`"`, 1)
		response = strings.Replace(response, `"value":"eth2"`, `"value":"`+portID+`"`, 1)
		return strings.Replace(response, `"descr":"eth2"`, `"descr":"`+descr+`"`, 1)
	}
	tests := []struct {
		name         string
		response     string
		configured   string
		wantAccepted bool
		wantPortID   string
		wantRemote   int
	}{
		{name: "matching configured MAC and port", response: base, configured: expectedMAC, wantAccepted: true, wantPortID: "eth2", wantRemote: 3},
		{name: "configured MAC mismatch", response: base, configured: "6c:63:f8:30:9a:06"},
		{name: "missing configured MAC", response: base},
		{name: "malformed chassis MAC", response: makeResponse("gateway", "eth2", "eth2"), configured: expectedMAC},
		{name: "invalid multicast chassis MAC", response: makeResponse("ff:ff:ff:ff:ff:ff", "eth2", "eth2"), configured: "ff:ff:ff:ff:ff:ff"},
		{name: "missing port evidence", response: makeResponse(expectedMAC, "", ""), configured: expectedMAC},
		{name: "ambiguous port evidence", response: makeResponse(expectedMAC, "eth2", "eth3"), configured: expectedMAC},
		{name: "standard MAC subtype remains accepted", response: strings.Replace(base, `"type":"local"`, `"type":"mac"`, 1), wantAccepted: true, wantPortID: "eth2", wantRemote: 3},
		{name: "observed Main LLDP output uses eth0 description", response: mainObserved, configured: expectedMAC, wantAccepted: true, wantPortID: "6c:63:f8:30:9a:06", wantRemote: 1},
		{name: "observed Node LLDP output uses eth2 description", response: nodeObserved, configured: expectedMAC, wantAccepted: true, wantPortID: "6c:63:f8:30:9a:07", wantRemote: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(ctx context.Context, name string, args ...string) (string, error) {
				if name == "lldpcli" {
					return tt.response, nil
				}
				return "", nil
			}
			p := NewAsusProviderWithOptions(runner, AsusProviderOptions{ExpectedUplinkMAC: tt.configured})
			chassis, portID, remote, cached, err := p.resolveLLDPNeighborContext(context.Background())
			if tt.wantAccepted {
				if err != nil || cached || chassis != expectedMAC || portID != tt.wantPortID || remote != tt.wantRemote {
					t.Fatalf("expected verified parent, got chassis=%q port=%q remote=%d cached=%v err=%v", chassis, portID, remote, cached, err)
				}
				return
			}
			if err == nil || chassis != "" || remote != 0 || cached {
				t.Fatalf("expected fail-closed result, got chassis=%q remote=%d cached=%v err=%v", chassis, remote, cached, err)
			}
		})
	}
}

func TestRetryExperienceRequiresCompatibleIntervalAndResetsSafely(t *testing.T) {
	p := NewAsusProvider(nil)
	start := time.Unix(100, 0)
	makeVAP := func(packets, retries int64, source string, uptime int64, present bool) []telemetry.VAP {
		return []telemetry.VAP{{Name: "ath1", InterfaceName: "wl1.1", BSSID: "02:00:00:00:00:01", StaTable: []telemetry.Station{{
			MAC: "02:00:00:00:00:11", Signal: -60, RSSI: 30,
			TxPackets: packets, TxRetries: retries, TxPacketsPresent: present, TxRetriesPresent: present,
			TxPacketCounterSource: "tx_ucast_pkts", RetryCounterSource: source, Uptime: uptime, UptimePresent: true,
		}}}}
	}
	first := makeVAP(100, 4, "tx_pkts_retries", 50, true)
	p.applyRetryInterval(first, start)
	if first[0].StaTable[0].Satisfaction != 0 {
		t.Fatal("first cumulative sample must not produce a retry score")
	}
	second := makeVAP(200, 14, "tx_pkts_retries", 60, true)
	p.applyRetryInterval(second, start.Add(10*time.Second))
	if score := second[0].StaTable[0].Satisfaction; score <= 0 || score >= 100 {
		t.Fatalf("expected interval heuristic score, got %d", score)
	}
	retriesOnly := makeVAP(200, 24, "tx_pkts_retries", 70, true)
	p.applyRetryInterval(retriesOnly, start.Add(15*time.Second))
	if retriesOnly[0].StaTable[0].Satisfaction != 0 {
		t.Fatal("retry-only delta without packet progress produced a score")
	}
	reset := makeVAP(2, 1, "tx_pkts_retries", 1, true)
	p.applyRetryInterval(reset, start.Add(20*time.Second))
	if reset[0].StaTable[0].Satisfaction != 0 {
		t.Fatal("counter/association reset must clear retry quality")
	}
	afterReset := makeVAP(20, 3, "tx_pkts_retries", 11, true)
	p.applyRetryInterval(afterReset, start.Add(30*time.Second))
	if afterReset[0].StaTable[0].Satisfaction == 0 {
		t.Fatal("compatible post-reset interval did not score")
	}
	changedSource := makeVAP(30, 4, "tx_data_pkts_retried", 21, true)
	p.applyRetryInterval(changedSource, start.Add(40*time.Second))
	if changedSource[0].StaTable[0].Satisfaction != 0 {
		t.Fatal("retry source change must invalidate interval")
	}
	departed := []telemetry.VAP{{Name: "ath1", InterfaceName: "wl1.1", BSSID: "02:00:00:00:00:01"}}
	p.applyRetryInterval(departed, start.Add(50*time.Second))
	if len(p.retryBaselines) != 0 {
		t.Fatalf("departed station baseline was retained: %+v", p.retryBaselines)
	}
	wrapProvider := NewAsusProvider(nil)
	nearWrap := makeVAP(int64(^uint64(0)>>1)-5, int64(^uint64(0)>>1)-3, "tx_pkts_retries", 40, true)
	wrapProvider.applyRetryInterval(nearWrap, start)
	wrapped := makeVAP(4, 2, "tx_pkts_retries", 50, true)
	wrapProvider.applyRetryInterval(wrapped, start.Add(10*time.Second))
	if wrapped[0].StaTable[0].Satisfaction != 0 {
		t.Fatal("unknown-width counter wrap was interpreted as a valid interval")
	}
}

func TestConfirmedUplinkLossClearsParentAndFDBAttachments(t *testing.T) {
	var lldpCalls atomic.Int32
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "nvram" {
			joined := strings.Join(args, " ")
			switch joined {
			case "get lan_hwaddr":
				return "00:11:22:33:44:55", nil
			case "get lan_gateway":
				return "10.0.0.1", nil
			case "get lan_ipaddr":
				return "10.0.0.2", nil
			}
		}
		if name == "lldpcli" {
			lldpCalls.Add(1)
			return "", nil
		}
		if name == "brctl" {
			return "", nil
		}
		return "", nil
	}
	readFile := func(path string) ([]byte, error) {
		switch {
		case strings.HasSuffix(path, "/carrier"):
			return []byte("0"), nil
		case strings.HasSuffix(path, "/speed"):
			return []byte("0"), nil
		case strings.HasSuffix(path, "/duplex"):
			return []byte("unknown"), nil
		case path == "/proc/net/dev":
			return []byte(""), nil
		case path == "/proc/net/arp":
			return []byte("IP address HW type Flags HW address Mask Device\n"), nil
		default:
			return nil, errors.New("missing fixture")
		}
	}
	p := NewAsusProvider(runner)
	p.readFile = readFile
	p.readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("no bridge fixtures") }
	p.parentCache = &verifiedParent{localInterface: "eth0", chassisMAC: "6c:63:f8:30:9a:05", portID: "eth2", remotePort: 3, verifiedAt: time.Now()}
	ports, _, uplink, err := p.GetPortTableContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.parentCache != nil || lldpCalls.Load() != 0 {
		t.Fatalf("confirmed link loss retained or re-read parent: cache=%+v calls=%d", p.parentCache, lldpCalls.Load())
	}
	if !uplink.LinkKnown || uplink.Up || uplink.UplinkMAC != "" || uplink.UplinkRemotePort != 0 {
		t.Fatalf("stale uplink survived confirmed link loss: %+v", uplink)
	}
	if len(ports[0].MacTable) != 0 || ports[0].LastConnection != nil {
		t.Fatalf("uplink attachments survived confirmed link loss: %+v", ports[0])
	}
}

func TestGatewayIdentityRequiresLLDPOrExplicitConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		mac  string
	}{
		{"globally administered ARP neighbor", "00:11:22:33:44:55"},
		{"locally administered ARP neighbor", "6e:63:f8:30:9a:05"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := func(ctx context.Context, name string, args ...string) (string, error) {
				if name == "nvram" {
					switch strings.Join(args, " ") {
					case "get lan_hwaddr":
						return "00:11:22:33:44:66", nil
					case "get lan_gateway":
						return "10.0.0.1", nil
					case "get lan_ipaddr":
						return "10.0.0.2", nil
					}
				}
				if name == "lldpcli" {
					return `{"lldp":{"interface":{}}}`, nil
				}
				return "", nil
			}
			p := NewAsusProvider(runner)
			p.readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("fixture bridge inventory unavailable") }
			p.readFile = func(path string) ([]byte, error) {
				switch {
				case strings.HasSuffix(path, "/carrier"):
					return []byte("1"), nil
				case strings.HasSuffix(path, "/speed"):
					return []byte("1000"), nil
				case strings.HasSuffix(path, "/duplex"):
					return []byte("full"), nil
				case path == "/proc/net/dev":
					return []byte(""), nil
				case path == "/proc/net/arp":
					return []byte("IP address HW type Flags HW address Mask Device\n10.0.0.1 0x1 0x2 " + tc.mac + " * br0\n"), nil
				default:
					return nil, errors.New("not in fixture")
				}
			}
			_, _, uplink, err := p.GetPortTableContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if uplink.UplinkMAC != "" || uplink.UplinkSource != "" {
				t.Fatalf("ARP-only gateway was presented as verified neighbor identity: %+v", uplink)
			}
			if uplink.IP != "10.0.0.2" {
				t.Fatalf("uplink local IP unexpectedly changed: %q", uplink.IP)
			}
		})
	}
}

func TestConfiguredGatewayIPPreservesOnlyValidIPv4(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"10.0.0.1", "10.0.0.1"},
		{"", ""},
		{"not an IP", ""},
		{"0.0.0.0", ""},
		{"127.0.0.1", ""},
		{"224.0.0.1", ""},
		{"255.255.255.255", ""},
		{"2001:db8::1", ""},
	} {
		if got := configuredGatewayIP(tc.in); got != tc.want {
			t.Errorf("configuredGatewayIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPerSampleProcSnapshotIsReadOnce(t *testing.T) {
	var calls atomic.Int32
	p := NewAsusProvider(nil)
	p.readFile = func(string) ([]byte, error) { calls.Add(1); return []byte("eth5: fixture"), nil }
	ctx := context.WithValue(context.Background(), sampleFileCacheKey{}, &sampleFileCache{values: make(map[string][]byte), errors: make(map[string]error)})
	for i := 0; i < 2; i++ {
		data, err := p.readSampleFile(ctx, "/proc/net/dev")
		if err != nil || string(data) != "eth5: fixture" {
			t.Fatalf("unexpected cached proc snapshot: %q %v", data, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("proc snapshot read %d times within one sample", calls.Load())
	}
}

func TestStaticInterfaceCacheExpiryAndFailedRefresh(t *testing.T) {
	now := time.Unix(100, 0)
	var calls atomic.Int32
	fail := false
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		calls.Add(1)
		if fail {
			return "", errors.New("temporary nvram failure")
		}
		return "eth4 eth6", nil
	}
	p := NewAsusProvider(runner)
	p.now = func() time.Time { return now }
	read := func(want string) {
		t.Helper()
		got, err := p.nvramGetContext(context.Background(), "wl_ifnames")
		if err != nil || got != want {
			t.Fatalf("cache read = %q, %v; want %q", got, err, want)
		}
	}
	read("eth4 eth6")
	read("eth4 eth6")
	if calls.Load() != 1 {
		t.Fatalf("cache hit spawned command, calls=%d", calls.Load())
	}

	now = now.Add(stableCacheTTL) // exact expiry is a miss
	fail = true
	if _, err := p.nvramGetContext(context.Background(), "wl_ifnames"); err == nil {
		t.Fatal("expected expired refresh failure")
	}
	if calls.Load() != 2 {
		t.Fatalf("expired entry was not refreshed, calls=%d", calls.Load())
	}

	now = now.Add(time.Second)
	fail = false
	read("eth4 eth6")
	if calls.Load() != 3 {
		t.Fatalf("failed refresh extended cache expiry, calls=%d", calls.Load())
	}
}

func TestCollectionDeadlineStopsLaterInterfaceCommands(t *testing.T) {
	var calls atomic.Int32
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		calls.Add(1)
		if name == "nvram" {
			return "eth4 eth6", nil
		}
		<-ctx.Done()
		return "", ctx.Err()
	}
	p := NewAsusProvider(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := p.Collect(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("collection error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("collection launched later commands after deadline: calls=%d, want nvram + first radio only", got)
	}
}

func TestSampleCommandCacheStoresEmptyAndFailedReads(t *testing.T) {
	var calls atomic.Int32
	runner := func(context.Context, string, ...string) (string, error) {
		calls.Add(1)
		return "", errors.New("unsupported")
	}
	p := NewAsusProvider(runner)
	ctx := context.WithValue(context.Background(), sampleCommandCacheKey{}, &sampleCommandCache{results: make(map[string]commandResult)})
	for i := 0; i < 2; i++ {
		if _, err := p.runCommand(ctx, "wl", "-i", "eth4", "chanim_stats"); err == nil {
			t.Fatal("expected cached command error")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate command ran %d times", calls.Load())
	}
	emptyCalls := atomic.Int32{}
	pEmpty := NewAsusProvider(func(context.Context, string, ...string) (string, error) { emptyCalls.Add(1); return "", nil })
	ctxEmpty := context.WithValue(context.Background(), sampleCommandCacheKey{}, &sampleCommandCache{results: make(map[string]commandResult)})
	_, _ = pEmpty.runCommand(ctxEmpty, "nvram", "get", "wl0_vifnames")
	_, _ = pEmpty.runCommand(ctxEmpty, "nvram", "get", "wl0_vifnames")
	if emptyCalls.Load() != 1 {
		t.Fatalf("valid empty command result was not cached: calls=%d", emptyCalls.Load())
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
	malformed := raw + "\n eth4: 1 2 not-a-number 0 0 0 0 0 3 4 0 0 0 0 0 0\n eth5: -1 2 0 0 0 0 0 0 3 4 0 0 0 0 0 0\n eth6: 1 2 0"
	if got := parseNetDev(malformed); len(got) != 2 {
		t.Fatalf("malformed/truncated counters must remain unavailable, got %+v", got)
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

type testDirEntry string

func (e testDirEntry) Name() string               { return string(e) }
func (e testDirEntry) IsDir() bool                { return true }
func (e testDirEntry) Type() os.FileMode          { return os.ModeDir }
func (e testDirEntry) Info() (os.FileInfo, error) { return nil, errors.New("unused") }

func TestBridgeDiscoveryAndEffectiveVLAN(t *testing.T) {
	dirs := map[string][]os.DirEntry{
		"/sys/class/net":               {testDirEntry("br-lan"), testDirEntry("br-guest")},
		"/sys/class/net/br-lan/brif":   {testDirEntry("eth1"), testDirEntry("eth2")},
		"/sys/class/net/br-guest/brif": {testDirEntry("eth3")},
	}
	files := map[string]string{
		"/sys/class/net/br-lan/brif/eth1/port_no":     "0x2",
		"/sys/class/net/br-guest/brif/eth3/port_no":   "0x4",
		"/sys/class/net/br-lan/bridge/vlan_filtering": "1", "/sys/class/net/br-guest/bridge/vlan_filtering": "1",
	}
	files["/sys/class/net/br-lan/brif/eth2/port_no"] = "0x3"
	readDir := func(path string) ([]os.DirEntry, error) {
		if v, ok := dirs[path]; ok {
			return v, nil
		}
		return nil, os.ErrNotExist
	}
	readFile := func(path string) ([]byte, error) {
		if v, ok := files[path]; ok {
			return []byte(v), nil
		}
		return nil, os.ErrNotExist
	}
	var vlanCalls []string
	run := func(_ context.Context, name string, args ...string) (string, error) {
		if name == "bridge" {
			vlanCalls = append(vlanCalls, strings.Join(args, " "))
			return "port vlan ids\neth1 20 PVID Egress Untagged\n", nil
		}
		return "", nil
	}
	bridges := discoverBridgeInventory(context.Background(), readDir, readFile, run)
	if len(bridges) != 2 || bridges["br-lan"].ports[2] != "eth1" || bridges["br-guest"].ports[4] != "eth3" {
		t.Fatalf("failed to discover multiple Linux bridges: %+v", bridges)
	}
	if len(vlanCalls) != 3 {
		t.Fatalf("expected per-member VLAN inspection, got %v", vlanCalls)
	}
	if got := effectiveVLAN(bridges["br-lan"], "eth1"); got != 20 {
		t.Fatalf("single effective VLAN=%d, want 20", got)
	}
	if got := effectiveVLAN(bridges["br-lan"], "eth2"); got != 0 {
		t.Fatalf("unknown VLAN guessed as %d", got)
	}
	bridges["br-lan"] = bridgeInventory{vlanFiltering: true, vlanByIf: map[string][]int{"eth1": {20, 30}}}
	if got := effectiveVLAN(bridges["br-lan"], "eth1"); got != 0 {
		t.Fatalf("trunk VLAN guessed as %d", got)
	}
	ranged := parseBridgeVLANShow("port vlan ids\neth1 20 PVID Egress Untagged\n                  30-40\n")
	if got := effectiveVLAN(bridgeInventory{vlanFiltering: true, vlanByIf: ranged}, "eth1"); got != 0 {
		t.Fatalf("VLAN range was dropped and fabricated single VLAN %d from %v", got, ranged["eth1"])
	}
	invalidRange := parseBridgeVLANShow("port vlan ids\neth1 20\n                  30-bad\n")
	if got := effectiveVLAN(bridgeInventory{vlanFiltering: true, vlanByIf: invalidRange}, "eth1"); got != 0 {
		t.Fatalf("malformed VLAN range became VLAN %d", got)
	}
	// The bridge's port_no values differ from physical UniFi port indexes:
	// bridge port 1 is eth1 (physical Port 2), bridge port 2 is eth2.
	entries := map[int][]telemetry.PortMacEntry{2: {{MAC: "02:00:00:00:00:01"}}, 3: {{MAC: "02:00:00:00:00:02"}}}
	assignPortVLANs(entries, physicalPortMap(map[int]string{1: "eth1", 2: "eth2"}), bridgeInventory{vlanFiltering: true, vlanByIf: map[string][]int{"eth1": {20}, "eth2": {30}}})
	if entries[2][0].Vlan != 20 || entries[3][0].Vlan != 30 {
		t.Fatalf("effective VLAN not assigned: %+v", entries[2])
	}
}

func TestCarrierIsStableWithinOneSample(t *testing.T) {
	reads := 0
	p := NewAsusProvider(nil)
	p.readFile = func(path string) ([]byte, error) {
		if path != "/sys/class/net/eth0/carrier" {
			return nil, os.ErrNotExist
		}
		reads++
		if reads == 1 {
			return []byte("1\n"), nil
		}
		return []byte("0\n"), nil
	}
	ctx := context.WithValue(context.Background(), sampleCarrierCacheKey{}, &sampleCarrierCache{values: make(map[string]struct {
		value int
		known bool
	})})
	first, firstKnown := p.readCarrier(ctx, "eth0")
	second, secondKnown := p.readCarrier(ctx, "eth0")
	if !firstKnown || !secondKnown || first != 1 || second != 1 || reads != 1 {
		t.Fatalf("carrier changed within sample: first=%d/%v second=%d/%v reads=%d", first, firstKnown, second, secondKnown, reads)
	}
}

func TestTriBandParentIdentityAndWidth(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) (string, error) {
		full := name + " " + strings.Join(args, " ")
		switch full {
		case "nvram get wl_ifnames":
			return "eth4 eth5 eth6", nil
		case "wl -i eth4 chanspec":
			return "6", nil
		case "wl -i eth5 chanspec", "wl -i wl1.1 chanspec":
			return "36l", nil
		case "wl -i eth6 chanspec", "wl -i wl2.1 chanspec":
			return "161/80", nil
		case "nvram get wl1_vifnames":
			return "wl1.1", nil
		case "nvram get wl2_vifnames":
			return "wl2.1", nil
		case "wl -i eth5 txchain":
			return "0x3", nil
		case "wl -i eth6 txchain":
			return "15 (0xf)", nil
		case "wl -i eth4 chanim_stats", "wl -i eth5 chanim_stats", "wl -i eth6 chanim_stats":
			return "chanspec tx inbss idle\n0x1006 1 1 80", nil
		case "wl -i wl1.1 ssid", "wl -i wl2.1 ssid":
			return `Current SSID: "test"`, nil
		case "wl -i wl1.1 bssid":
			return "02:00:00:00:01:01", nil
		case "wl -i wl2.1 bssid":
			return "02:00:00:00:02:01", nil
		case "wl -i wl1.1 assoclist", "wl -i wl2.1 assoclist":
			return "assoclist 02:00:00:00:00:11", nil
		case "wl -i wl1.1 sta_info 02:00:00:00:00:11", "wl -i wl2.1 sta_info 02:00:00:00:00:11":
			return "rssi: -60\nrates: 600 Mbps (11ax 40MHz 2ss)", nil
		}
		return "", nil
	}
	p := NewAsusProvider(runner)
	radios, e := p.GetRadios()
	if e != nil {
		t.Fatal(e)
	}
	if len(radios) != 3 || radios[1].HT != "40" || radios[2].HT != "80" || radios[1].Nss != 2 || radios[2].Nss != 4 ||
		radios[0].Is11ac || !radios[1].Is11ac || !radios[2].Is11ac {
		t.Fatalf("bad radio inventory: %+v", radios)
	}
	if got, want := []string{radios[0].Name, radios[1].Name, radios[2].Name}, []string{"wifi-ng", "wifi-na", "wifi-na-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default radio names = %v, want legacy names %v", got, want)
	}
	vaps, e := p.GetVAPs(nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(vaps) != 2 || vaps[0].RadioName != radios[1].Name || vaps[1].RadioName != radios[2].Name || vaps[0].RadioName == vaps[1].RadioName {
		t.Fatalf("parent identity lost: %+v", vaps)
	}
	stats, e := p.GetRadioStats()
	if e != nil {
		t.Fatal(e)
	}
	if len(stats) != 3 || stats[1].Name != radios[1].Name || stats[2].Name != radios[2].Name {
		t.Fatalf("stats identity lost: %+v", stats)
	}
	for i, want := range []int{20, 40, 80} {
		if stats[i].Bandwidth != want || stats[i].Channel != radios[i].Channel {
			t.Fatalf("radio %s live channel/width mismatch: %+v (want %d MHz)", radios[i].Name, stats[i], want)
		}
	}

	compact := NewAsusProviderWithOptions(runner, AsusProviderOptions{CompactRadioNames: true})
	compactRadios, err := compact.GetRadios()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := []string{compactRadios[0].Name, compactRadios[1].Name, compactRadios[2].Name}, []string{"wifi-ng", "wifi-na", "wifi-na1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compact radio names = %v, want %v", got, want)
	}
	compactVAPs, err := compact.GetVAPs(nil)
	if err != nil {
		t.Fatal(err)
	}
	compactStats, err := compact.GetRadioStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(compactVAPs) != 2 || len(compactStats) != 3 {
		t.Fatalf("compact identities missing: vaps=%+v stats=%+v", compactVAPs, compactStats)
	}
	for i, vap := range compactVAPs {
		if vap.RadioName != compactRadios[i+1].Name {
			t.Errorf("VAP %s radio_name = %q, want %q", vap.Name, vap.RadioName, compactRadios[i+1].Name)
		}
	}
	for i, stat := range compactStats {
		if stat.Name != compactRadios[i].Name {
			t.Errorf("radio stat name = %q, want %q", stat.Name, compactRadios[i].Name)
		}
		if parts := strings.Split(stat.Name+"-client_signal_avg", "-"); len(parts) < 2 || len(parts) > 3 {
			t.Errorf("radio metric key %q falls outside observed 2/3-segment contract", strings.Join(parts, "-"))
		}
	}
}

func TestFDBAgeIsLastSeenAndMultipleClientsAreUnambiguous(t *testing.T) {
	raw := "3 02:00:00:00:00:11 no 5.0\n3 02:00:00:00:00:12 no 7.0"
	macs, last := parseBrctlShowmacs(raw, map[int]string{3: "eth3"}, nil)
	if len(macs[4]) != 2 || last[4] != nil {
		t.Fatal("multiple clients presented as one last connection")
	}
	_, last = parseBrctlShowmacs("3 02:00:00:00:00:11 no 5.0", map[int]string{3: "eth3"}, nil)
	if last[4].ConnectedAt != 0 {
		t.Fatal("FDB age treated as connection duration")
	}
	macs, last = parseBrctlShowmacs("3 02:00:00:00:00:11 no 5.0\n3 02:00:00:00:00:11 no 3.0\n3 02:00:00:00:00:12 no -1\n3 02:00:00:00:00:13 no NaN", map[int]string{3: "eth3"}, nil)
	if len(macs[4]) != 1 || macs[4][0].MAC != "02:00:00:00:00:11" || last[4] == nil {
		t.Fatalf("duplicate or invalid FDB entries were handled as clients: macs=%+v last=%+v", macs, last)
	}
}
