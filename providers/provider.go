package providers

import (
	"context"
	"time"

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
	GetPortTable() ([]telemetry.Port, []telemetry.EthernetPort, *telemetry.Uplink, error)
	GetLLDPTable() ([]telemetry.LLDPEntry, error)
	GetSystemStats() (*telemetry.SystemStats, *telemetry.SysStats, error)
}

// Sample is one bounded provider read. Optional sections are nil when their
// read failed; callers must not turn those failures into fresh zero values.
type Sample struct {
	Radios       []telemetry.Radio
	VAPs         []telemetry.VAP
	RadioStats   []telemetry.RadioStat
	Ports        []telemetry.Port
	Ethernet     []telemetry.EthernetPort
	Uplink       *telemetry.Uplink
	LLDP         []telemetry.LLDPEntry
	System       *telemetry.SystemStats
	LegacySystem *telemetry.SysStats
	StartedAt    time.Time
	CompletedAt  time.Time
	StageErrors  map[string]string
}

// ContextTelemetryProvider exposes a single serial, cancelable collection.
// Implementations must pass ctx to each external operation and stop starting
// work as soon as it is canceled.
type ContextTelemetryProvider interface {
	GetHostInfoContext(context.Context) (*HostInfo, error)
	Collect(context.Context, map[string]string) (*Sample, error)
}
