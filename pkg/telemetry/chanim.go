package telemetry

import (
	"strconv"
	"strings"
)

// ChanimDetails contains optional passive fields reported by Broadcom
// chanim_stats. Nil means the firmware did not provide a valid field.
type ChanimDetails struct {
	OBSSPercent  *int
	NoiseDBm     *int
	NoCatPercent *int
	NoPktPercent *int
}

// ParseChanimDetails reads only named optional fields from a chanim_stats
// header/row. It does not infer unreported airtime or noise values.
func ParseChanimDetails(raw string) ChanimDetails {
	var header map[string]int
	var details ChanimDetails
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "chanspec" {
			header = make(map[string]int, len(fields))
			for i, field := range fields {
				header[field] = i
			}
			continue
		}
		if header == nil || !strings.HasPrefix(fields[0], "0x") {
			continue
		}
		details.OBSSPercent = optionalIntField(fields, header, "obss", 0, 100)
		details.NoiseDBm = optionalIntField(fields, header, "knoise", -150, 0)
		details.NoCatPercent = optionalIntField(fields, header, "nocat", 0, 100)
		details.NoPktPercent = optionalIntField(fields, header, "nopkt", 0, 100)
		return details
	}
	return details
}

func optionalIntField(fields []string, header map[string]int, name string, min, max int) *int {
	index, ok := header[name]
	if !ok || index >= len(fields) {
		return nil
	}
	value, err := strconv.Atoi(fields[index])
	if err != nil || value < min || value > max {
		return nil
	}
	return &value
}
