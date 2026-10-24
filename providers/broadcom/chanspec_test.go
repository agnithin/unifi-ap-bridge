package broadcom

import "testing"

func TestParseChanspec(t *testing.T) {
	tests := []struct {
		input       string
		wantChannel int
		wantWidth   int
	}{
		{"6", 6, 20},
		{"6l", 6, 20},
		{"161", 161, 80},
		{"161u", 161, 80},
		{"161/80", 161, 80},
		{"36/40", 36, 40},
		{"36/160", 36, 160},
		{"0xe0a1", 161, 80}, // 161 (0xa1) with 80MHz bit set (0x10) -> 0x10a1 / 0xe0a1
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
