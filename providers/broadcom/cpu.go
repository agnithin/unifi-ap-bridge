package broadcom

import (
	"math"
	"strconv"
	"strings"
)

type cpuSample struct{ total, idle uint64 }

func parseCPUSample(raw string) cpuSample {
	fields := strings.Fields(strings.SplitN(raw, "\n", 2)[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}
	}
	var sample cpuSample
	// guest/guest_nice are already included in user/nice: count only the first 8.
	for i := 1; i < len(fields) && i <= 8; i++ {
		v, e := strconv.ParseUint(fields[i], 10, 64)
		if e != nil {
			return cpuSample{}
		}
		sample.total += v
		if i == 4 || i == 5 {
			sample.idle += v
		}
	}
	return sample
}
func cpuBusyPercent(old, current cpuSample) float64 {
	if old.total == 0 || current.total <= old.total || current.idle < old.idle {
		return -1
	}
	total := current.total - old.total
	idle := current.idle - old.idle
	if idle > total {
		return -1
	}
	return math.Round(float64(total-idle)/float64(total)*1000) / 10
}
