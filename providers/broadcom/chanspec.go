package broadcom

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	chanspecRegex = regexp.MustCompile(`^(\d{1,3})(?:/(20|40|80|160))?([lu])?(?:\s|$)`)
)

// ParseChanspec extracts primary channel and bandwidth in MHz.
func ParseChanspec(raw string) (channel int, width int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 20
	}

	// Hex format (e.g. 0xe0a1 or 0x1006)
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		if val, err := strconv.ParseUint(raw[2:], 16, 16); err == nil {
			channel = int(val & 0xFF)
			// Broadcom 11ac/11ax chanspec format:
			// Bits 11-13: bandwidth (0x3800)
			bwBits := (val & 0x3800) >> 11
			switch bwBits {
			case 5: // 160MHz (0x2800)
				width = 160
			case 4: // 80MHz (0x2000)
				width = 80
			case 3: // 40MHz (0x1800)
				width = 40
			case 2: // 20MHz (0x1000)
				width = 20
			default:
				if channel > 14 {
					width = 80
				} else {
					width = 20
				}
			}
			// The low byte is the center channel; control-sideband bits select
			// the primary 20 MHz channel within the bonded block.
			if width > 20 {
				sideband := int((val & 0x0700) >> 8)
				if sideband >= width/20 {
					return 0, width
				}
				channel = channel - (width/10 - 2) + 4*sideband
			}
			return channel, width
		}
	}

	match := chanspecRegex.FindStringSubmatch(raw)
	if len(match) > 1 {
		ch, err := strconv.Atoi(match[1])
		if err == nil {
			channel = ch
		}
	}

	if len(match) > 2 && match[2] != "" {
		bw, err := strconv.Atoi(match[2])
		if err == nil {
			width = bw
		}
	}

	if len(match) > 3 && match[3] != "" && width == 0 {
		width = 40
	}
	if width == 0 {
		width = 20
	}

	return channel, width
}

func BandForChannel(channel int) string {
	if channel <= 14 {
		return "ng"
	}
	return "na"
}
