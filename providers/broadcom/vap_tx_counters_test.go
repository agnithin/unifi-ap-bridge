package broadcom

import (
	"math"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

func counterVAP(stations ...telemetry.Station) telemetry.VAP {
	return telemetry.VAP{Name: "ath1", InterfaceName: "wl1.1", BSSID: "02:00:00:00:00:01", NumSta: len(stations), StaTable: stations}
}
func counterStation(mac string, success, retry, exhausted, uptime int64) telemetry.Station {
	return telemetry.Station{MAC: mac, TxSuccessPackets: success, TxSuccessPacketsPresent: true, TxSuccessCounterSource: "tx_total_pkts_sent", TxRetries: retry, TxRetriesPresent: true, RetryCounterSource: "tx_pkts_retries", TxRetryExhausted: exhausted, TxRetryExhaustedPresent: true, TxExhaustedCounterSource: "tx_pkts_retry_exhausted", Uptime: uptime, UptimePresent: true}
}
func assertNoVAPCounters(t *testing.T, v telemetry.VAP) {
	t.Helper()
	if v.TxSuccess != nil || v.TxTotal != nil || v.TxRetries != nil || v.WiFiTxAttempts != nil || v.WiFiTxDropped != nil {
		t.Fatalf("partial/invalid VAP interval published: %+v", v)
	}
}
func assertVAPCounters(t *testing.T, v telemetry.VAP, success, total, retries, attempts, dropped int64) {
	t.Helper()
	values := []*int64{v.TxSuccess, v.TxTotal, v.TxRetries, v.WiFiTxAttempts, v.WiFiTxDropped}
	want := []int64{success, total, retries, attempts, dropped}
	for i, p := range values {
		if p == nil || *p != want[i] {
			t.Fatalf("VAP counters=%v, want %v", values, want)
		}
	}
}

func TestVAPCounterIntervalUsesStableUnicastDeltasIncludingFailures(t *testing.T) {
	tracker := newVAPCounterTracker()
	at := time.Unix(100, 0)
	first := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 100, 4, 1, 30), counterStation("02:00:00:00:00:12", 20, 1, 0, 12))}
	tracker.Observe(first, at)
	assertNoVAPCounters(t, first[0])
	second := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 200, 16, 2, 40), counterStation("02:00:00:00:00:12", 50, 6, 1, 22))}
	tracker.Observe(second, at.Add(10*time.Second))
	assertVAPCounters(t, second[0], 130, 132, 17, 149, 2)
}

func TestVAPCounterIntervalOmitsFirstResetMembershipUnknownAndLongGap(t *testing.T) {
	base := time.Unix(200, 0)
	t.Run("first sample", func(t *testing.T) {
		tr := newVAPCounterTracker()
		v := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 1, 0, 10))}
		tr.Observe(v, base)
		assertNoVAPCounters(t, v[0])
	})
	t.Run("reset", func(t *testing.T) {
		tr := newVAPCounterTracker()
		a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 10, 2, 1, 10))}
		tr.Observe(a, base)
		b := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 0, 0, 1))}
		tr.Observe(b, base.Add(10*time.Second))
		assertNoVAPCounters(t, b[0])
		c := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 4, 1, 0, 4))}
		tr.Observe(c, base.Add(20*time.Second))
		assertVAPCounters(t, c[0], 3, 3, 1, 4, 0)
	})
	t.Run("membership change", func(t *testing.T) {
		tr := newVAPCounterTracker()
		a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 1, 0, 10))}
		tr.Observe(a, base)
		b := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:12", 3, 2, 0, 20))}
		tr.Observe(b, base.Add(time.Second))
		assertNoVAPCounters(t, b[0])
		c := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:12", 4, 4, 0, 21))}
		tr.Observe(c, base.Add(2*time.Second))
		assertVAPCounters(t, c[0], 1, 1, 2, 3, 0)
	})
	t.Run("unknown source", func(t *testing.T) {
		tr := newVAPCounterTracker()
		a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 1, 0, 10))}
		tr.Observe(a, base)
		unknown := counterStation("02:00:00:00:00:11", 2, 2, 0, 11)
		unknown.TxRetryExhaustedPresent = false
		b := []telemetry.VAP{counterVAP(unknown)}
		tr.Observe(b, base.Add(time.Second))
		assertNoVAPCounters(t, b[0])
		c := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 3, 3, 0, 12))}
		tr.Observe(c, base.Add(2*time.Second))
		assertNoVAPCounters(t, c[0])
	})
	t.Run("long gap", func(t *testing.T) {
		tr := newVAPCounterTracker()
		a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 1, 0, 10))}
		tr.Observe(a, base)
		b := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 2, 2, 0, 11))}
		tr.Observe(b, base.Add(maxCounterGap+time.Second))
		assertNoVAPCounters(t, b[0])
	})
}

func TestVAPCounterIntervalOverflowZeroDeltaAndEmptyVAP(t *testing.T) {
	base := time.Unix(300, 0)
	t.Run("overflow", func(t *testing.T) {
		tr := newVAPCounterTracker()
		a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 0, 0, 0, 1), counterStation("02:00:00:00:00:12", 0, 0, 0, 1))}
		tr.Observe(a, base)
		b := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", math.MaxInt64, 0, 0, 2), counterStation("02:00:00:00:00:12", math.MaxInt64, 0, 0, 2))}
		tr.Observe(b, base.Add(time.Second))
		assertNoVAPCounters(t, b[0])
	})
	t.Run("zero delta", func(t *testing.T) {
		tr := newVAPCounterTracker()
		s := counterStation("02:00:00:00:00:11", 5, 3, 1, 10)
		a := []telemetry.VAP{counterVAP(s)}
		tr.Observe(a, base)
		b := []telemetry.VAP{counterVAP(s)}
		tr.Observe(b, base.Add(time.Second))
		assertVAPCounters(t, b[0], 0, 0, 0, 0, 0)
	})
	t.Run("empty vap clears", func(t *testing.T) {
		tr := newVAPCounterTracker()
		a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 1, 0, 1))}
		tr.Observe(a, base)
		empty := counterVAP()
		tr.Observe([]telemetry.VAP{empty}, base.Add(time.Second))
		assertNoVAPCounters(t, empty)
		next := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 2, 2, 0, 2))}
		tr.Observe(next, base.Add(2*time.Second))
		assertNoVAPCounters(t, next[0])
	})
}

func TestVAPCounterRejectsPartialStationMembership(t *testing.T) {
	tr := newVAPCounterTracker()
	at := time.Unix(400, 0)
	a := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 1, 1, 0, 1))}
	tr.Observe(a, at)
	b := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 2, 2, 0, 2))}
	b[0].NumSta = 2
	tr.Observe(b, at.Add(time.Second))
	assertNoVAPCounters(t, b[0])
	c := []telemetry.VAP{counterVAP(counterStation("02:00:00:00:00:11", 3, 3, 0, 3))}
	tr.Observe(c, at.Add(2*time.Second))
	assertNoVAPCounters(t, c[0])
}
