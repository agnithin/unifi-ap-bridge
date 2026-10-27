//go:build linux

package capture

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

type packetSocket struct{ fd int }

func openPacketReader(name string) (PacketReader, error) {
	if name != "br0" {
		return nil, errors.New("interface must be br0")
	}
	ifi, err := netInterfaceByName(name)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(0x0003)))
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{Protocol: htons(0x0003), Ifindex: ifi}); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return &packetSocket{fd: fd}, nil
}

func netInterfaceByName(name string) (int, error) {
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return 0, fmt.Errorf("find interface %s: %w", name, err)
	}
	return ifi.Index, nil
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

func (p *packetSocket) ReadPacket(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	timeout := 100 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	tv := syscall.NsecToTimeval(timeout.Nanoseconds())
	if err := syscall.SetsockoptTimeval(p.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		return 0, err
	}
	n, _, err := syscall.Recvfrom(p.fd, buf, syscall.MSG_TRUNC)
	if err == nil {
		return n, nil
	}
	if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EINTR) {
		return 0, errNoPacket
	}
	if errors.Is(err, syscall.EBADF) {
		return 0, context.Canceled
	}
	return 0, err
}
func (p *packetSocket) Close() error { return syscall.Close(p.fd) }
