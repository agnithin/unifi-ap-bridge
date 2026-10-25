package telemetry

import (
	"encoding/json"
	"testing"
)

func TestAPInformUsesInterfaceNameAndLiveInterfaceTable(t *testing.T) {
	ports := []Port{
		{PortIdx: 1, IfName: "eth0", Up: true, Speed: 2500, FullDuplex: true, IsUplink: true, RxBytes: 1234},
		{PortIdx: 2, IfName: "eth1", Up: false},
		{PortIdx: 4, IfName: "eth3", Up: true, Speed: 1000, FullDuplex: true, MacTable: []PortMacEntry{{MAC: "10:2c:b1:73:3d:00"}}},
	}
	uplink := &Uplink{Name: "eth0", MAC: "fc:34:97:4f:ac:60", IP: "10.100.1.11"}
	payload := InformPayload{FirmwareCaps: FirmwareCapSwitchPorts, Uplink: uplink.Name, InterfaceTable: BuildInterfaceTable(ports, nil, uplink), PortTable: ports}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["fw_caps"] != float64(16) {
		t.Fatal("missing AP switch-port capability; controller will not merge port stats")
	}
	if wire["uplink"] != "eth0" {
		t.Fatalf("uplink must be an interface name, got %v", wire["uplink"])
	}
	interfaces, ok := wire["if_table"].([]any)
	if !ok || len(interfaces) != 3 {
		t.Fatal("missing AP interface telemetry")
	}
	first := interfaces[0].(map[string]any)
	if first["up"] != true || first["speed"] != float64(2500) || first["rx_bytes"] != float64(1234) || first["mac"] != uplink.MAC || first["num_port"] != float64(1) {
		t.Fatalf("incorrect uplink telemetry: %v", first)
	}
	if interfaces[1].(map[string]any)["up"] != false {
		t.Fatal("down port reported up")
	}
	lan := interfaces[2].(map[string]any)
	if lan["name"] != "eth3" || lan["speed"] != float64(1000) || lan["up"] != true {
		t.Fatalf("missing active LAN interface: %v", lan)
	}
	port := wire["port_table"].([]any)[2].(map[string]any)
	if port["ifname"] != "eth3" || len(port["mac_table"].([]any)) != 1 {
		t.Fatal("lost physical port/client association")
	}
}

func TestSharedSwitchMACUsesOneManagementInterface(t *testing.T) {
	ports := []Port{{PortIdx: 1, IfName: "eth0", Up: true, Speed: 2500}, {PortIdx: 2, IfName: "eth1"}, {PortIdx: 3, IfName: "eth2"}, {PortIdx: 4, IfName: "eth3", Up: true, Speed: 1000}}
	identities := []EthernetPort{{PortIdx: 1, MAC: "fc:34:97:4f:ac:60"}, {PortIdx: 2, MAC: "fc:34:97:4f:ac:60"}, {PortIdx: 3, MAC: "fc:34:97:4f:ac:60"}, {PortIdx: 4, MAC: "fc:34:97:4f:ac:60"}}
	rows := BuildInterfaceTable(ports, identities, &Uplink{Name: "eth0", MAC: identities[0].MAC})
	if len(rows) != 1 || rows[0].Name != "eth0" || rows[0].NumPort != 4 || rows[0].Speed != 2500 {
		t.Fatalf("shared switch identity lost: %+v", rows)
	}
}
