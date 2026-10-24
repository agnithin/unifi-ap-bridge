package profile

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
	"github.com/agnithin/unifi-ap-bridge/providers"
)

type VAPConfig struct {
	Name     string `json:"name"`
	Radio    string `json:"radio"`
	BSSID    string `json:"bssid"`
	SourceAP string `json:"source_ap"`
	SSID     string `json:"ssid"`
	Channel  int    `json:"channel"`
	WlanID   string `json:"wlan_id,omitempty"`
}

type ProfileData struct {
	MAC      string      `json:"mac"`
	IP       string      `json:"ip"`
	Hostname string      `json:"hostname"`
	Model    string      `json:"model"`
	Firmware string      `json:"firmware"`
	Clients  []string    `json:"clients"`
	VAPs     []VAPConfig `json:"vaps"`
}

type ProfileProvider struct {
	data ProfileData
}

func NewProfileProvider(path string) (*ProfileProvider, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile: %w", err)
	}

	var data ProfileData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("failed to parse profile json: %w", err)
	}

	normMAC, err := inform.NormalizeMAC(data.MAC)
	if err != nil {
		return nil, err
	}
	data.MAC = normMAC

	return &ProfileProvider{data: data}, nil
}

func (p *ProfileProvider) Name() string {
	return "profile_simulator"
}

func (p *ProfileProvider) GetHostInfo() (*providers.HostInfo, error) {
	return &providers.HostInfo{
		Hostname: p.data.Hostname,
		MAC:      p.data.MAC,
		IP:       p.data.IP,
		Uptime:   3600,
	}, nil
}

func (p *ProfileProvider) GetRadios() ([]telemetry.Radio, error) {
	radios := []telemetry.Radio{
		{Name: "wifi-ng", Radio: "ng", Channel: 6},
		{Name: "wifi-na", Radio: "na", Channel: 161},
	}
	return radios, nil
}

func (p *ProfileProvider) GetVAPs(wlanMapping map[string]string) ([]telemetry.VAP, error) {
	vaps := make([]telemetry.VAP, 0, len(p.data.VAPs))

	for _, vc := range p.data.VAPs {
		band := "ng"
		if vc.Channel > 14 {
			band = "na"
		}

		bssid, _ := inform.NormalizeMAC(vc.BSSID)

		stations := []telemetry.Station{}
		// In canary profile, attach allowlisted client to the 5 GHz VAP
		if vc.Channel == 161 && len(p.data.Clients) > 0 {
			clientMAC, _ := inform.NormalizeMAC(p.data.Clients[0])
			stations = append(stations, telemetry.Station{
				MAC:        clientMAC,
				Radio:      band,
				RadioProto: "ax", // Wi-Fi 6!
				VAPName:    vc.Name,
				BSSID:      bssid,
				ESSID:      vc.SSID,
				Channel:    vc.Channel,
				Signal:     -71,
				RSSI:       19,
				Noise:      -90,
				Authorized: true,
				IsGuest:    false,
				TxBytes:    104857600,
				RxBytes:    52428800,
				TxPackets:  95000,
				RxPackets:  62000,
				TxRetries:  120,
				TxRate:     816660,
				RxRate:     720580,
				Uptime:     3600,
				IdleTime:   0,
			})
		}

		wlanID := vc.WlanID
		if mapped, ok := wlanMapping[vc.SSID]; ok && len(mapped) == 24 {
			wlanID = mapped
		}

		var vapTxBytes, vapRxBytes, vapTxPackets, vapRxPackets int64
		for _, s := range stations {
			vapTxBytes += s.TxBytes
			vapRxBytes += s.RxBytes
			vapTxPackets += s.TxPackets
			vapRxPackets += s.RxPackets
		}

		vaps = append(vaps, telemetry.VAP{
			Name:      vc.Name,
			Radio:     band,
			RadioName: vc.Radio,
			MAC:       bssid,
			BSSID:     bssid,
			ESSID:     vc.SSID,
			Channel:   vc.Channel,
			State:     "RUN",
			Up:        true,
			IsGuest:   false,
			NumSta:    len(stations),
			WlanID:    wlanID,
			TxBytes:   vapTxBytes,
			RxBytes:   vapRxBytes,
			TxPackets: vapTxPackets,
			RxPackets: vapRxPackets,
			StaTable:  stations,
		})
	}

	return vaps, nil
}

func (p *ProfileProvider) GetRadioStats() ([]telemetry.RadioStat, error) {
	return []telemetry.RadioStat{
		{Name: "wifi-ng", Channel: 6, NumSta: 0, CuTotal: 42, CuSelfRx: 3, CuSelfTx: 8},
		{Name: "wifi-na", Channel: 161, NumSta: 1, CuTotal: 18, CuSelfRx: 4, CuSelfTx: 10},
	}, nil
}
