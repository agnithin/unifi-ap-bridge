package broadcom

import (
	"math"
	"strings"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

const (
	maxCounterVAPs     = 64
	maxCounterStations = 512
	maxCounterGap      = 120 * time.Second
)

type vapStationCounters struct{ success, retries, exhausted, uptime int64 }
type vapCounterBaseline struct {
	at       time.Time
	stations map[string]vapStationCounters
}
type vapCounterTracker struct{ baselines map[string]vapCounterBaseline }

func newVAPCounterTracker() *vapCounterTracker {
	return &vapCounterTracker{baselines: make(map[string]vapCounterBaseline)}
}

// Observe emits interval counters only when every station on a VAP has the
// exact host-requested success/retry/failure sources and stable monotonic
// counters over an unchanged membership window.
func (t *vapCounterTracker) Observe(vaps []telemetry.VAP, at time.Time) {
	if t.baselines == nil {
		t.baselines = make(map[string]vapCounterBaseline)
	}
	for i := range vaps {
		clearVAPIntervalCounters(&vaps[i])
	}
	if at.IsZero() || len(vaps) > maxCounterVAPs {
		t.baselines = make(map[string]vapCounterBaseline)
		return
	}
	counts := make(map[string]int, len(vaps))
	keys := make([]string, len(vaps))
	for i := range vaps {
		keys[i] = vapCounterKey(vaps[i])
		if keys[i] != "" {
			counts[keys[i]]++
		}
	}
	next := make(map[string]vapCounterBaseline, len(vaps))
	for i := range vaps {
		key := keys[i]
		if key == "" || counts[key] != 1 {
			continue
		}
		current, ok := snapshotVAPCounters(vaps[i], at)
		if !ok {
			continue
		}
		next[key] = current
		previous, exists := t.baselines[key]
		if !exists || !previous.at.Before(at) || at.Sub(previous.at) > maxCounterGap || !sameCounterMembers(previous.stations, current.stations) {
			continue
		}
		txSuccess, txFail, txRetries, ok := intervalVAPCounters(previous.stations, current.stations)
		if !ok {
			continue
		}
		txTotal, ok := checkedCounterAdd(txSuccess, txFail)
		if !ok {
			continue
		}
		attempts, ok := checkedCounterAdd(txTotal, txRetries)
		if !ok {
			continue
		}
		vaps[i].TxSuccess = counterPointer(txSuccess)
		vaps[i].TxTotal = counterPointer(txTotal)
		vaps[i].TxRetries = counterPointer(txRetries)
		vaps[i].WiFiTxAttempts = counterPointer(attempts)
		vaps[i].WiFiTxDropped = counterPointer(txFail)
	}
	t.baselines = next
}

func clearVAPIntervalCounters(v *telemetry.VAP) {
	v.TxSuccess, v.TxTotal, v.TxRetries = nil, nil, nil
	v.WiFiTxAttempts, v.WiFiTxDropped = nil, nil
}

func vapCounterKey(v telemetry.VAP) string {
	iface := strings.TrimSpace(v.InterfaceName)
	if iface == "" {
		iface = strings.TrimSpace(v.Name)
	}
	bssid := strings.ToLower(strings.TrimSpace(v.BSSID))
	if iface == "" || bssid == "" {
		return ""
	}
	return iface + "|" + bssid
}

func snapshotVAPCounters(v telemetry.VAP, at time.Time) (vapCounterBaseline, bool) {
	if len(v.StaTable) == 0 || len(v.StaTable) > maxCounterStations || v.NumSta != len(v.StaTable) {
		return vapCounterBaseline{}, false
	}
	stations := make(map[string]vapStationCounters, len(v.StaTable))
	for _, s := range v.StaTable {
		mac := strings.ToLower(strings.TrimSpace(s.MAC))
		if mac == "" || s.TxSuccessCounterSource != "tx_total_pkts_sent" || !s.TxSuccessPacketsPresent || s.TxExhaustedCounterSource != "tx_pkts_retry_exhausted" || !s.TxRetryExhaustedPresent || s.RetryCounterSource != "tx_pkts_retries" || !s.TxRetriesPresent || !s.UptimePresent {
			return vapCounterBaseline{}, false
		}
		if s.TxSuccessPackets < 0 || s.TxRetryExhausted < 0 || s.TxRetries < 0 || s.Uptime < 0 {
			return vapCounterBaseline{}, false
		}
		if _, duplicate := stations[mac]; duplicate {
			return vapCounterBaseline{}, false
		}
		stations[mac] = vapStationCounters{success: s.TxSuccessPackets, retries: s.TxRetries, exhausted: s.TxRetryExhausted, uptime: s.Uptime}
	}
	return vapCounterBaseline{at: at, stations: stations}, true
}

func sameCounterMembers(a, b map[string]vapStationCounters) bool {
	if len(a) != len(b) {
		return false
	}
	for mac := range a {
		if _, ok := b[mac]; !ok {
			return false
		}
	}
	return true
}

func intervalVAPCounters(previous, current map[string]vapStationCounters) (success, failed, retries int64, ok bool) {
	for mac, now := range current {
		before, found := previous[mac]
		if !found || now.uptime < before.uptime || now.success < before.success || now.exhausted < before.exhausted || now.retries < before.retries {
			return 0, 0, 0, false
		}
		var valid bool
		success, valid = checkedCounterAdd(success, now.success-before.success)
		if !valid {
			return 0, 0, 0, false
		}
		failed, valid = checkedCounterAdd(failed, now.exhausted-before.exhausted)
		if !valid {
			return 0, 0, 0, false
		}
		retries, valid = checkedCounterAdd(retries, now.retries-before.retries)
		if !valid {
			return 0, 0, 0, false
		}
	}
	return success, failed, retries, true
}

func checkedCounterAdd(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || b > math.MaxInt64-a {
		return 0, false
	}
	return a + b, true
}

func counterPointer(v int64) *int64 { return &v }
