package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
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
	version   = "1.2.5"
	commit    = "none"
	buildDate = "unknown"
)

type Config struct {
	Enabled          bool              `json:"enabled"`
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
		Enabled:       true,
		ControllerURL: "",
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

func updateWebUIStatus(host *providers.HostInfo, modelProfile models.ModelProfile, cfg *Config, stateMgr *inform.StateManager, totalStations int, radioStats []telemetry.RadioStat, ports []telemetry.Port, sysStats *telemetry.SystemStats, health *exchangeHealth, exchangeErr error, sampleFresh bool, sampleCompletedAt time.Time, sampleErrors map[string]string) {
	statusDir := "/tmp/var/wwwext/unifi-bridge"

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
			if rs.CuTotal > cuTotal5G {
				cuTotal5G = rs.CuTotal
			}
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
		"enabled":                cfg.Enabled,
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
		"sample_fresh":           sampleFresh,
		"sample_updated_at":      sampleCompletedAt.Unix(),
		"sample_age_sec": func() int64 {
			if sampleCompletedAt.IsZero() {
				return -1
			}
			return int64(time.Since(sampleCompletedAt).Seconds())
		}(),
		"sample_stage_errors": sampleErrors,
		"commit":              commit,
		"build_date":          buildDate,
		"last_status":         lastStatus,
		"total_stations":      totalStations,
		"stations_2g":         sta2G,
		"stations_5g":         sta5G,
		"cu_total_2g":         cuTotal2G,
		"cu_total_5g":         cuTotal5G,
		"radio_stats":         radioStats,
		"ports":               ports,
		"system_stats":        sysStats,
		"recent_logs":         logsCopy,
	}

	raw, err := json.MarshalIndent(statusObj, "", "  ")
	if err != nil {
		reportStatusWriteFailure("marshal status", err)
		return
	}

	jsContent := fmt.Sprintf("window.unifiBridgeStatus = %s;\n", string(raw))
	if err := writeStatusFile(statusDir, []byte(jsContent), osStatusFileOps); err != nil {
		reportStatusWriteFailure("publish status", err)
	}
}

func main() {
	enabledStatusFlag := flag.Bool("is-enabled", false, "Exit 0 when enabled, 3 when disabled; no hardware access")
	applySettingsFlag := flag.Bool("apply-settings", false, "Merge settings from stdin without starting telemetry")
	syncSettingsFlag := flag.Bool("sync-settings", false, "Merge shared settings from stdin while preserving node-local configuration")
	settingsFormat := flag.String("settings-format", "json", "Settings input: json or custom")
	requestIDFlag := flag.String("request-id", "", "Expected WebUI settings request ID")
	gatewayFlag := flag.String("gateway", "", "Gateway IP for automatic controller discovery")
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

	if *applySettingsFlag || *syncSettingsFlag {
		if *configPath == "" {
			log.Fatal("-config is required for settings")
		}
		if *applySettingsFlag && *syncSettingsFlag {
			log.Fatal("-apply-settings and -sync-settings cannot be combined")
		}
		var err error
		if *syncSettingsFlag {
			err = mergeSharedSettings(*configPath, os.Stdin, *gatewayFlag)
		} else {
			err = mergeSettings(*configPath, os.Stdin, *settingsFormat, *gatewayFlag, *requestIDFlag)
		}
		if err != nil {
			log.Fatal(err)
		}
		if *applySettingsFlag {
			if settingsChanged {
				fmt.Println("SETTINGS_CHANGED=1")
			} else {
				fmt.Println("SETTINGS_CHANGED=0")
			}
		}
		return
	}

	log.SetOutput(&boundedLogWriter{file: os.Stderr, maxBytes: 1024 * 1024})

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	if *enabledStatusFlag {
		if !cfg.Enabled {
			os.Exit(3)
		}
		return
	}
	if !cfg.Enabled {
		if err := publishDisabledStatus("/tmp/var/wwwext/unifi-bridge", cfg); err != nil {
			reportStatusWriteFailure("publish disabled status", err)
		}
		log.Print("Bridge disabled in configuration")
		return
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var provider providers.ContextTelemetryProvider
	if *profileFlag != "" {
		p, err := profile.NewProfileProvider(*profileFlag)
		if err != nil {
			log.Fatalf("failed to load profile: %v", err)
		}
		provider = p
	} else {
		provider = broadcom.NewAsusProvider(nil)
	}

	startupCtx, startupCancel := context.WithTimeout(ctx, 8*time.Second)
	host, err := provider.GetHostInfoContext(startupCtx)
	startupCancel()
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

	ticker := time.NewTicker(time.Duration(cfg.IntervalSec) * time.Second)
	defer ticker.Stop()

	health := &exchangeHealth{}
	lastTotalStations := 0
	lastRadioStats := []telemetry.RadioStat(nil)
	lastPorts := []telemetry.Port(nil)
	lastSystemStats := (*telemetry.SystemStats)(nil)
	lastSampleAt := time.Time{}
	lastStageErrors := map[string]string(nil)
	sampleBudget := 8 * time.Second
	if intervalBudget := time.Duration(cfg.IntervalSec) * time.Second; intervalBudget < sampleBudget {
		sampleBudget = intervalBudget
	}
	runExchange := func() (exchangeErr error) {
		health.LastAttempt = time.Now()
		sampleFresh := false

		defer func() {
			updateWebUIStatus(host, modelProfile, cfg, stateMgr, lastTotalStations, lastRadioStats, lastPorts, lastSystemStats, health, exchangeErr, sampleFresh, lastSampleAt, lastStageErrors)
		}()

		sampleCtx, sampleCancel := context.WithTimeout(ctx, sampleBudget)
		sample, err := provider.Collect(sampleCtx, cfg.WlanMapping)
		sampleCancel()
		if err != nil {
			return fmt.Errorf("telemetry sample failed: %w", err)
		}
		sampleFresh = true
		lastSampleAt = sample.CompletedAt
		lastStageErrors = sample.StageErrors
		radios, vaps := sample.Radios, sample.VAPs
		radioStats := sample.RadioStats
		ports, ethTable, uplink := sample.Ports, sample.Ethernet, sample.Uplink
		lldpEntries := sample.LLDP
		systemStats, sysStatsLegacy := sample.System, sample.LegacySystem
		sysStats := systemStats
		if systemStats != nil {
			host.Uptime = systemStats.Uptime
		}

		if uplink != nil {
			applyConfiguredUplinkFallback(uplink, cfg)
		}

		telemetry.AssignRadioStationCounts(radioStats, vaps)
		totalStations := 0
		for _, v := range vaps {
			totalStations += v.NumSta
		}
		if _, failed := sample.StageErrors["radio_stats"]; !failed {
			lastRadioStats = radioStats
		}
		if _, failed := sample.StageErrors["ports"]; !failed {
			lastPorts = ports
		}
		if _, failed := sample.StageErrors["system"]; !failed {
			lastSystemStats = sysStats
		}
		lastTotalStations = totalStations

		now := time.Now().Unix()
		key := stateMgr.GetKey()
		mode := stateMgr.GetMode()
		adopted := stateMgr.IsAdopted()

		uplinkName := ""
		if uplink != nil {
			uplinkName = uplink.Name
		}
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
			InterfaceTable:  telemetry.BuildInterfaceTable(ports, ethTable, uplink),
			PortTable:       ports,
			SwPortTable:     ports,
			Uplink:          uplinkName,
			LLDPTable:       lldpEntries,
			SysStats:        sysStatsLegacy,
			SystemStats:     systemStats,
		}

		if modelProfile.ModelCode == "U6IW" {
			payload.FirmwareCaps = telemetry.FirmwareCapSwitchPorts
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

func applyConfiguredUplinkFallback(uplink *telemetry.Uplink, cfg *Config) {
	if uplink == nil || (uplink.LinkKnown && !uplink.Up) {
		return
	}
	if cfg.UplinkMAC != "" && uplink.UplinkMAC == "" {
		if mac, err := net.ParseMAC(cfg.UplinkMAC); err == nil && len(mac) == 6 && mac[0]&1 == 0 && (mac[0]|mac[1]|mac[2]|mac[3]|mac[4]|mac[5]) != 0 {
			uplink.UplinkMAC = mac.String()
			if uplink.UplinkSource == "" {
				uplink.UplinkSource = "configured_topology"
			}
		}
	}
	verifiedLLDP := uplink.UplinkSource == "lldp_uplink" || uplink.UplinkSource == "lldp_uplink_cached"
	if !verifiedLLDP && uplink.UplinkRemotePort <= 0 && cfg.UplinkRemotePort > 0 {
		uplink.UplinkRemotePort = cfg.UplinkRemotePort
		if uplink.UplinkSource == "" {
			uplink.UplinkSource = "configured_topology"
		}
	}
}
