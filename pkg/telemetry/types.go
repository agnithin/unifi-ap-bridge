package telemetry

// FirmwareCapSwitchPorts enables the controller's AP port-statistics merge.
const FirmwareCapSwitchPorts = 1 << 4

type Station struct {
	MAC        string `json:"mac"`
	Radio      string `json:"radio"`                 // "ng" or "na"
	RadioProto string `json:"radio_proto,omitempty"` // "ax", "ac", "n", "a", "g", "b"
	VAPName    string `json:"vap_name,omitempty"`
	BSSID      string `json:"bssid"`
	ESSID      string `json:"essid"`
	Channel    int    `json:"channel"`
	Signal     int    `json:"signal,omitempty"` // dBm (e.g. -71)
	RSSI       int    `json:"rssi,omitempty"`   // SNR (e.g. 19)
	Noise      int    `json:"noise,omitempty"`  // dBm (e.g. -90)
	Authorized bool   `json:"authorized"`
	IsGuest    bool   `json:"is_guest"`
	Is11ax     bool   `json:"is_11ax,omitempty"`
	Is11ac     bool   `json:"is_11ac,omitempty"`
	Is11n      bool   `json:"is_11n,omitempty"`
	// Traffic & Airtime Telemetry
	TxBytes                  int64  `json:"tx_bytes"`
	RxBytes                  int64  `json:"rx_bytes"`
	TxPackets                int64  `json:"tx_packets"`
	RxPackets                int64  `json:"rx_packets"`
	TxRetries                int64  `json:"tx_retries,omitempty"`
	TxSuccessPackets         int64  `json:"-"`
	TxRetryExhausted         int64  `json:"-"`
	TxSuccessPacketsPresent  bool   `json:"-"`
	TxSuccessCounterSource   string `json:"-"`
	TxRetryExhaustedPresent  bool   `json:"-"`
	TxExhaustedCounterSource string `json:"-"`
	TxRate                   int    `json:"tx_rate,omitempty"`      // in kbps (e.g. 58500)
	RxRate                   int    `json:"rx_rate,omitempty"`      // in kbps (e.g. 54000)
	Uptime                   int64  `json:"uptime,omitempty"`       // seconds connected
	IdleTime                 int64  `json:"idletime,omitempty"`     // seconds idle
	Satisfaction             int    `json:"satisfaction,omitempty"` // WiFi Experience score (0-100)
	TxPacketsPresent         bool   `json:"-"`
	TxPacketCounterSource    string `json:"-"`
	TxRetriesPresent         bool   `json:"-"`
	RetryCounterSource       string `json:"-"`
	UptimePresent            bool   `json:"-"`
}

type VAP struct {
	AvgClientSignal *int      `json:"avg_client_signal,omitempty"`
	Name            string    `json:"name"`
	InterfaceName   string    `json:"-"`
	Radio           string    `json:"radio"`
	RadioName       string    `json:"radio_name"`
	MAC             string    `json:"mac"`
	BSSID           string    `json:"bssid"`
	ESSID           string    `json:"essid"`
	Channel         int       `json:"channel"`
	State           string    `json:"state"` // "RUN"
	Up              bool      `json:"up"`
	CountersKnown   bool      `json:"counters_known,omitempty"`
	IsGuest         bool      `json:"is_guest"`
	NumSta          int       `json:"num_sta"`
	WlanID          string    `json:"id,omitempty"` // Legacy 24-hex controller ObjectId
	TxBytes         *int64    `json:"tx_bytes,omitempty"`
	RxBytes         *int64    `json:"rx_bytes,omitempty"`
	TxPackets       *int64    `json:"tx_packets,omitempty"`
	RxPackets       *int64    `json:"rx_packets,omitempty"`
	TxSuccess       *int64    `json:"tx_success,omitempty"`
	TxTotal         *int64    `json:"tx_total,omitempty"`
	TxRetries       *int64    `json:"tx_retries,omitempty"`
	WiFiTxAttempts  *int64    `json:"wifi_tx_attempts,omitempty"`
	WiFiTxDropped   *int64    `json:"wifi_tx_dropped,omitempty"`
	StaTable        []Station `json:"sta_table"`
}

// RadioDriverStats is the native radio_table[].athstats wire projection.
// Measured utilization, interference duty cycle, and noise floor.
type RadioDriverStats struct {
	Name       string `json:"name"`
	CuTotal    int    `json:"cu_total"`
	CuSelfRx   int    `json:"cu_self_rx"`
	CuSelfTx   int    `json:"cu_self_tx"`
	CuInterf   *int   `json:"cu_interf,omitempty"`
	NoiseFloor *int   `json:"noise_floor,omitempty"`
}

type Radio struct {
	DriverStats    *RadioDriverStats `json:"athstats,omitempty"`
	Name           string            `json:"name"`
	Radio          string            `json:"radio"` // "ng" or "na"
	Channel        int               `json:"channel"`
	HT             string            `json:"ht,omitempty"`
	Nss            int               `json:"nss,omitempty"`
	MaxTxPower     int               `json:"max_txpower,omitempty"`
	MinTxPower     int               `json:"min_txpower,omitempty"`
	BuiltinAntenna bool              `json:"builtin_antenna"`
	Is11ax         bool              `json:"is_11ax"`
	Is11ac         bool              `json:"is_11ac,omitempty"`
	HasHE          bool              `json:"has_he"`
	HasVHT         bool              `json:"has_vht,omitempty"`
	TxPowerMode    string            `json:"tx_power_mode,omitempty"`
	CuTotal        int               `json:"cu_total"`            // Total channel utilization % (0-100)
	CuSelfRx       int               `json:"cu_self_rx"`          // AP receive airtime % (0-100)
	CuSelfTx       int               `json:"cu_self_tx"`          // AP transmit airtime % (0-100)
	CuInterf       *int              `json:"cu_interf,omitempty"` // Non-WiFi RF interference % (0-100)
}

type RadioStat struct {
	Bandwidth       int    `json:"bw,omitempty"` // Actual operating width in MHz, independent of controller intent.
	Name            string `json:"name"`
	Channel         int    `json:"channel"`
	NumSta          int    `json:"num_sta"`
	CuTotal         int    `json:"cu_total"`                   // Total channel utilization % (0-100)
	CuSelfRx        int    `json:"cu_self_rx"`                 // AP receive airtime % (0-100)
	CuSelfTx        int    `json:"cu_self_tx"`                 // AP transmit airtime % (0-100)
	CuInterf        *int   `json:"cu_interf,omitempty"`        // Non-WiFi RF interference % (0-100)
	CuOBSS          *int   `json:"cu_obss,omitempty"`          // Driver-reported overlapping-BSS airtime, when available.
	CuNoCat         *int   `json:"cu_nocat,omitempty"`         // Driver CCA no-category activity: bad-FCS frames and some control frames, %.
	CuNoPkt         *int   `json:"cu_nopkt,omitempty"`         // Driver CCA invalid-PLCP/CRS noise-like activity, %; not a non-Wi-Fi classification.
	CCAInterference *int   `json:"cca_interference,omitempty"` // Broadcom measured CCA interference.
	NoiseDBm        *int   `json:"noise_dbm,omitempty"`        // Driver-reported channel noise floor, when available.
}

type PortMacEntry struct {
	MAC    string `json:"mac"`
	Age    int    `json:"age,omitempty"`
	Uptime int    `json:"uptime,omitempty"`
	Vlan   int    `json:"vlan,omitempty"`
	Static bool   `json:"static,omitempty"`
}

type PortLastConnection struct {
	Connected   bool   `json:"connected"`
	MAC         string `json:"mac,omitempty"`
	IP          string `json:"ip,omitempty"`
	ConnectedAt int64  `json:"connected_at,omitempty"`
	LastSeen    int64  `json:"last_seen,omitempty"`
}

type Port struct {
	IfName         string              `json:"ifname"`
	PortIdx        int                 `json:"port_idx"`
	Name           string              `json:"name"`
	Media          string              `json:"media"`
	Up             bool                `json:"up"`
	LinkKnown      bool                `json:"link_known,omitempty"`
	CountersKnown  bool                `json:"counters_known,omitempty"`
	Speed          int                 `json:"speed"` // 10, 100, 1000, 2500
	FullDuplex     bool                `json:"full_duplex"`
	Enable         bool                `json:"enable"`
	Enabled        bool                `json:"enabled"`
	IsUplink       bool                `json:"is_uplink"`
	OpMode         string              `json:"op_mode,omitempty"`
	Forward        string              `json:"forward,omitempty"`
	Autoneg        bool                `json:"autoneg,omitempty"`
	PortPoe        bool                `json:"port_poe,omitempty"`
	StpState       string              `json:"stp_state,omitempty"`
	StpPathcost    int                 `json:"stp_pathcost,omitempty"`
	RxBytes        int64               `json:"rx_bytes"`
	TxBytes        int64               `json:"tx_bytes"`
	RxPackets      int64               `json:"rx_packets"`
	TxPackets      int64               `json:"tx_packets"`
	RxErrors       int64               `json:"rx_errors"`
	TxErrors       int64               `json:"tx_errors"`
	RxDropped      int64               `json:"rx_dropped"`
	TxDropped      int64               `json:"tx_dropped"`
	MacTable       []PortMacEntry      `json:"mac_table,omitempty"`
	LastConnection *PortLastConnection `json:"last_connection,omitempty"`
}

type EthernetPort struct {
	Up            bool   `json:"up"`
	LinkKnown     bool   `json:"link_known,omitempty"`
	CountersKnown bool   `json:"counters_known,omitempty"`
	IP            string `json:"ip,omitempty"`
	Netmask       string `json:"netmask,omitempty"`
	RxBytes       int64  `json:"rx_bytes"`
	TxBytes       int64  `json:"tx_bytes"`
	RxPackets     int64  `json:"rx_packets"`
	TxPackets     int64  `json:"tx_packets"`
	RxErrors      int64  `json:"rx_errors"`
	TxErrors      int64  `json:"tx_errors"`
	RxDropped     int64  `json:"rx_dropped"`
	TxDropped     int64  `json:"tx_dropped"`
	Name          string `json:"name"`
	MAC           string `json:"mac"`
	PortIdx       int    `json:"port_idx,omitempty"`
	Speed         int    `json:"speed"`
	FullDuplex    bool   `json:"full_duplex"`
	NumPort       int    `json:"num_port"`
}

type Uplink struct {
	Name             string `json:"name"`
	MAC              string `json:"mac"`
	IP               string `json:"ip,omitempty"`
	Netmask          string `json:"netmask,omitempty"`
	Type             string `json:"type"` // "wire"
	Speed            int    `json:"speed"`
	FullDuplex       bool   `json:"full_duplex"`
	Up               bool   `json:"up"`
	LinkKnown        bool   `json:"link_known,omitempty"`
	NumPort          int    `json:"num_port"`
	PortIdx          int    `json:"port_idx,omitempty"`
	Media            string `json:"media,omitempty"`
	MaxSpeed         int    `json:"max_speed,omitempty"`
	UplinkSource     string `json:"uplink_source,omitempty"`
	UplinkMAC        string `json:"uplink_mac,omitempty"`
	UplinkRemotePort int    `json:"uplink_remote_port,omitempty"`
	RxBytes          int64  `json:"rx_bytes"`
	TxBytes          int64  `json:"tx_bytes"`
	RxPackets        int64  `json:"rx_packets"`
	TxPackets        int64  `json:"tx_packets"`
	RxErrors         int64  `json:"rx_errors"`
	TxErrors         int64  `json:"tx_errors"`
	RxDropped        int64  `json:"rx_dropped"`
}

type LastUplink struct {
	PortIdx          int    `json:"port_idx,omitempty"`
	UplinkMAC        string `json:"uplink_mac,omitempty"`
	UplinkRemotePort int    `json:"uplink_remote_port,omitempty"`
	Type             string `json:"type,omitempty"`
}

type LLDPEntry struct {
	ChassisID     string `json:"chassis_id"`
	IsWired       bool   `json:"is_wired"`
	LocalPortIdx  int    `json:"local_port_idx"`
	LocalPortName string `json:"local_port_name"`
	PortID        string `json:"port_id"`
}

type SysStats struct {
	LoadAvg1  string `json:"loadavg_1,omitempty"`
	LoadAvg5  string `json:"loadavg_5,omitempty"`
	LoadAvg15 string `json:"loadavg_15,omitempty"`
	MemUsed   int64  `json:"mem_used,omitempty"`
	MemTotal  int64  `json:"mem_total,omitempty"`
	MemBuffer int64  `json:"mem_buffer,omitempty"`
}

type SystemStats struct {
	CPU    *float64 `json:"cpu,omitempty"` // CPU utilization percentage (0.0 - 100.0)
	Mem    float64  `json:"mem"`           // Memory utilization percentage (0.0 - 100.0)
	Uptime int64    `json:"uptime"`        // System uptime in seconds
}

type InformPayload struct {
	FirmwareCaps    int            `json:"fw_caps"`
	MAC             string         `json:"mac"`
	IP              string         `json:"ip"`
	Hostname        string         `json:"hostname"`
	Model           string         `json:"model"`
	Version         string         `json:"version"`
	Default         bool           `json:"default"`
	State           int            `json:"state"` // 1 = disconnected/pending, 4 = adopted/connected
	Uptime          int64          `json:"uptime"`
	Time            int64          `json:"time"`
	InformURL       string         `json:"inform_url"`
	XAuthKey        string         `json:"x_authkey"`
	DefaultKey      bool           `json:"_default_key"`
	CfgVersion      string         `json:"cfgversion,omitempty"`
	RadioTable      []Radio        `json:"radio_table"`
	RadioTableStats []RadioStat    `json:"radio_table_stats"`
	VAPTable        []VAP          `json:"vap_table"`
	EthernetTable   []EthernetPort `json:"ethernet_table,omitempty"`
	InterfaceTable  []EthernetPort `json:"if_table,omitempty"`
	PortTable       []Port         `json:"port_table,omitempty"`
	SwPortTable     []Port         `json:"sw_port_table,omitempty"`
	Uplink          string         `json:"uplink,omitempty"`
	UplinkMAC       string         `json:"uplink_mac,omitempty"`
	LastUplink      *LastUplink    `json:"last_uplink,omitempty"`
	LLDPTable       []LLDPEntry    `json:"lldp_table,omitempty"`
	SysStats        *SysStats      `json:"sys_stats,omitempty"`
	SystemStats     *SystemStats   `json:"system-stats,omitempty"`
}
