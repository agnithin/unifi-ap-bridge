# unifi-ap-bridge

A high-performance, ultra-lightweight standalone daemon and WebUI addon for **Asuswrt-Merlin** and Linux routers that bridges Broadcom Wi-Fi and switch telemetry into the **Ubiquiti UniFi Inform Protocol** (`:8080/inform`).

It emulates a genuine UniFi 6 Access Point—featuring native **UniFi 6 In-Wall (`U6-IW`)** switch port emulation as well as **UniFi 6 Mesh (`U6M`)**, **UniFi 6 Pro (`U6P`)**, and **UniFi 6 Lite (`U6L`)**—allowing non-UniFi access points to seamlessly report native wireless client classification, association telemetry, Wi-Fi 6 (`802.11ax`) connection rates, physical switch port link states, and live RSSI into the **UniFi Network Controller**.

---

## Current deployment

Last verified **2026-10-25, approximately 01:45 PDT**: both XT8 APs run **v1.2.4 on `dev`**, code commit `f77fafbc08688faca4b0fd9a286b73790d5a25da`. Both are adopted, running, and sending successful GCM heartbeats. A runtime sample counted 31 stations on Main and 29 on Node; counts fluctuate normally.

The actual UniFi browser UI now shows Main connected to Cloud Gateway Max **Port 1** and Node to **Port 3**, both at **2.5 GbE**. Port Manager shows those uplinks active and **Bridge-EufyHomeBase3 on Node Port 4 at GbE**. This closes the two reported UI issues. Credential rotation, controlled outage/recovery checks, and broader review remain open. The operator approved promoting all current changes to `main` on 2026-10-25. The APs remain on the verified `dev` deployment.

See [audit state and action items](docs/AUDIT_STATE.md) and the [architecture reference](docs/ARCHITECTURE.md).

### v1.2.4 update-check fix

Checking for updates now finishes in the background without the Asus Applying/Complete overlay. Installed and latest versions consistently use a `v` prefix and explicit track labels. Checking `main` does not change the installed `dev` track; an older stable release is labeled **INSTALLED IS NEWER**. Request matching and timeouts prevent stale status from reloading or leaving the check stuck. Both `main` and `dev` checks were verified in the live Asus browser after deployment.

### v1.2.3 UI compatibility fix

The prior bridge sent an uplink object shaped like a REST response and omitted the raw `if_table` required by AP informs. It also advertised no firmware capabilities, so UniFi did not merge live port stats into Port Manager. The fix sends the uplink interface name, reports the shared Ethernet interface correctly, and advertises the implemented switch-port capability. Regression tests cover the wire field types, physical link/counter mapping, shared MAC handling, and capability flag.

## Key Features

* **Zero Dependencies & Completely Standalone:**
  * Compiles to a single static Go binary (~6.9 MB stripped).
  * Requires **no Python**, **no Entware**, **no USB drive**, and **no third-party addons (no scMerlin needed)**.
  * Runs directly on clean Asuswrt-Merlin firmware.
* **Native Wireless Classification:**
  * Clients switch from generic wired switch port attachments to true wireless clients with live signal bars, SNR, and frequency bands (2.4 GHz vs 5 GHz).
  * IoT devices (ESP32, Sonoff, Wyze, smart appliances) correctly classify as **Wi-Fi 4 (`802.11n`)** rather than Wi-Fi 3.
  * Modern mobile and laptop devices classify as **Wi-Fi 6 (`802.11ax`)** or **Wi-Fi 5 (`802.11ac`)**.
* **Physical Ethernet Ports & Switch Port Manager Integration (v1.2.0+):**
  * Emulates multi-port In-Wall Access Points (**`U6-IW`** / UniFi 6 In-Wall) allowing the AP and its downstream physical ports to appear directly in the **UniFi Port Manager** (`/network/default/ports`).
  * Telemetry reports all physical interfaces from `/sys/class/net/ethX` and `/proc/net/dev`:
    * **Port 1 (Uplink)**: 2.5 Gbps WAN/LAN port with link speed, full duplex, and live byte/packet counters.
    * **Ports 2–4 (LAN)**: 1 Gbps Gigabit LAN ports with carrier state, speed, duplex, and real-time wire traffic metrics.
  * **Switch Bridge FDB Client Discovery (v1.2.2+):**
    * Inspects the Linux bridge forwarding database (`brctl showmacs br0` and `/sys/class/net/br0/brif/<iface>/port_no`) and matches learned MACs against `/proc/net/arp`.
    * Emits `mac_table` and `last_connection` per physical port, allowing UniFi Port Manager to associate downstream wired clients (such as smart home bridges or Ethernet PCs) directly to their physical AP LAN port (`sw_mac` and `sw_port` in controller).
* **LLDP Neighbor Discovery & Topology Telemetry (v1.2.2+):**
  * Automatically queries LLDP neighbors via `lldpcli -f json show neighbors` to discover the upstream gateway device MAC (Cloud Gateway Max `6c:63:f8:30:9a:05`), port MACs, and physical parent ports (Port 1 for Main AP, Port 3 for Node AP).
  * Preserves dynamically discovered remote ports so cluster configuration synchronization does not overwrite satellite nodes with Main's uplink port, preventing port collisions on the upstream gateway switch.
  * Populates `uplink_mac`, `last_uplink`, and `lldp_table` from available neighbor data, aligning with gateway `downlink_table` entries.
* **Router CPU & Memory Utilization Telemetry (v1.2.0+):**
  * Reports real-time CPU load percentage and active memory utilization (`system-stats` and legacy `sys_stats`) derived from `/proc/loadavg` and `/proc/meminfo`.
  * Renders live CPU and RAM status cards in the Asuswrt WebUI and CLI.
* **WiFi Experience Scoring (v1.2.0+):**
  * Dynamically computes 0–100% `satisfaction` scores for each associated station based on SNR, RSSI signal attenuation, and packet retry penalties, populating the UniFi "WiFi Experience" column.
* **Live RF Channel Utilization & Airtime Telemetry (v1.1.0+):**
  * Continuously polls Broadcom `wl chanim_stats` on physical 2.4 GHz and 5 GHz radios.
  * Dynamically computes total channel utilization (`cu_total`), AP transmit airtime (`cu_self_tx`), and AP receive airtime (`cu_self_rx`).
  * Streams RF metrics directly into `radio_table` and `radio_table_stats` in the UniFi Inform payload and displays real-time channel load percentages in the Asuswrt WebUI and CLI.
* **Cumulative Client & VAP Traffic Counters (v1.1.0+):**
  * Parses unicast transmission and reception bytes/packets (`tx ucast bytes`, `rx ucast bytes`, `tx ucast pkts`, `rx ucast pkts`), packet retries, uplink/downlink bitrates, association uptime, and idletime from `wl sta_info`.
  * Excludes multicast/broadcast beacon bursts from client download metrics to guarantee accurate accounting.
  * Aggregates counters per-client in `sta_table` and per-SSID in `vap_table`, populating UniFi real-time client throughput and bandwidth utilization graphs.
* **Ultra-Low Resource Footprint:**
  * **RAM:** ~3–5 MB RSS (< 1% of 512 MB router RAM).
  * **CPU:** Samples telemetry at a default 10-second interval. Sampling cost varies with station count and external `wl`/NVRAM/LLDP commands; no sub-millisecond or fixed CPU-use guarantee is established.
  * **Runtime writes:** Caches, logs, and WebUI status live under `/tmp/`. Regular-file daemon logs are capped at 1 MiB. Persistent adoption state is `0600` and saved atomically when controller state changes; flash writes are reduced, not eliminated.
* **Native Asuswrt-Merlin WebUI Integration & Form Isolation (v1.2.2+):**
  * Integrated directly into **Advanced Settings > Wireless** tab bar (e.g. `http://<router-ip>/user3.asp`).
  * Uses Asuswrt-Merlin dynamic user slot allocation (`user1`–`user20.asp`), preventing collisions with YazFi, MerVLAN, or scMerlin.
  * Neutralizes Asuswrt's intrusive `window.refreshpage` reloading behavior; background polling updates metrics dynamically without scrolling jumps or overwriting in-flight form edits.
* **Automated WebUI Updates & Release Tracks (`main` vs `dev`):**
  * Built-in version checker and one-click updater right from the router WebUI.
  * **Release Tracks**: Switch between official stable releases (`main`) and cutting-edge development builds (`dev`).
  * Checks GitHub release tags and raw version metadata. Development builds can share a version number; verify the runtime commit as well as the version after an update.
* **Cluster Mesh Synchronization & Propagation:**
  * Auto-discovers Satellite Nodes directly from Asuswrt NVRAM (`cfg_device_list`) or MerVLAN.
  * Centralized management: configuring settings or clicking "Update Now" on the Main AP WebUI automatically propagates binaries and configurations to all Satellite Nodes over SSH.
  * Preserves per-node cryptographic adoption identities (`state.json`), preventing adoption collisions.
  * Cluster table reports sampled client distribution and runtime version. Healthy status requires a live remote PID and fresh telemetry; a version match is labeled `Version Match (Config Unverified)`.
  * SSH operations are bounded to 30 seconds for status and 120 seconds for transfers. Dropbear-compatible keepalive/idle options and a shell watchdog avoid unsupported `ConnectTimeout` assumptions.
* **Daemon Resilience & Accurate Status:**
  * Separates `last_attempt` from `last_successful_inform`; failed attempts no longer appear as successful heartbeats.
  * Refreshes uptime from each system sample and exposes commit/build date in runtime status.
  * Defaults non-positive configured intervals to 10 seconds; three consecutive HTTP 401/404 rejections are required before adoption recovery. HTTP 400 does not clear identity.
  * Rejects malformed adoption keys and enforces addon directory `0755` / SSH directory `0700`.
* **Built-in CLI Companion (`unifi-bridge`):**
  * Provides quick command-line diagnostics and operations: `unifi-bridge status`, `sync`, `update check`, `update apply`, `reset`, `start`, `stop`, `restart`, `check`, `logs`.
  * Interactive text console menu for easy terminal administration.
* **Automated Health Watchdog:**
  * Automatic cron watchdog via Asuswrt `cru` checking process health every 5 minutes and respawning on failures.
* **Multi-Arch Automated CI/CD:**
  * GitHub Actions automated builds producing static ARMv7 (`armv7l`) and ARM64 (`aarch64`) tarballs and SHA-256 manifests after Go, cluster regression, and shell syntax checks. Normal release-tag and `dev` pushes trigger builds; documentation-only commits can skip CI. The current updater does not enforce the manifest automatically; the deployed package was manually verified.

---

## Architecture

```
unifi-ap-bridge/
├── .github/workflows/
│   └── release.yml          # GitHub Actions multi-arch release & dev CI/CD (ARMv7 & ARM64)
├── bin/                     # Pre-compiled static binaries (ARMv7 & ARM64)
├── cmd/
│   └── unifi-bridge/        # Standalone persistent daemon binary entrypoint (main.go)
├── pkg/
│   ├── inform/              # UniFi Inform wire protocol (binary header, AES-CBC, AES-GCM, Zlib)
│   ├── models/              # Device capabilities catalog (U6IW, U6M, U6P, U6L)
│   └── telemetry/           # Schema definitions (VAPs, Stations, Radios, Stats)
├── providers/               # Pluggable radio telemetry providers
│   ├── provider.go          # Interface: GetRadios(), GetVAPs(), GetRadioStats()
│   ├── broadcom/            # Broadcom Asuswrt provider (nvram, wl, chanspec, sta_info)
│   └── profile/             # Hardware profiles (temperature, memory, system load)
├── scripts/
│   ├── install.sh           # One-line installer and startup hook
│   ├── uninstall.sh         # Clean uninstaller and mount rollback
│   ├── unifi-bridge.sh      # CLI manager companion (/jffs/scripts/unifi-bridge)
│   ├── update.sh            # Automated update engine & track switcher
│   ├── sync_nodes.sh        # Cluster discovery, node sync, and update propagation
│   ├── watchdog.sh          # Self-healing health cron watchdog
│   └── apply_config.sh      # Native /start_apply.htm WebUI backend handler
├── webui/
│   └── unifi-bridge.asp     # Asuswrt-Merlin Wireless WebUI page
└── version.txt              # Semantic version definition
```

---

## Quick Start (Asuswrt-Merlin)

### 1. Installation
SSH into your Asuswrt-Merlin router (Main router or Satellite Node in AP mode) and run:

```sh
curl -fsSL https://raw.githubusercontent.com/agnithin/unifi-ap-bridge/main/scripts/install.sh | sh
```

### 2. Configuration & Adoption
1. In the router WebUI, navigate to **Advanced Settings > Wireless** and click the **UniFi AP Bridge** tab (or open `http://<router-ip>/user3.asp`).
2. Verify **Controller Discovery Mode** is set to **Auto-detect Gateway** (`http://<gateway-ip>:8080/inform`) or supply a custom controller URL.
3. Open your **UniFi Network Web UI** (`https://<controller-ip>`), go to **UniFi Devices**.
4. Locate the pending AP (e.g., `UniFi 6 Mesh` matching your router's MAC address).
5. Click **Adopt**.
6. The bridge automatically completes the cryptographic key exchange (`setparam`), transitions to **AES-GCM**, and starts streaming live wireless telemetry.

---

## Updates & Release Tracks

You can check for updates and switch release tracks directly from the **WebUI** or via the **CLI**:

### Via WebUI
1. Navigate to **Advanced Settings > Wireless > UniFi AP Bridge**.
2. Under **Software Updates & Release Track Management**:
   * Select your desired track: **Official Release (`main`)** or **Development Builds (`dev`)**.
   * Click **Check for Updates**.
   * If a new version is available, click **Update Now**. The router will download the package, restart the daemon, and automatically propagate the update to all satellite nodes.

### Via CLI
```bash
# Check for updates on official release track
unifi-bridge update check main

# Check for updates on development track
unifi-bridge update check dev

# Apply update for current or specified track
unifi-bridge update apply dev

# Interactive terminal menu (option 7 for update manager)
unifi-bridge
```

---

## CLI Management Companion

The `unifi-bridge` command is available directly on the router CLI:

```bash
unifi-bridge status            # Live PID, model, version, 2.4G/5G client counts, CPU temp, and cluster sync
unifi-bridge sync              # Immediately push configuration to all Satellite Nodes
unifi-bridge update check      # Check GitHub repository for new software updates
unifi-bridge update apply      # Download and install latest update across cluster
unifi-bridge check             # Probe UniFi controller inform endpoint connectivity
unifi-bridge logs              # Follow live daemon logs (tail -f /tmp/unifi-bridge.log)
unifi-bridge restart           # Gracefully restart the bridge daemon
unifi-bridge stop              # Stop the daemon
unifi-bridge start             # Start the daemon
```

---

## Wi-Fi Generation Classification

| Client Standard | Protocol Spec | 2.4 GHz Band | 5 GHz Band | Controller Badge |
|---|---|---|---|---|
| **Wi-Fi 6** | `802.11ax` | `wifi-ng` (HE) | `wifi-na` (HE) | **WiFi 6** |
| **Wi-Fi 5** | `802.11ac` | N/A | `wifi-na` (VHT) | **WiFi 5** |
| **Wi-Fi 4** | `802.11n` | `wifi-ng` (HT) | `wifi-na` (HT) | **WiFi 4** |
| **Legacy** | `802.11b/g/a` | `wifi-b/g` | `wifi-a` | **WiFi 3 / Legacy** |

---

## Development & Testing

```bash
# Run Go tests and cluster regression checks
go test -race ./...
go vet ./...
python3 scripts/test_sync_nodes.py
python3 scripts/test_update_backend.py
python3 scripts/test_update_apply.py
python3 scripts/test_startup.py
python3 scripts/test_secret_guard.py
python3 scripts/check_secrets.py
node scripts/test_update_ui.js
for script in scripts/*.sh; do sh -n "$script"; done

# Build the command package, including health/log helpers.
# Supply metadata so runtime provenance is available for local builds as in CI.
BUILD_VERSION=$(tr -d '[:space:]' < version.txt)
BUILD_COMMIT=$(git rev-parse HEAD)
BUILD_DATE=$(date -u +%Y-%m-%d)
BUILD_FLAGS="-s -w -X main.version=$BUILD_VERSION -X main.commit=$BUILD_COMMIT -X main.buildDate=$BUILD_DATE"
go build -ldflags="$BUILD_FLAGS" -o bin/unifi-bridge ./cmd/unifi-bridge

# Cross-compile static ARMv7 binary for Asuswrt-Merlin (RT-AX86U, XT8, etc.)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="$BUILD_FLAGS" -o bin/unifi-bridge-armv7 ./cmd/unifi-bridge

# Cross-compile static ARM64 binary (GT-AXE16000, RT-AX89X, etc.)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$BUILD_FLAGS" -o bin/unifi-bridge-arm64 ./cmd/unifi-bridge
```

---

## Roadmap & Feature Status

### Completed Features
* [x] **Zero-Dependency Architecture:** Static Go binary running directly on clean Asuswrt-Merlin firmware.
* [x] **UniFi Inform Engine:** Wire protocol implementation supporting AES-128-CBC bootstrap and AES-128-GCM adopted encryption.
* [x] **Broadcom Wireless Telemetry:** Automatic discovery of physical and virtual VAPs across 2.4 GHz and 5 GHz bands.
* [x] **Wi-Fi 6 (`802.11ax`) & Wi-Fi 4 (`802.11n`) Classification:** Generation mapping covered by parser tests; unknown driver formats require validation.
* [x] **Physical Switch Port Telemetry & Bridge FDB Client Discovery (`U6-IW` Emulation) (v1.2.2):** 2.5G WAN uplink and 1G LAN ports reporting live carrier state, speed, full duplex, STP state, traffic counters, and learned client MAC tables (`mac_table`) directly into the UniFi Port Manager.
* [x] **LLDP Topology Telemetry & Conflict Prevention (v1.2.2):** Discovers upstream neighbors via `lldpcli`, preserving distinct remote ports (Port 1 for Main, Port 3 for Node) and aligning with gateway `downlink_table` entries.
* [x] **Router CPU/Memory Stats & WiFi Experience Scoring (v1.2.0):** The deployed build reports load-derived CPU estimates and heuristic experience scores. The current source fix uses CPU counter deltas and avoids invented noise/experience when measurements are missing; remaining quality improvements are tracked below.
* [x] **RF Channel Utilization & Airtime Telemetry (v1.1.0):** Broadcom `chanim_stats` streaming `cu_total`, `cu_self_rx`, and `cu_self_tx` into UniFi RF graphs and router WebUI.
* [x] **Cumulative Client Traffic Usage Counters (v1.1.0):** Parsing `tx/rx` unicast bytes, packets, retries, rates, and association uptime from `wl sta_info`.
* [x] **Native Asuswrt WebUI Integration & Form Isolation (v1.2.2):** Advanced Settings > Wireless tab with dynamic slot discovery (`user1`–`user20.asp`), zero-refresh background polling, and form preservation.
* [x] **Multi-Arch Automated CI/CD:** GitHub Actions release workflow building ARMv7 and ARM64 binary packages.
* [x] **Automated In-WebUI Updates & Dual Tracks:** Instant release/dev update checks and one-click upgrades.
* [x] **Cluster Mesh Synchronization:** Centralized AP configuration and binary propagation over SSH.

### Prioritized backlog and operator decisions

The detailed evidence and acceptance criteria are in [the project audit/backlog](docs/AUDIT_STATE.md#8-approved-backlog-and-fix-batch--2026-10-25). The current batch is source changes under test, not a deployed release. APs remain on the verified v1.2.4 dev build.

| Work | Status / next step |
|---|---|
| B1–B2: Reliable bridge-addon updates | Fail-closed download/checksum/archive/version/startup/heartbeat checks implemented locally. Immutable complete release bundles, transaction locking, rollback and verified per-node outcomes remain pending. Stable release publication is separate from merging main. |
| B3: Preserve settings | JSON merge preserves custom WLAN mappings, node lists, topology and unknown fields; validation, private atomic writes and no-op detection implemented locally. |
| B4: Respect disabled state | Daemon and shared tracked startup enforce enabled=false for boot, cluster and manual entry points; startup distribution included in addon updates. Local regression tests pass. |
| B5: Accurate tri-band telemetry | Separate physical radio/VAP identities, actual channel widths and per-radio counts implemented locally. Third-radio rendering under U6IW still needs controller acceptance testing. |
| B6: Exposed controller key | Historical exposure confirmed; the old key still authenticated at investigation time. Operator approved rotation; replacement creation/install/revocation verification is tracked in the audit. No values belong in Git/chat. |
| B7–B10: Health and telemetry correctness | CPU deltas, unknown noise/experience handling, topology/FDB corrections, UI freshness/unverified labels and watchdog log cap implemented locally. Sampling deadlines/caching, stable traffic counters, rollback, structured config verification and remaining form transport work stay open. |
| F1: WLAN discovery | Resolve/cache exact SSID-to-legacy-ObjectId mapping; handle duplicate names and modern UUID differences. Keep broad API credentials off the AP. |
| F2: Guest/VLAN classification | Report actual VAP bridge/VLAN/isolation metadata first. Classification does not enforce network policy. |
| F3: Roaming | Already visible in UniFi per operator. No replacement roam-event implementation planned without a demonstrated gap; optional future detail is timing, reason codes and transitions shorter than sampling. |
| F4: RF diagnostics | Passive statistics only for now. Continuous spectrum/scanning-radio parity requires supported hardware; active off-channel scans/captures and proprietary AirView/Channel AI parity are deferred. |
| F5: Simple UniFi device commands | Reboot and other small commands are eligible for an explicit typed allowlist and controlled tests. Reboot interrupts service; no arbitrary command execution or automatic live reboot tests. |
| F6: UniFi provisioning / firmware | **Deferred by operator:** firmware updates, UniFi-triggered bridge upgrades and broad SSID/security/radio/VLAN provisioning. Asus remains the configuration authority. Existing GitHub bridge-addon updater correctness is a separate maintenance fix. |
| F7: Community distribution | amtm/SNBForums submission after installation, release integrity and compatibility gaps are closed. |


---

## License

GNU General Public License v3.0 (GPL-3.0) — see [LICENSE](LICENSE) for details.  
Copyright (C) 2026 Nithin. All rights reserved.
