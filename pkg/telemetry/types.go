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
}

type RadioStat struct {
	Name    string `json:"name"`
	Channel int    `json:"channel"`
	NumSta  int    `json:"num_sta"`
}

type InformPayload struct {
	MAC             string      `json:"mac"`
	IP              string      `json:"ip"`
	Hostname        string      `json:"hostname"`
	Model           string      `json:"model"`
	Version         string      `json:"version"`
	Default         bool        `json:"default"`
	State           int         `json:"state"` // 1 = disconnected/pending, 4 = adopted/connected
	Uptime          int64       `json:"uptime"`
	Time            int64       `json:"time"`
	InformURL       string      `json:"inform_url"`
	XAuthKey        string      `json:"x_authkey"`
	DefaultKey      bool        `json:"_default_key"`
	CfgVersion      string      `json:"cfgversion,omitempty"`
	RadioTable      []Radio     `json:"radio_table"`
	RadioTableStats []RadioStat `json:"radio_table_stats"`
	VAPTable        []VAP       `json:"vap_table"`
}
