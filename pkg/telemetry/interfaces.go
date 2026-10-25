package telemetry

import "fmt"

// BuildInterfaceTable emits raw AP inform interfaces. UniFi derives its REST
// ethernet_table and uplink object from if_table and the uplink interface name.
func BuildInterfaceTable(ports []Port, identities []EthernetPort, uplink *Uplink) []EthernetPort {
	byPort := make(map[int]EthernetPort, len(identities))
	for _, entry := range identities {
		byPort[entry.PortIdx] = entry
	}
	out := make([]EthernetPort, 0, len(ports))
	byMAC := make(map[string]int)
	for _, port := range ports {
		entry := byPort[port.PortIdx]
		entry.PortIdx = port.PortIdx
		if port.IfName != "" {
			entry.Name = port.IfName
		}
		if entry.Name == "" {
			entry.Name = fmt.Sprintf("eth%d", port.PortIdx-1)
		}
		entry.NumPort = 1
		entry.Up, entry.Speed, entry.FullDuplex = port.Up, port.Speed, port.FullDuplex
		entry.RxBytes, entry.TxBytes = port.RxBytes, port.TxBytes
		entry.RxPackets, entry.TxPackets = port.RxPackets, port.TxPackets
		entry.RxErrors, entry.TxErrors = port.RxErrors, port.TxErrors
		entry.RxDropped, entry.TxDropped = port.RxDropped, port.TxDropped
		if uplink != nil && entry.Name == uplink.Name {
			entry.IP, entry.Netmask, entry.MAC = uplink.IP, uplink.Netmask, uplink.MAC
		}
		// The XT8 exposes one hardware MAC across its switch interfaces.
		// UniFi keys Ethernet interfaces by MAC; advertise that shared interface
		// once, with its physical port count, rather than losing rows on ingest.
		if entry.MAC != "" {
			if existing, ok := byMAC[entry.MAC]; ok {
				out[existing].NumPort++
				continue
			}
			byMAC[entry.MAC] = len(out)
		}
		out = append(out, entry)
	}
	return out
}
