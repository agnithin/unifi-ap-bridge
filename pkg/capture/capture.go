package capture

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const (
	MaxDuration        = 10 * time.Second
	MaxPackets         = 100
	MaxSnapLen         = 128
	MaxPCAPSize        = 32 * 1024
	ethernetHeaderSize = 14
)

var errNoPacket = errors.New("no packet before receive timeout")

type Request struct {
	Interface   string
	ClientMAC   string
	Duration    time.Duration
	PacketLimit int
	SnapLen     int
	Allow       bool
}

type PacketReader interface {
	// ReadPacket returns the original wire frame length and copies up to len(dst)
	// bytes into dst. Linux uses MSG_TRUNC so the PCAP can preserve orig_len.
	ReadPacket(context.Context, []byte) (int, error)
	Close() error
}
type ReaderOpener func(string) (PacketReader, error)

// Run writes a bounded Ethernet pcap in /tmp. Captures contain only frames
// addressed to or sent by the exact supplied unicast client MAC.
func Run(ctx context.Context, req Request) (string, error) {
	return run(ctx, req, openPacketReader, "/tmp")
}

func validate(req Request) (net.HardwareAddr, error) {
	if !req.Allow {
		return nil, errors.New("packet capture requires --allow-packet-capture")
	}
	if req.Interface != "br0" {
		return nil, errors.New("packet capture is restricted to interface br0")
	}
	mac, err := net.ParseMAC(req.ClientMAC)
	if err != nil || len(mac) != 6 {
		return nil, errors.New("client MAC must be a six-byte Ethernet address")
	}
	if mac[0]&1 != 0 || isZeroMAC(mac) {
		return nil, errors.New("client MAC must be a nonzero unicast address")
	}
	if req.Duration <= 0 || req.Duration > MaxDuration {
		return nil, fmt.Errorf("capture duration must be between 1ns and %s", MaxDuration)
	}
	if req.PacketLimit < 1 || req.PacketLimit > MaxPackets {
		return nil, fmt.Errorf("packet limit must be between 1 and %d", MaxPackets)
	}
	if req.SnapLen < ethernetHeaderSize || req.SnapLen > MaxSnapLen {
		return nil, fmt.Errorf("snap length must be between %d and %d", ethernetHeaderSize, MaxSnapLen)
	}
	if 24+req.PacketLimit*(16+req.SnapLen) > MaxPCAPSize {
		return nil, errors.New("packet capture limits exceed 32 KiB")
	}
	return mac, nil
}

func isZeroMAC(mac net.HardwareAddr) bool {
	for _, b := range mac {
		if b != 0 {
			return false
		}
	}
	return true
}

func run(ctx context.Context, req Request, open ReaderOpener, dir string) (string, error) {
	mac, err := validate(req)
	if err != nil {
		return "", err
	}
	if open == nil {
		return "", errors.New("packet capture reader is unavailable on this platform")
	}
	f, err := os.CreateTemp(dir, "unifi-bridge-packet-*.pcap")
	if err != nil {
		return "", fmt.Errorf("create private packet capture: %w", err)
	}
	path, keep := f.Name(), false
	defer func() {
		_ = f.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return "", fmt.Errorf("protect packet capture: %w", err)
	}
	reader, err := open(req.Interface)
	if err != nil {
		return "", fmt.Errorf("open packet socket on br0 (requires root or CAP_NET_RAW): %w", err)
	}
	defer reader.Close()
	if err := writePCAPHeader(f, uint32(req.SnapLen)); err != nil {
		return "", err
	}

	captureCtx, cancel := context.WithTimeout(ctx, req.Duration)
	defer cancel()
	buf := make([]byte, req.SnapLen)
	written, count := 24, 0
	for count < req.PacketLimit {
		n, readErr := reader.ReadPacket(captureCtx, buf)
		if readErr != nil {
			if errors.Is(readErr, errNoPacket) {
				if captureCtx.Err() == context.DeadlineExceeded {
					break
				}
				if captureCtx.Err() != nil {
					return "", captureCtx.Err()
				}
				continue
			}
			if captureCtx.Err() == context.DeadlineExceeded {
				break
			}
			return "", fmt.Errorf("read packet: %w", readErr)
		}
		if captureCtx.Err() == context.DeadlineExceeded {
			break
		}
		if captureCtx.Err() != nil {
			return "", captureCtx.Err()
		}
		if n < ethernetHeaderSize || n > 65535 {
			return "", errors.New("packet reader returned invalid Ethernet wire length")
		}
		capturedLen := n
		if capturedLen > len(buf) {
			capturedLen = len(buf)
		}
		frame := buf[:capturedLen]
		if !matchesMAC(frame, mac) {
			continue
		}
		recordSize := 16 + capturedLen
		if written+recordSize > MaxPCAPSize {
			break
		}
		if err := writePCAPRecord(f, frame, n); err != nil {
			return "", fmt.Errorf("write packet capture: %w", err)
		}
		written += recordSize
		count++
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("sync packet capture: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close packet capture: %w", err)
	}
	keep = true
	return path, nil
}

func matchesMAC(frame []byte, mac net.HardwareAddr) bool {
	if len(frame) < ethernetHeaderSize {
		return false
	}
	return bytes.Equal(frame[:6], mac) || bytes.Equal(frame[6:12], mac)
}

func writePCAPHeader(w io.Writer, snaplen uint32) error {
	var b [24]byte
	binary.LittleEndian.PutUint32(b[0:4], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 4)
	binary.LittleEndian.PutUint32(b[16:20], snaplen)
	binary.LittleEndian.PutUint32(b[20:24], 1) // DLT_EN10MB
	_, err := w.Write(b[:])
	return err
}

func writePCAPRecord(w io.Writer, frame []byte, originalLen int) error {
	now := time.Now()
	var hdr [16]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(now.Unix()))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(now.Nanosecond()/1000))
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(len(frame)))
	binary.LittleEndian.PutUint32(hdr[12:16], uint32(originalLen))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(frame)
	return err
}
