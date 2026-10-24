package providers

import (
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
)

type HostInfo struct {
	Hostname string
	MAC      string
	IP       string
	Uptime   int64
}

// TelemetryProvider abstracts the physical wireless radio subsystem.
type TelemetryProvider interface {
	Name() string
	GetHostInfo() (*HostInfo, error)
	GetRadios() ([]telemetry.Radio, error)
	GetVAPs(wlanMapping map[string]string) ([]telemetry.VAP, error)
	GetRadioStats() ([]telemetry.RadioStat, error)
}
