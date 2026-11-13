package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSyslogLine_wlceventd_ReAssoc(t *testing.T) {
	line := "Nov 12 22:27:38 wlceventd: wlceventd_proc_event(695): eth5: ReAssoc E8:C9:13:7D:11:CC, status: Successful (0), rssi:-74"
	ev := ParseSyslogLine(line)
	if ev == nil {
		t.Fatalf("expected parsed event, got nil")
	}

	if ev.Kind != EventKindReAssoc {
		t.Errorf("expected EventKindReAssoc, got %s", ev.Kind)
	}
	if ev.Interface != "eth5" {
		t.Errorf("expected eth5, got %s", ev.Interface)
	}
	if ev.MAC != "e8:c9:13:7d:11:cc" {
		t.Errorf("expected lowercase MAC e8:c9:13:7d:11:cc, got %s", ev.MAC)
	}
	if ev.Status != "Successful (0)" {
		t.Errorf("expected status 'Successful (0)', got '%s'", ev.Status)
	}
	if ev.RSSI != -74 || !ev.RSSIValid {
		t.Errorf("expected valid RSSI -74, got %d (valid=%v)", ev.RSSI, ev.RSSIValid)
	}
	if ev.Timestamp.IsZero() {
		t.Errorf("expected non-zero parsed timestamp")
	}
}

func TestParseSyslogLine_wlceventd_Disassoc(t *testing.T) {
	line := "Nov 12 22:27:58 wlceventd: wlceventd_proc_event(662): eth6: Disassoc E8:C9:13:7D:11:CC, status: 0, reason: Disassociated because sending station is leaving (or has left) BSS (8), rssi:0"
	ev := ParseSyslogLine(line)
	if ev == nil {
		t.Fatalf("expected parsed event, got nil")
	}

	if ev.Kind != EventKindDisassoc {
		t.Errorf("expected EventKindDisassoc, got %s", ev.Kind)
	}
	if ev.Interface != "eth6" {
		t.Errorf("expected eth6, got %s", ev.Interface)
	}
	if ev.ReasonCode != 8 {
		t.Errorf("expected reason code 8, got %d", ev.ReasonCode)
	}
	if ev.RSSIValid {
		t.Errorf("expected RSSI 0 to be marked invalid, got valid")
	}
}

func TestParseSyslogLine_wlceventd_DeauthInd(t *testing.T) {
	line := "Nov 12 22:27:57 wlceventd: wlceventd_proc_event(645): eth6: Deauth_ind E8:C9:13:7D:11:CC, status: 0, reason: Disassociated due to inactivity (4), rssi:-82"
	ev := ParseSyslogLine(line)
	if ev == nil {
		t.Fatalf("expected parsed event, got nil")
	}

	if ev.Kind != EventKindDeauth {
		t.Errorf("expected EventKindDeauth, got %s", ev.Kind)
	}
	if ev.ReasonCode != 4 {
		t.Errorf("expected reason code 4, got %d", ev.ReasonCode)
	}
	if ev.RSSI != -82 || !ev.RSSIValid {
		t.Errorf("expected RSSI -82 (valid), got %d (%v)", ev.RSSI, ev.RSSIValid)
	}
}

func TestParseSyslogLine_roamast_Disconnect(t *testing.T) {
	line := "Nov 12 09:48:40 roamast: eth6: disconnect weak signal strength station [e8:c9:13:7d:11:cc]"
	ev := ParseSyslogLine(line)
	if ev == nil {
		t.Fatalf("expected parsed event, got nil")
	}

	if ev.Kind != EventKindRoamastSteer {
		t.Errorf("expected EventKindRoamastSteer, got %s", ev.Kind)
	}
	if ev.Interface != "eth6" {
		t.Errorf("expected eth6, got %s", ev.Interface)
	}
	if ev.MAC != "e8:c9:13:7d:11:cc" {
		t.Errorf("expected lowercase MAC e8:c9:13:7d:11:cc, got %s", ev.MAC)
	}
}

func TestParseSyslogLine_NodeInterface(t *testing.T) {
	line := "Nov 12 22:16:30 wlceventd: wlceventd_proc_event(662): wl1.1: Disassoc 4E:D8:5C:48:33:4D, status: 0, reason: Disassociated because sending station is leaving (or has left) BSS (8), rssi:0"
	ev := ParseSyslogLine(line)
	if ev == nil {
		t.Fatalf("expected parsed event, got nil")
	}

	if ev.Interface != "wl1.1" {
		t.Errorf("expected wl1.1, got %s", ev.Interface)
	}
	if ev.MAC != "4e:d8:5c:48:33:4d" {
		t.Errorf("expected 4e:d8:5c:48:33:4d, got %s", ev.MAC)
	}
}

func TestParseSyslogLine_UnrelatedLines(t *testing.T) {
	unrelated := []string{
		"Nov 12 22:51:11 dropbear[4109]: Child connection from 10.100.1.10:46808",
		"Nov 12 22:52:07 avahi-daemon[2012]: Record not fitting, dropping.",
		"Nov 12 22:25:36 crond[123]: USER nithin pid 4567 cmd /jffs/addons/unifi-bridge/watchdog.sh",
		"",
	}
	for _, l := range unrelated {
		if ev := ParseSyslogLine(l); ev != nil {
			t.Errorf("expected nil for unrelated line '%s', got %+v", l, ev)
		}
	}
}

func TestSyslogFollower_TailingAndEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "syslog.log")

	// Pre-seed with an old line
	if err := os.WriteFile(logPath, []byte("Nov 12 22:00:00 wlceventd: wlceventd_proc_event(685): eth5: Auth 11:22:33:44:55:66, status: Successful (0), rssi:0\n"), 0644); err != nil {
		t.Fatalf("failed to create log: %v", err)
	}

	eventCh := make(chan SyslogWirelessEvent, 10)
	follower := NewSyslogFollower(logPath, func(ev SyslogWirelessEvent) {
		eventCh <- ev
	})
	follower.pollDelay = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	follower.Start(ctx)
	time.Sleep(50 * time.Millisecond) // Let it seek to EOF

	// Append a new fresh event
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open log for append: %v", err)
	}
	_, _ = f.WriteString("Nov 12 22:27:38 wlceventd: wlceventd_proc_event(695): eth5: ReAssoc E8:C9:13:7D:11:CC, status: Successful (0), rssi:-74\n")
	_ = f.Close()

	select {
	case ev := <-eventCh:
		if ev.MAC != "e8:c9:13:7d:11:cc" || ev.Kind != EventKindReAssoc {
			t.Errorf("unexpected event received: %+v", ev)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for syslog event")
	}
}

func TestStationTimeline_RecordInstantEvent(t *testing.T) {
	timeline := NewStationTimeline(10, 50)

	// Baseline snapshot with 1 station
	now := time.Now()
	vaps := []VAP{
		{
			Radio:     "ng",
			RadioName: "wifi-ng",
			BSSID:     "6a:34:97:46:7b:11",
			ESSID:     "blueberry",
			StaTable: []Station{
				{
					MAC:    "4e:d8:5c:48:33:4d",
					Radio:  "ng",
					Signal: -65,
				},
			},
		},
	}
	timeline.Observe(vaps, now, true)

	// Instant disassoc event
	disassocTime := now.Add(2 * time.Second)
	se := timeline.RecordInstantEvent(SyslogWirelessEvent{
		Timestamp: disassocTime,
		Interface: "eth4",
		Kind:      EventKindDisassoc,
		MAC:       "4E:D8:5C:48:33:4D",
	})

	if se == nil {
		t.Fatalf("expected StationEvent from RecordInstantEvent, got nil")
	}
	if se.Type != "observed_disassociation" {
		t.Errorf("expected observed_disassociation, got %s", se.Type)
	}
	if se.MAC != "4e:d8:5c:48:33:4d" {
		t.Errorf("expected lowercase MAC, got %s", se.MAC)
	}
	if se.PollGapSeconds != 2 {
		t.Errorf("expected PollGapSeconds 2, got %d", se.PollGapSeconds)
	}

	// Instant ReAssoc on 5GHz radio (eth6)
	reassocTime := now.Add(3 * time.Second)
	se2 := timeline.RecordInstantEvent(SyslogWirelessEvent{
		Timestamp: reassocTime,
		Interface: "eth6",
		Kind:      EventKindReAssoc,
		MAC:       "4E:D8:5C:48:33:4D",
		RSSI:      -55,
		RSSIValid: true,
	})

	if se2 == nil {
		t.Fatalf("expected StationEvent from ReAssoc, got nil")
	}
	if se2.Type != "observed_association" {
		t.Errorf("expected observed_association, got %s", se2.Type)
	}
	if se2.Signal != -55 || !se2.SignalKnown {
		t.Errorf("expected signal -55, got %d (%v)", se2.Signal, se2.SignalKnown)
	}
	if se2.Band != "5GHz" || se2.Radio != "na" {
		t.Errorf("expected 5GHz / na, got %s / %s", se2.Band, se2.Radio)
	}
}
