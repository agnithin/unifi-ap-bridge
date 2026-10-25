package telemetry

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
	TxBytes   int64 `json:"tx_bytes"`
	RxBytes   int64 `json:"rx_bytes"`
	TxPackets int64 `json:"tx_packets"`
	RxPackets int64 `json:"rx_packets"`
	TxRetries int64 `json:"tx_retries,omitempty"`
	TxRate    int   `json:"tx_rate,omitempty"` // in kbps (e.g. 58500)
	RxRate    int   `json:"rx_rate,omitempty"` // in kbps (e.g. 54000)
	Uptime       int64 `json:"uptime,omitempty"`       // seconds connected
	IdleTime     int64 `json:"idletime,omitempty"`     // seconds idle
	Satisfaction int   `json:"satisfaction,omitempty"` // WiFi Experience score (0-100)
}

type VAP struct {
	Name      string    `json:"name"`
	Radio     string    `json:"radio"`
	RadioName string    `json:"radio_name"`
	MAC       string    `json:"mac"`
	BSSID     string    `json:"bssid"`
	ESSID     string    `json:"essid"`
	Channel   int       `json:"channel"`
	State     string    `json:"state"` // "RUN"
	Up        bool      `json:"up"`
	IsGuest   bool      `json:"is_guest"`
	NumSta    int       `json:"num_sta"`
	WlanID    string    `json:"id,omitempty"` // Legacy 24-hex controller ObjectId
	TxBytes   int64     `json:"tx_bytes"`
	RxBytes   int64     `json:"rx_bytes"`
	TxPackets int64     `json:"tx_packets"`
	RxPackets int64     `json:"rx_packets"`
	StaTable  []Station `json:"sta_table"`
}

type Radio struct {
	Name           string `json:"name"`
	Radio          string `json:"radio"` // "ng" or "na"
	Channel        int    `json:"channel"`
	HT             string `json:"ht,omitempty"`
	Nss            int    `json:"nss,omitempty"`
	MaxTxPower     int    `json:"max_txpower,omitempty"`
	MinTxPower     int    `json:"min_txpower,omitempty"`
	BuiltinAntenna bool   `json:"builtin_antenna"`
	Is11ax         bool   `json:"is_11ax"`
	HasHE          bool   `json:"has_he"`
	HasVHT         bool   `json:"has_vht,omitempty"`
	TxPowerMode    string `json:"tx_power_mode,omitempty"`
	CuTotal        int    `json:"cu_total"`   // Total channel utilization % (0-100)
	CuSelfRx       int    `json:"cu_self_rx"` // AP receive airtime % (0-100)
	CuSelfTx       int    `json:"cu_self_tx"` // AP transmit airtime % (0-100)
}

type RadioStat struct {
	Name     string `json:"name"`
	Channel  int    `json:"channel"`
	NumSta   int    `json:"num_sta"`
	CuTotal  int    `json:"cu_total"`   // Total channel utilization % (0-100)
	CuSelfRx int    `json:"cu_self_rx"` // AP receive airtime % (0-100)
	CuSelfTx int    `json:"cu_self_tx"` // AP transmit airtime % (0-100)
}

type Port struct {
	PortIdx    int    `json:"port_idx"`
	Name       string `json:"name"`
	Media      string `json:"media"`
	Up         bool   `json:"up"`
	Speed      int    `json:"speed"` // 10, 100, 1000, 2500
	FullDuplex bool   `json:"full_duplex"`
	Enable     bool   `json:"enable"`
	IsUplink   bool   `json:"is_uplink"`
	RxBytes    int64  `json:"rx_bytes"`
	TxBytes    int64  `json:"tx_bytes"`
	RxPackets  int64  `json:"rx_packets"`
	TxPackets  int64  `json:"tx_packets"`
	RxErrors   int64  `json:"rx_errors"`
	TxErrors   int64  `json:"tx_errors"`
	RxDropped  int64  `json:"rx_dropped"`
	TxDropped  int64  `json:"tx_dropped"`
}

type EthernetPort struct {
	Name       string `json:"name"`
	MAC        string `json:"mac"`
	Speed      int    `json:"speed"`
	FullDuplex bool   `json:"full_duplex"`
	NumPort    int    `json:"num_port"`
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
	NumPort          int    `json:"num_port"`
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
	UplinkMAC        string `json:"uplink_mac,omitempty"`
	UplinkRemotePort int    `json:"uplink_remote_port,omitempty"`
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
	CPU    float64 `json:"cpu"`    // CPU utilization percentage (0.0 - 100.0)
	Mem    float64 `json:"mem"`    // Memory utilization percentage (0.0 - 100.0)
	Uptime int64   `json:"uptime"` // System uptime in seconds
}

type InformPayload struct {
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
	PortTable       []Port         `json:"port_table,omitempty"`
	Uplink          *Uplink        `json:"uplink,omitempty"`
	UplinkMAC       string         `json:"uplink_mac,omitempty"`
	LastUplink      *LastUplink    `json:"last_uplink,omitempty"`
	LLDPTable       []LLDPEntry    `json:"lldp_table,omitempty"`
	SysStats        *SysStats      `json:"sys_stats,omitempty"`
	SystemStats     *SystemStats   `json:"system-stats,omitempty"`
}
