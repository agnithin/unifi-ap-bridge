package inform

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type FixtureVector struct {
	Version    uint32 `json:"version"`
	Mode       string `json:"mode"`
	Compressed bool   `json:"compressed"`
	PacketHex  string `json:"packet_hex"`
}

type Fixture struct {
	Provenance string          `json:"provenance"`
	KeyHex     string          `json:"key_hex"`
	IVHex      string          `json:"iv_hex"`
	MAC        string          `json:"mac"`
	Payload    map[string]any  `json:"payload"`
	Vectors    []FixtureVector `json:"vectors"`
}

func loadFixture(t *testing.T) Fixture {
	data, err := os.ReadFile("testdata/wire-vectors.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	var f Fixture
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&f); err != nil {
		t.Fatalf("failed to parse fixture json: %v", err)
	}
	return f
}

func TestWireDecodeKnownAnswerVectors(t *testing.T) {
	fixture := loadFixture(t)
	key, err := hex.DecodeString(fixture.KeyHex)
	if err != nil {
		t.Fatalf("invalid fixture key: %v", err)
	}

	for _, vec := range fixture.Vectors {
		t.Run(vec.Mode+"_v"+string(rune('0'+vec.Version))+"_comp_"+string(rune('0'+map[bool]int{false: 0, true: 1}[vec.Compressed])), func(t *testing.T) {
			packetBytes, err := hex.DecodeString(vec.PacketHex)
			if err != nil {
				t.Fatalf("failed to decode packet hex: %v", err)
			}

			// Unpack independent known-answer packet
			unpacked, err := UnpackInform(packetBytes, key, fixture.MAC)
			if err != nil {
				t.Fatalf("unpack failed: %v", err)
			}

			if unpacked.PacketVersion != vec.Version {
				t.Errorf("version mismatch: got %d, want %d", unpacked.PacketVersion, vec.Version)
			}
			if string(unpacked.Mode) != vec.Mode {
				t.Errorf("mode mismatch: got %s, want %s", unpacked.Mode, vec.Mode)
			}
			if unpacked.Compressed != vec.Compressed {
				t.Errorf("compressed mismatch: got %v, want %v", unpacked.Compressed, vec.Compressed)
			}
			if !reflect.DeepEqual(unpacked.Payload, fixture.Payload) {
				t.Errorf("payload mismatch: got %+v, want %+v", unpacked.Payload, fixture.Payload)
			}
		})
	}
}

func TestWireRoundtripAndExactUncompressedEncoding(t *testing.T) {
	fixture := loadFixture(t)
	key, _ := hex.DecodeString(fixture.KeyHex)
	iv, _ := hex.DecodeString(fixture.IVHex)

	for _, vec := range fixture.Vectors {
		t.Run(vec.Mode+"_v"+string(rune('0'+vec.Version))+"_comp_"+string(rune('0'+map[bool]int{false: 0, true: 1}[vec.Compressed])), func(t *testing.T) {
			// Pack with explicit IV
			packed, err := PackInform(fixture.Payload, fixture.MAC, key, PackOptions{
				Mode:          CipherMode(vec.Mode),
				PacketVersion: vec.Version,
				Compressed:    vec.Compressed,
				IV:            iv,
			})
			if err != nil {
				t.Fatalf("pack failed: %v", err)
			}

			// Uncompressed vectors must match byte-for-byte with the independent fixture
			if !vec.Compressed {
				if hex.EncodeToString(packed) != vec.PacketHex {
					t.Errorf("uncompressed pack hex mismatch:\n got:  %s\n want: %s", hex.EncodeToString(packed), vec.PacketHex)
				}
			}

			// Roundtrip unpack verification
			unpacked, err := UnpackInform(packed, key, fixture.MAC)
			if err != nil {
				t.Fatalf("roundtrip unpack failed: %v", err)
			}

			if !reflect.DeepEqual(unpacked.Payload, fixture.Payload) {
				t.Errorf("roundtrip payload mismatch: got %+v, want %+v", unpacked.Payload, fixture.Payload)
			}
		})
	}
}

func TestWireTamperResistance(t *testing.T) {
	fixture := loadFixture(t)
	key, _ := hex.DecodeString(fixture.KeyHex)

	for _, vec := range fixture.Vectors {
		packetBytes, _ := hex.DecodeString(vec.PacketHex)

		// Truncated packet
		if _, err := UnpackInform(packetBytes[:35], key, ""); err == nil {
			t.Errorf("expected error on truncated packet")
		}

		// Wrong key
		wrongKey := make([]byte, 16)
		if _, err := UnpackInform(packetBytes, wrongKey, ""); err == nil {
			t.Errorf("expected error on wrong key")
		}

		// Corrupted ciphertext
		corrupted := make([]byte, len(packetBytes))
		copy(corrupted, packetBytes)
		corrupted[len(corrupted)-1] ^= 0x01
		if _, err := UnpackInform(corrupted, key, ""); err == nil {
			t.Errorf("expected error on corrupted ciphertext")
		}
	}
}
