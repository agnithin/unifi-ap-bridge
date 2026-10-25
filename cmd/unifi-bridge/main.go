package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
	"github.com/agnithin/unifi-ap-bridge/pkg/models"
	"github.com/agnithin/unifi-ap-bridge/pkg/telemetry"
	"github.com/agnithin/unifi-ap-bridge/providers"
	"github.com/agnithin/unifi-ap-bridge/providers/broadcom"
	"github.com/agnithin/unifi-ap-bridge/providers/profile"
)

var (
	version   = "1.2.2"
	commit    = "none"
	buildDate = "unknown"
)

type Config struct {
	ControllerURL    string            `json:"controller_url"`
	IntervalSec      int               `json:"interval_sec"`
	Model            string            `json:"model"`
	StatePath        string            `json:"state_path"`
	Branch           string            `json:"branch,omitempty"`
	UplinkMAC        string            `json:"uplink_mac,omitempty"`
	UplinkRemotePort int               `json:"uplink_remote_port,omitempty"`
	WlanMapping      map[string]string `json:"wlan_mapping"`
}

func loadConfig(path string) (*Config, error) {
	cfg := &Config{
		ControllerURL: "http://10.100.1.1:8080/inform",
		IntervalSec:   10,
		Model:         "U6M",
		StatePath:     "/jffs/addons/unifi-bridge/state.json",
		Branch:        "main",
		WlanMapping:   make(map[string]string),
	}

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config json: %w", err)
	}
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = 10
	}
	return cfg, nil
}

var (
	recentLogs []string
	logMu      sync.Mutex
)

func logEvent(format string, v ...any) string {
	msg := fmt.Sprintf(format, v...)
	log.Println(msg)
	logMu.Lock()
	defer logMu.Unlock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	recentLogs = append(recentLogs, entry)
	if len(recentLogs) > 20 {
		recentLogs = recentLogs[len(recentLogs)-20:]
	}
	return msg
}

func updateWebUIStatus(host *providers.HostInfo, modelProfile models.ModelProfile, cfg *Config, stateMgr *inform.StateManager, totalStations int, radioStats []telemetry.RadioStat, ports []telemetry.Port, sysStats *telemetry.SystemStats, health *exchangeHealth, exchangeErr error) {
	statusDir := "/tmp/var/wwwext/unifi-bridge"
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		return
	}

	sta2G := 0
	sta5G := 0
	cuTotal2G := 0
	cuTotal5G := 0
	for _, rs := range radioStats {
		if rs.Channel <= 14 {
			sta2G += rs.NumSta
			cuTotal2G = rs.CuTotal
		} else {
			sta5G += rs.NumSta
			cuTotal5G = rs.CuTotal
		}
	}

	lastStatus := fmt.Sprintf("Active (%s)", stateMgr.GetMode())
	if exchangeErr != nil {
		lastStatus = fmt.Sprintf("Error: %v", exchangeErr)
	}

	logMu.Lock()
	logsCopy := make([]string, len(recentLogs))
	copy(logsCopy, recentLogs)
	logMu.Unlock()

	statusObj := map[string]any{
		"running":                true,
		"version":                version,
		"branch":                 cfg.Branch,
		"pid":                    os.Getpid(),
		"hostname":               host.Hostname,
		"mac":                    host.MAC,
		"ip":                     host.IP,
		"model":                  modelProfile.DisplayName,
		"model_code":             modelProfile.ModelCode,
		"firmware":               modelProfile.Firmware,
		"controller_url":         cfg.ControllerURL,
		"interval_sec":           cfg.IntervalSec,
		"adopted":                stateMgr.IsAdopted(),
		"mode":                   string(stateMgr.GetMode()),
		"uptime":                 host.Uptime,
		"last_heartbeat":         health.lastSuccessString(),
		"last_successful_inform": health.lastSuccessString(),
		"last_attempt":           health.LastAttempt.Format("2006-01-02 15:04:05"),
		"status_updated_at":      time.Now().Unix(),
		"commit":                 commit,
		"build_date":             buildDate,
		"last_status":            lastStatus,
		"total_stations":         totalStations,
		"stations_2g":            sta2G,
		"stations_5g":            sta5G,
		"cu_total_2g":            cuTotal2G,
		"cu_total_5g":            cuTotal5G,
		"radio_stats":            radioStats,
		"ports":                  ports,
		"system_stats":           sysStats,
		"recent_logs":            logsCopy,
	}

	raw, err := json.MarshalIndent(statusObj, "", "  ")
	if err != nil {
		return
	}

	jsContent := fmt.Sprintf("window.unifiBridgeStatus = %s;\n", string(raw))
	tmpPath := filepath.Join(statusDir, "status.js.tmp")
	finalPath := filepath.Join(statusDir, "status.js")
	if err := os.WriteFile(tmpPath, []byte(jsContent), 0644); err == nil {
		_ = os.Rename(tmpPath, finalPath)
	}
}

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	controllerFlag := flag.String("controller", "", "UniFi controller inform URL")
	intervalFlag := flag.Int("interval", 0, "Heartbeat interval in seconds")
	modelFlag := flag.String("model", "", "Emulated UniFi AP model (U6IW, U6M, U6P, U6L)")
	stateFlag := flag.String("state", "", "Path to persistent state file")
	onceFlag := flag.Bool("once", false, "Execute single inform exchange and exit")
	profileFlag := flag.String("profile", "", "Path to synthetic profile JSON for simulation/testing")
	wlanMapFlag := flag.String("wlan-map", "", "JSON mapping of SSID to 24-hex WLAN ObjectId")
	dumpFlag := flag.Bool("dump", false, "Dump rendered inform JSON payload to stdout")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.BoolVar(versionFlag, "v", false, "Print version information and exit (shorthand)")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("unifi-bridge %s (commit: %s, built: %s)\n", version, commit, buildDate)
		return
	}

	log.SetOutput(&boundedLogWriter{file: os.Stderr, maxBytes: 1024 * 1024})

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	if *controllerFlag != "" {
		cfg.ControllerURL = *controllerFlag
	}
	if *intervalFlag > 0 {
		cfg.IntervalSec = *intervalFlag
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}
	if *stateFlag != "" {
		cfg.StatePath = *stateFlag
	}
	if *wlanMapFlag != "" {
		var mapping map[string]string
		if err := json.Unmarshal([]byte(*wlanMapFlag), &mapping); err == nil {
			cfg.WlanMapping = mapping
		}
	}

	modelProfile := models.GetProfile(cfg.Model)

	var provider providers.TelemetryProvider
	if *profileFlag != "" {
		p, err := profile.NewProfileProvider(*profileFlag)
		if err != nil {
			log.Fatalf("failed to load profile: %v", err)
		}
		provider = p
	} else {
		provider = broadcom.NewAsusProvider(nil)
	}

	host, err := provider.GetHostInfo()
	if err != nil {
		log.Fatalf("failed to query host info: %v", err)
	}

	stateMgr, err := inform.NewStateManager(cfg.StatePath, host.MAC)
	if err != nil {
		log.Fatalf("failed to initialize state manager: %v", err)
	}

	client := inform.NewInformClient(cfg.ControllerURL, 15*time.Second, "")

	logEvent("[unifi-bridge] Starting AP bridge for %s (%s, IP: %s)", host.Hostname, host.MAC, host.IP)
	logEvent("[unifi-bridge] Emulating %s (firmware: %s), reporting to %s",
		modelProfile.DisplayName, modelProfile.Firmware, cfg.ControllerURL)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ticker := time.NewTicker(time.Duration(cfg.IntervalSec) * time.Second)
	defer ticker.Stop()

	health := &exchangeHealth{}
	runExchange := func() (exchangeErr error) {
		health.LastAttempt = time.Now()
		var totalStations int
		var radioStats []telemetry.RadioStat
		var ports []telemetry.Port
		var sysStats *telemetry.SystemStats

		defer func() {
			updateWebUIStatus(host, modelProfile, cfg, stateMgr, totalStations, radioStats, ports, sysStats, health, exchangeErr)
		}()

		radios, err := provider.GetRadios()
		if err != nil {
			return fmt.Errorf("failed to get radios: %w", err)
		}

		vaps, err := provider.GetVAPs(cfg.WlanMapping)
		if err != nil {
			return fmt.Errorf("failed to get vaps: %w", err)
		}

		// Retrieve radio stats including RF channel utilization and airtime metrics
		radioStats, err = provider.GetRadioStats()
		if err != nil || len(radioStats) == 0 {
			radioStats = make([]telemetry.RadioStat, 0, len(radios))
			for _, r := range radios {
				radioStats = append(radioStats, telemetry.RadioStat{
					Name:    r.Name,
					Channel: r.Channel,
				})
			}
		}

		// Retrieve Ethernet ports, uplink, LLDP and system load telemetry
		pTable, ethTable, uplink, _ := provider.GetPortTable()
		ports = pTable
		lldpEntries, _ := provider.GetLLDPTable()
		systemStats, sysStatsLegacy, _ := provider.GetSystemStats()
		sysStats = systemStats
		if systemStats != nil {
			host.Uptime = systemStats.Uptime
		}

		if uplink != nil {
			if cfg.UplinkMAC != "" {
				uplink.UplinkMAC = cfg.UplinkMAC
			}
			if cfg.UplinkRemotePort > 0 {
				uplink.UplinkRemotePort = cfg.UplinkRemotePort
			}
		}

		// Calculate station count per radio
		for i := range radioStats {
			staCount := 0
			for _, v := range vaps {
				if radioStats[i].Name == v.RadioName || radioStats[i].Name == ("wifi-"+v.Radio) {
					staCount += v.NumSta
				}
			}
			radioStats[i].NumSta = staCount
		}

		for _, v := range vaps {
			totalStations += v.NumSta
		}

		now := time.Now().Unix()
		key := stateMgr.GetKey()
		mode := stateMgr.GetMode()
		adopted := stateMgr.IsAdopted()

		payload := telemetry.InformPayload{
			MAC:             host.MAC,
			IP:              host.IP,
			Hostname:        host.Hostname,
			Model:           modelProfile.ModelCode,
			Version:         modelProfile.Firmware,
			Default:         !adopted,
			State:           map[bool]int{false: 1, true: 4}[adopted],
			Uptime:          host.Uptime,
			Time:            now,
			InformURL:       cfg.ControllerURL,
			XAuthKey:        fmt.Sprintf("%x", key),
			DefaultKey:      !adopted,
			CfgVersion:      stateMgr.GetCfgVersion(),
			RadioTable:      radios,
			RadioTableStats: radioStats,
			VAPTable:        vaps,
			EthernetTable:   ethTable,
			PortTable:       ports,
			SwPortTable:     ports,
			Uplink:          uplink,
			LLDPTable:       lldpEntries,
			SysStats:        sysStatsLegacy,
			SystemStats:     systemStats,
		}

		if uplink != nil && uplink.UplinkMAC != "" {
			payload.UplinkMAC = uplink.UplinkMAC
			payload.LastUplink = &telemetry.LastUplink{
				PortIdx:          uplink.PortIdx,
				UplinkMAC:        uplink.UplinkMAC,
				UplinkRemotePort: uplink.UplinkRemotePort,
				Type:             "wire",
			}
		}

		if *dumpFlag {
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(b))
		}

		packet, err := inform.PackInform(payload, host.MAC, key, inform.PackOptions{
			Mode:          mode,
			PacketVersion: 1,
			Compressed:    true,
		})
		if err != nil {
			return fmt.Errorf("failed to pack inform packet: %w", err)
		}

		respRaw, err := client.Post(ctx, packet)
		resetAdoption := health.observePost(err)
		if err != nil {
			if adopted && resetAdoption {
				logEvent("[unifi-bridge] Controller rejected 3 consecutive informs; resetting adoption state")
				if resetErr := stateMgr.Reset(); resetErr != nil {
					return fmt.Errorf("failed to persist adoption reset: %w", resetErr)
				}
			}
			return err
		}

		respPacket, err := inform.UnpackInform(respRaw, key, host.MAC)
		if err != nil {
			return fmt.Errorf("failed to unpack controller response: %w", err)
		}

		changed, err := stateMgr.ApplyControllerResponse(respPacket.Payload)
		if err != nil {
			return fmt.Errorf("failed to apply controller response: %w", err)
		}
		if changed {
			if err := stateMgr.Save(); err != nil {
				logEvent("[unifi-bridge] Warning: failed to save state: %v", err)
			}
			logEvent("[unifi-bridge] Adoption state updated: adopted=%v, mode=%s",
				stateMgr.IsAdopted(), stateMgr.GetMode())
		}

		health.LastSuccess = time.Now()
		respType, _ := respPacket.Payload["_type"].(string)
		logEvent("[unifi-bridge] Heartbeat ok (%s) | stations: %d | mode: %s",
			respType, totalStations, stateMgr.GetMode())

		return nil
	}

	// First execution immediately
	if err := runExchange(); err != nil {
		logEvent("[unifi-bridge] Initial exchange error: %v", err)
	}
	if *onceFlag {
		return
	}

	for {
		select {
		case <-ctx.Done():
			logEvent("[unifi-bridge] Shutting down daemon gracefully...")
			return
		case <-ticker.C:
			if err := runExchange(); err != nil {
				logEvent("[unifi-bridge] Heartbeat error: %v", err)
			}
		}
	}
}
