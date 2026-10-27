package broadcom

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

const maxCCAStatsOutput = 4096
const ccaQueryTimeout = 150 * time.Millisecond
const ccaDeadlineReserve = 250 * time.Millisecond

// parseCCAInterference accepts the bounded summary format emitted by
// `wl cca_get_stats -n -s 1`. It requires one unambiguous row for the current
// channel; the driver's sample age remains unknown.
func parseCCAInterference(raw string, expectedChannel int) (*int, error) {
	if len(raw) == 0 || len(raw) > maxCCAStatsOutput {
		return nil, fmt.Errorf("CCA output size is invalid")
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) > 16 {
		return nil, fmt.Errorf("too many CCA output lines")
	}
	header := []string{"chan", "dur", "ibss", "obss", "interf", "num", "seconds"}
	headerSeen := false
	var row []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "Using" || fields[0] == "Summaries:" {
			continue
		}
		if equalFields(fields, header) {
			if headerSeen {
				return nil, fmt.Errorf("duplicate CCA header")
			}
			headerSeen = true
			continue
		}
		if !headerSeen {
			return nil, fmt.Errorf("unrecognized CCA output")
		}
		if row != nil {
			return nil, fmt.Errorf("multiple CCA rows")
		}
		row = fields
	}
	if !headerSeen || len(row) != 9 {
		return nil, fmt.Errorf("missing or malformed CCA summary row")
	}
	channel, err := strconv.Atoi(row[0])
	if err != nil || channel <= 0 || channel != expectedChannel {
		return nil, fmt.Errorf("CCA channel does not match current radio")
	}
	duration, err := strconv.Atoi(row[1])
	if err != nil || duration <= 0 {
		return nil, fmt.Errorf("invalid CCA duration")
	}
	for _, i := range []int{2, 4, 6} {
		if _, err := parseCCAPercent(row[i]); err != nil {
			return nil, err
		}
	}
	for _, i := range []int{3, 5, 7} {
		if row[i] != "Low" && row[i] != "Medium" && row[i] != "High" {
			return nil, fmt.Errorf("invalid CCA category")
		}
	}
	interference, _ := parseCCAPercent(row[6])
	samples, err := strconv.Atoi(row[8])
	if err != nil || samples != 1 {
		return nil, fmt.Errorf("CCA sample count does not match requested single sample")
	}
	return &interference, nil
}

func parseCCAPercent(raw string) (int, error) {
	if !strings.HasSuffix(raw, "%") {
		return 0, fmt.Errorf("invalid CCA percentage")
	}
	n, err := strconv.Atoi(strings.TrimSuffix(raw, "%"))
	if err != nil || n < 0 || n > 100 {
		return 0, fmt.Errorf("CCA percentage out of range")
	}
	return n, nil
}

func equalFields(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p *AsusProvider) readCCAInterference(ctx context.Context, iface string, channel int) *int {
	if err := ctx.Err(); err != nil {
		return nil
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= ccaQueryTimeout+ccaDeadlineReserve {
		return nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, ccaQueryTimeout)
	defer cancel()
	raw, err := p.wlCmdContext(queryCtx, iface, "cca_get_stats", "-n", "-s", "1")
	if err != nil || ctx.Err() != nil {
		return nil
	}
	value, err := parseCCAInterference(raw, channel)
	if err != nil || queryCtx.Err() != nil {
		return nil
	}
	return value
}

func (p *AsusProvider) enrichCCAInterferenceContext(ctx context.Context, stats []telemetry.RadioStat) {
	if err := ctx.Err(); err != nil || len(stats) == 0 {
		return
	}
	if deadline, ok := ctx.Deadline(); ok {
		maxCost := time.Duration(len(stats))*ccaQueryTimeout + ccaDeadlineReserve
		if time.Until(deadline) <= maxCost {
			return
		}
	}
	for iface, radioName := range p.radioNames {
		if ctx.Err() != nil {
			return
		}
		for i := range stats {
			if stats[i].Name == radioName {
				stats[i].CCAInterference = p.readCCAInterference(ctx, iface, stats[i].Channel)
				break
			}
		}
	}
}
