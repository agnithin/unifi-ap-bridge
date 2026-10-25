package models

type ModelProfile struct {
	ModelCode   string
	DisplayName string
	Firmware    string
	PortCount   int
	HasSwitch   bool
	Radios      []RadioSpec
}

type RadioSpec struct {
	Name    string // e.g. "wifi-ng", "wifi-na"
	Band    string // "ng" (2.4G) or "na" (5G)
	HasHE   bool   // Wi-Fi 6 (802.11ax)
	HasVHT  bool   // Wi-Fi 5 (802.11ac)
	MaxBW   int    // 80 or 160 MHz
	MaxTxP  int    // dBm
}

var Catalog = map[string]ModelProfile{
	"U6M": {
		ModelCode:   "U6M",
		DisplayName: "UniFi 6 Mesh",
		Firmware:    "6.8.2.15592",
		PortCount:   1,
		HasSwitch:   false,
		Radios: []RadioSpec{
			{Name: "wifi-ng", Band: "ng", HasHE: true, HasVHT: false, MaxBW: 40, MaxTxP: 22},
			{Name: "wifi-na", Band: "na", HasHE: true, HasVHT: true, MaxBW: 160, MaxTxP: 26},
		},
	},
	"U6P": {
		ModelCode:   "U6P",
		DisplayName: "UniFi 6 Pro",
		Firmware:    "6.8.2.15592",
		PortCount:   1,
		HasSwitch:   false,
		Radios: []RadioSpec{
			{Name: "wifi-ng", Band: "ng", HasHE: true, HasVHT: false, MaxBW: 40, MaxTxP: 22},
			{Name: "wifi-na", Band: "na", HasHE: true, HasVHT: true, MaxBW: 160, MaxTxP: 26},
		},
	},
	"U6L": {
		ModelCode:   "U6L",
		DisplayName: "UniFi 6 Lite",
		Firmware:    "6.8.2.15592",
		PortCount:   1,
		HasSwitch:   false,
		Radios: []RadioSpec{
			{Name: "wifi-ng", Band: "ng", HasHE: false, HasVHT: false, MaxBW: 40, MaxTxP: 23},
			{Name: "wifi-na", Band: "na", HasHE: true, HasVHT: true, MaxBW: 80, MaxTxP: 23},
		},
	},
	"U6IW": {
		ModelCode:   "U6IW",
		DisplayName: "UniFi 6 In-Wall",
		Firmware:    "6.8.2.15592",
		PortCount:   4,
		HasSwitch:   true,
		Radios: []RadioSpec{
			{Name: "wifi-ng", Band: "ng", HasHE: true, HasVHT: false, MaxBW: 40, MaxTxP: 22},
			{Name: "wifi-na", Band: "na", HasHE: true, HasVHT: true, MaxBW: 160, MaxTxP: 26},
		},
	},
	"U6-IW": {
		ModelCode:   "U6IW",
		DisplayName: "UniFi 6 In-Wall",
		Firmware:    "6.8.2.15592",
		PortCount:   4,
		HasSwitch:   true,
		Radios: []RadioSpec{
			{Name: "wifi-ng", Band: "ng", HasHE: true, HasVHT: false, MaxBW: 40, MaxTxP: 22},
			{Name: "wifi-na", Band: "na", HasHE: true, HasVHT: true, MaxBW: 160, MaxTxP: 26},
		},
	},
	"U6EntIW": {
		ModelCode:   "U6EntIW",
		DisplayName: "UniFi 6 Enterprise In-Wall",
		Firmware:    "6.8.2.15592",
		PortCount:   4,
		HasSwitch:   true,
		Radios: []RadioSpec{
			{Name: "wifi-ng", Band: "ng", HasHE: true, HasVHT: false, MaxBW: 40, MaxTxP: 22},
			{Name: "wifi-na", Band: "na", HasHE: true, HasVHT: true, MaxBW: 160, MaxTxP: 26},
		},
	},
}

func GetProfile(modelCode string) ModelProfile {
	if prof, ok := Catalog[modelCode]; ok {
		return prof
	}
	return Catalog["U6M"]
}
