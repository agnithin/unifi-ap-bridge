//go:build !linux

package capture

import "errors"

func openPacketReader(string) (PacketReader, error) {
	return nil, errors.New("LAN packet capture is supported only on Linux")
}
