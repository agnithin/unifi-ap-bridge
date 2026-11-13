package telemetry

import (
	"bufio"
	"context"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SyslogEventKind string

const (
	EventKindAuth         SyslogEventKind = "auth"
	EventKindAssoc        SyslogEventKind = "assoc"
	EventKindReAssoc      SyslogEventKind = "reassoc"
	EventKindDisassoc     SyslogEventKind = "disassoc"
	EventKindDeauth       SyslogEventKind = "deauth"
	EventKindRoamastSteer SyslogEventKind = "roamast_steer"
)

// SyslogWirelessEvent represents an instantaneous wireless transition
// parsed from Broadcom wlceventd or Asuswrt roamast syslog streams.
type SyslogWirelessEvent struct {
	Timestamp  time.Time       `json:"timestamp"`
	Interface  string          `json:"interface"`
	Kind       SyslogEventKind `json:"kind"`
	MAC        string          `json:"mac"`
	Status     string          `json:"status,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	ReasonCode int             `json:"reason_code,omitempty"`
	RSSI       int             `json:"rssi,omitempty"`
	RSSIValid  bool            `json:"rssi_valid,omitempty"`
}

var (
	// wlceventd Regex handles:
	// Nov 12 22:27:38 wlceventd: wlceventd_proc_event(695): eth5: ReAssoc E8:C9:13:7D:11:CC, status: Successful (0), rssi:-74
	// Nov 12 22:27:57 wlceventd: wlceventd_proc_event(645): eth6: Deauth_ind E8:C9:13:7D:11:CC, status: 0, reason: Disassociated due to inactivity (4), rssi:-82
	// Nov 12 22:27:58 wlceventd: wlceventd_proc_event(662): eth6: Disassoc E8:C9:13:7D:11:CC, status: 0, reason: Disassociated because sending station is leaving (or has left) BSS (8), rssi:0
	wlceventdRegex = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d+\s+\d{2}:\d{2}:\d{2})\s+wlceventd:\s+wlceventd_proc_event\(\d+\):\s+([a-zA-Z0-9._]+):\s+([A-Za-z_]+)\s+([0-9a-fA-F:]{17}),\s+status:\s*([^,]+?)(?:,\s*reason:\s*(.*?))?,\s*rssi:(-?\d+)`)

	// roamast Disconnect Regex handles:
	// Nov 12 09:48:40 roamast: eth6: disconnect weak signal strength station [e8:c9:13:7d:11:cc]
	roamastDisconnectRegex = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d+\s+\d{2}:\d{2}:\d{2})\s+roamast:\s+([a-zA-Z0-9._]+):\s+disconnect weak signal strength station\s+\[([0-9a-fA-F:]{17})\]`)
)

// ParseSyslogLine parses a single syslog line for Broadcom wireless transition events.
// It returns nil if the line does not match a known wireless driver event.
func ParseSyslogLine(line string) *SyslogWirelessEvent {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "wlceventd:") && !strings.Contains(line, "roamast:") {
		return nil
	}

	if m := wlceventdRegex.FindStringSubmatch(line); len(m) == 8 {
		rawTime := m[1]
		iface := m[2]
		action := m[3]
		mac := strings.ToLower(m[4])
		status := strings.TrimSpace(m[5])
		reason := strings.TrimSpace(m[6])
		rssiStr := m[7]

		kind := mapActionToKind(action)
		if kind == "" {
			return nil
		}

		rssi, err := strconv.Atoi(rssiStr)
		rssiValid := (err == nil && rssi != 0)

		reasonCode := parseReasonCode(reason)
		eventTime := parseSyslogTimestamp(rawTime)

		return &SyslogWirelessEvent{
			Timestamp:  eventTime,
			Interface:  iface,
			Kind:       kind,
			MAC:        mac,
			Status:     status,
			Reason:     reason,
			ReasonCode: reasonCode,
			RSSI:       rssi,
			RSSIValid:  rssiValid,
		}
	}

	if m := roamastDisconnectRegex.FindStringSubmatch(line); len(m) == 4 {
		rawTime := m[1]
		iface := m[2]
		mac := strings.ToLower(m[3])
		eventTime := parseSyslogTimestamp(rawTime)

		return &SyslogWirelessEvent{
			Timestamp: eventTime,
			Interface: iface,
			Kind:      EventKindRoamastSteer,
			MAC:       mac,
			Reason:    "roamast weak signal disconnect",
		}
	}

	return nil
}

func mapActionToKind(action string) SyslogEventKind {
	switch strings.ToLower(action) {
	case "auth":
		return EventKindAuth
	case "assoc":
		return EventKindAssoc
	case "reassoc":
		return EventKindReAssoc
	case "disassoc":
		return EventKindDisassoc
	case "deauth", "deauth_ind":
		return EventKindDeauth
	default:
		return ""
	}
}

func parseReasonCode(reason string) int {
	if reason == "" {
		return 0
	}
	idx := strings.LastIndex(reason, "(")
	if idx != -1 {
		end := strings.LastIndex(reason, ")")
		if end > idx {
			if code, err := strconv.Atoi(reason[idx+1 : end]); err == nil {
				return code
			}
		}
	}
	return 0
}

func parseSyslogTimestamp(raw string) time.Time {
	now := time.Now()
	// Syslog timestamps format: "Nov 12 22:27:38"
	t, err := time.ParseInLocation("Jan 2 15:04:05", raw, now.Location())
	if err != nil {
		return now
	}
	// Year is omitted in syslog format; apply current year
	return time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
}

// SyslogFollower continuously tails a syslog file in RAM tmpfs, emitting
// real-time wireless transitions via a non-blocking callback.
type SyslogFollower struct {
	path      string
	onEvent   func(SyslogWirelessEvent)
	pollDelay time.Duration
	startAtEnd bool

	mu      sync.Mutex
	running bool
}

// NewSyslogFollower initializes a follower for path (defaults to /tmp/syslog.log).
func NewSyslogFollower(path string, onEvent func(SyslogWirelessEvent)) *SyslogFollower {
	if path == "" {
		path = "/tmp/syslog.log"
	}
	return &SyslogFollower{
		path:        path,
		onEvent:     onEvent,
		pollDelay:   250 * time.Millisecond,
		startAtEnd:  true,
	}
}

// Start begins following the syslog file in a background goroutine.
func (f *SyslogFollower) Start(ctx context.Context) {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return
	}
	f.running = true
	f.mu.Unlock()

	go f.run(ctx)
}

func (f *SyslogFollower) run(ctx context.Context) {
	defer func() {
		f.mu.Lock()
		f.running = false
		f.mu.Unlock()
	}()

	var file *os.File
	var reader *bufio.Reader
	var lastOffset int64
	initial := true

	closeFile := func() {
		if file != nil {
			_ = file.Close()
			file = nil
			reader = nil
			lastOffset = 0
		}
	}
	defer closeFile()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if file == nil {
			var err error
			file, err = os.Open(f.path)
			if err != nil {
				// Syslog file may not exist yet; wait and retry
				select {
				case <-ctx.Done():
					return
				case <-time.After(f.pollDelay * 2):
					continue
				}
			}

			if initial && f.startAtEnd {
				// On fresh startup, seek to EOF so we don't replay old history
				if offset, err := file.Seek(0, io.SeekEnd); err == nil {
					lastOffset = offset
				}
			} else {
				lastOffset = 0
			}
			initial = false
			reader = bufio.NewReader(file)
		}

		line, err := reader.ReadString('\n')
		if err == nil {
			lastOffset += int64(len(line))
			if ev := ParseSyslogLine(line); ev != nil && f.onEvent != nil {
				f.onEvent(*ev)
			}
			continue
		}

		if err == io.EOF {
			// At end of file: check for file rotation or truncation
			fi, statErr := os.Stat(f.path)
			if statErr != nil {
				// File was removed or inaccessible
				closeFile()
			} else if fi.Size() < lastOffset {
				// File was rotated or truncated: reopen from beginning
				closeFile()
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(f.pollDelay):
				continue
			}
		} else {
			// Read error encountered; reopen
			closeFile()
			select {
			case <-ctx.Done():
				return
			case <-time.After(f.pollDelay):
				continue
			}
		}
	}
}
