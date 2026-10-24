package inform

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	HeaderSize    = 40
	MaxPacketSize = 131072 // 128 KB
	MaxJSONSize   = 65536  // 64 KB
	Magic         = "TNBU"

	FlagEncrypted  uint16 = 0x01
	FlagCompressed uint16 = 0x02
	FlagSnappy     uint16 = 0x04
	FlagGCM        uint16 = 0x08
)

var (
	ErrInvalidPacketSize   = errors.New("invalid packet size")
	ErrUnsupportedHeader   = errors.New("unsupported inform header")
	ErrUnsupportedFlags    = errors.New("unsupported encryption or compression flags")
	ErrLengthMismatch      = errors.New("payload length does not match packet")
	ErrIdentityMismatch    = errors.New("response identity does not match expected mac")
	ErrCiphertextFailed    = errors.New("ciphertext validation or decryption failed")
	ErrDecompressionFailed = errors.New("decompression failed or exceeded size limit")
	ErrJSONLimitExceeded   = errors.New("json payload exceeded size limit")
	ErrInvalidKey          = errors.New("inform key must be exactly 16 bytes")
	ErrInvalidIV           = errors.New("inform iv must be exactly 16 bytes")
)

// DefaultBootstrapKey is MD5("ubnt")
var DefaultBootstrapKey = md5.Sum([]byte("ubnt"))

type CipherMode string

const (
	ModeCBC CipherMode = "CBC"
	ModeGCM CipherMode = "GCM"
)

type PackOptions struct {
	Mode          CipherMode
	PacketVersion uint32
	Compressed    bool
	IV            []byte // Optional: random if nil, exactly 16 bytes if provided
}

type Packet struct {
	MAC           string
	PacketVersion uint32
	Mode          CipherMode
	Compressed    bool
	Payload       map[string]any
	RawJSON       []byte
}

func NormalizeMAC(raw string) (string, error) {
	clean := strings.ReplaceAll(strings.ReplaceAll(raw, ":", ""), "-", "")
	if len(clean) != 12 {
		return "", fmt.Errorf("invalid mac address length: %s", raw)
	}
	bytes, err := hex.DecodeString(clean)
	if err != nil {
		return "", fmt.Errorf("invalid mac hex: %w", err)
	}
	formatted := net.HardwareAddr(bytes).String()
	return strings.ToLower(formatted), nil
}

func parseMACBytes(macStr string) ([6]byte, error) {
	var mac [6]byte
	clean := strings.ReplaceAll(strings.ReplaceAll(macStr, ":", ""), "-", "")
	if len(clean) != 12 {
		return mac, fmt.Errorf("invalid mac length: %s", macStr)
	}
	decoded, err := hex.DecodeString(clean)
	if err != nil {
		return mac, err
	}
	copy(mac[:], decoded)
	return mac, nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, ErrCiphertextFailed
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize {
		return nil, ErrCiphertextFailed
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, ErrCiphertextFailed
		}
	}
	return data[:len(data)-padLen], nil
}

func zlibCompress(plain []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(plain); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func zlibDecompress(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, ErrDecompressionFailed
	}
	defer r.Close()

	limited := io.LimitReader(r, MaxJSONSize+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, ErrDecompressionFailed
	}
	if len(out) > MaxJSONSize {
		return nil, ErrDecompressionFailed
	}
	return out, nil
}

// PackInform serializes and encrypts an inform packet.
func PackInform(payload any, mac string, key []byte, opts PackOptions) ([]byte, error) {
	if len(key) != 16 {
		return nil, ErrInvalidKey
	}
	normMAC, err := NormalizeMAC(mac)
	if err != nil {
		return nil, err
	}
	macBytes, err := parseMACBytes(normMAC)
	if err != nil {
		return nil, err
	}

	mode := opts.Mode
	if mode == "" {
		mode = ModeCBC
	}
	if mode != ModeCBC && mode != ModeGCM {
		return nil, ErrUnsupportedFlags
	}

	plainJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(plainJSON) > MaxJSONSize {
		return nil, ErrJSONLimitExceeded
	}

	iv := opts.IV
	if iv == nil {
		iv = make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, iv); err != nil {
			return nil, err
		}
	} else if len(iv) != 16 {
		return nil, ErrInvalidIV
	}

	body := plainJSON
	if opts.Compressed {
		compressed, err := zlibCompress(body)
		if err != nil {
			return nil, err
		}
		body = compressed
	}

	var flags uint16 = FlagEncrypted
	if opts.Compressed {
		flags |= FlagCompressed
	}
	if mode == ModeGCM {
		flags |= FlagGCM
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	var cipherLen uint32
	if mode == ModeCBC {
		padded := pkcs7Pad(body, aes.BlockSize)
		encrypted := make([]byte, len(padded))
		enc := cipher.NewCBCEncrypter(block, iv)
		enc.CryptBlocks(encrypted, padded)
		body = encrypted
		cipherLen = uint32(len(body))
	} else {
		// GCM tag is 16 bytes appended to ciphertext
		cipherLen = uint32(len(body) + 16)
	}

	// 40-byte header
	header := make([]byte, HeaderSize)
	copy(header[0:4], Magic)
	binary.BigEndian.PutUint32(header[4:8], opts.PacketVersion)
	copy(header[8:14], macBytes[:])
	binary.BigEndian.PutUint16(header[14:16], flags)
	copy(header[16:32], iv)
	binary.BigEndian.PutUint32(header[32:36], 1) // payload version
	binary.BigEndian.PutUint32(header[36:40], cipherLen)

	if mode == ModeGCM {
		// UniFi authenticates the 40-byte header with a 16-byte nonce
		aead, err := cipher.NewGCMWithNonceSize(block, 16)
		if err != nil {
			return nil, err
		}
		body = aead.Seal(nil, iv, body, header)
	}

	packet := append(header, body...)
	if len(packet) > MaxPacketSize {
		return nil, ErrInvalidPacketSize
	}
	return packet, nil
}

// UnpackInform decrypts and deserializes an inform packet.
func UnpackInform(data []byte, key []byte, expectedMAC string) (*Packet, error) {
	if len(key) != 16 {
		return nil, ErrInvalidKey
	}
	if len(data) <= HeaderSize || len(data) > MaxPacketSize {
		return nil, ErrInvalidPacketSize
	}

	if string(data[0:4]) != Magic {
		return nil, ErrUnsupportedHeader
	}
	packetVersion := binary.BigEndian.Uint32(data[4:8])
	if packetVersion > 1 {
		return nil, ErrUnsupportedHeader
	}

	macBytes := data[8:14]
	macStr := net.HardwareAddr(macBytes).String()
	normMAC, err := NormalizeMAC(macStr)
	if err != nil {
		return nil, err
	}

	if expectedMAC != "" {
		expNorm, err := NormalizeMAC(expectedMAC)
		if err != nil {
			return nil, err
		}
		if normMAC != expNorm {
			return nil, ErrIdentityMismatch
		}
	}

	flags := binary.BigEndian.Uint16(data[14:16])
	if flags&FlagEncrypted == 0 {
		return nil, ErrUnsupportedFlags
	}
	if flags&^uint16(FlagEncrypted|FlagCompressed|FlagGCM) != 0 {
		return nil, ErrUnsupportedFlags
	}

	iv := data[16:32]
	payloadVersion := binary.BigEndian.Uint32(data[32:36])
	if payloadVersion != 1 {
		return nil, ErrUnsupportedHeader
	}

	payloadLength := binary.BigEndian.Uint32(data[36:40])
	if int(payloadLength) != len(data)-HeaderSize {
		return nil, ErrLengthMismatch
	}

	ciphertext := data[HeaderSize:]
	mode := ModeCBC
	if flags&FlagGCM != 0 {
		mode = ModeGCM
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	var decrypted []byte
	if mode == ModeGCM {
		if len(ciphertext) < 16 {
			return nil, ErrCiphertextFailed
		}
		aead, err := cipher.NewGCMWithNonceSize(block, 16)
		if err != nil {
			return nil, err
		}
		header := data[:HeaderSize]
		dec, err := aead.Open(nil, iv, ciphertext, header)
		if err != nil {
			return nil, ErrCiphertextFailed
		}
		decrypted = dec
	} else {
		if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
			return nil, ErrCiphertextFailed
		}
		buf := make([]byte, len(ciphertext))
		dec := cipher.NewCBCDecrypter(block, iv)
		dec.CryptBlocks(buf, ciphertext)
		unpadded, err := pkcs7Unpad(buf, aes.BlockSize)
		if err != nil {
			return nil, ErrCiphertextFailed
		}
		decrypted = unpadded
	}

	var rawJSON []byte
	if flags&FlagCompressed != 0 {
		decompressed, err := zlibDecompress(decrypted)
		if err != nil {
			return nil, err
		}
		rawJSON = decompressed
	} else {
		rawJSON = decrypted
	}

	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to parse json payload: %w", err)
	}

	return &Packet{
		MAC:           normMAC,
		PacketVersion: packetVersion,
		Mode:          mode,
		Compressed:    flags&FlagCompressed != 0,
		Payload:       payload,
		RawJSON:       rawJSON,
	}, nil
}
