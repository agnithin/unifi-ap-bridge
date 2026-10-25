package broadcom

import "testing"

func TestParseChanspec(t *testing.T) {
	tests := []struct {
		input       string
		wantChannel int
		wantWidth   int
	}{
		{"6", 6, 20},
		{"6l", 6, 40},
		{"161", 161, 20},
		{"161u", 161, 40},
		{"161/80", 161, 80},
		{"36/40", 36, 40},
		{"36/160", 36, 160},
		{"0xe39b", 161, 80}, // center 155, upper control sideband -> channel 161
		{"0xd826", 36, 40},  // center 38, lower sideband -> channel 36
		{"0xd024", 36, 20},
		{"36l (0xd826)", 36, 40},
		{"6 (0x1006)", 6, 20},
		{"161/80 (0xe39b)", 161, 80},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotCh, gotW := ParseChanspec(tt.input)
			if gotCh != tt.wantChannel {
				t.Errorf("channel: got %d, want %d", gotCh, tt.wantChannel)
			}
			if gotW != tt.wantWidth {
				t.Errorf("width: got %d, want %d", gotW, tt.wantWidth)
			}
		})
	}
}
