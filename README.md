# unifi-ap-bridge

> **Current delivery status — 2026-10-26 PDT:** Main is deployed and runtime-accepted on `cd7bd64` (v1.2.5 dev), including the wireless diagnostics batch. Node remains untouched on last-verified `6224430`. Asus diagnostics and unchanged-save acceptance passed; changed-save, stale-radio cleanup and Node rollout remain pending. See [current audit state](docs/AUDIT_STATE.md).

A high-performance, ultra-lightweight standalone daemon and WebUI addon for **Asuswrt-Merlin** and Linux routers that bridges Broadcom Wi-Fi and switch telemetry into the **Ubiquiti UniFi Inform Protocol** (`:8080/inform`).

It emulates a genuine UniFi 6 Access Point—featuring native **UniFi 6 In-Wall (`U6-IW`)** switch port emulation as well as **UniFi 6 Mesh (`U6M`)**, **UniFi 6 Pro (`U6P`)**, and **UniFi 6 Lite (`U6L`)**—allowing non-UniFi access points to seamlessly report native wireless client classification, association telemetry, Wi-Fi 6 (`802.11ax`) connection rates, physical switch port link states, and live RSSI into the **UniFi Network Controller**.

---

## Current deployment

Main is accepted on `cd7bd64`, version `1.2.5 dev`. The exact-commit updater
and SSH runner exited 0 in 17.690 seconds. Independent checks verified protected
config/adoption/key state, fresh GCM, watchdog, clean transaction/locks, physical
radio widths 20/40/80 MHz and a 2.5 GbE Port 1 uplink. Noise/OBSS diagnostics are live.
Reboot handling is disabled and no scan was run.

UniFi now displays Main's upper 5 GHz channel 161 at 80 MHz after canonical
802.11ac capability reporting and a normal controller width save. Main's extra
configured Auto radio remains open. Signed-in diagnostics and unchanged-save acceptance passed without restart
or Node sync; changed-save acceptance remains pending. Node was
not accessed or changed; its last verified build remains `6224430`.

See [audit state](docs/AUDIT_STATE.md) and [architecture](docs/ARCHITECTURE.md).

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
  * v1.2.5 source uses `/proc/stat` CPU deltas and `/proc/meminfo`, preferring MemAvailable; first/unknown CPU samples are omitted. This replaces the prior v1.2.4 load-derived estimate.
  * Renders live CPU and RAM status cards in the Asuswrt WebUI and CLI.
* **WiFi Experience Scoring (v1.2.0+):**
  * Dynamically computes 0–100% `satisfaction` scores for each associated station using a signal/cumulative-retry heuristic; v1.2.5 omits the score when signal is missing. This is not native UniFi scoring parity, populating the UniFi "WiFi Experience" column.
* **Live RF Channel Utilization & Airtime Telemetry (v1.1.0+):**
  * Continuously polls Broadcom `wl chanim_stats` on physical 2.4 GHz and 5 GHz radios.
  * Dynamically computes total channel utilization (`cu_total`), AP transmit airtime (`cu_self_tx`), and AP receive airtime (`cu_self_rx`).
  * Streams RF metrics directly into `radio_table` and `radio_table_stats` in the UniFi Inform payload and displays real-time channel load percentages in the Asuswrt WebUI and CLI.
* **Cumulative Client & VAP Traffic Counters (v1.1.0+):**
  * Parses unicast transmission and reception bytes/packets (`tx ucast bytes`, `rx ucast bytes`, `tx ucast pkts`, `rx ucast pkts`), packet retries, uplink/downlink bitrates, association uptime, and idletime from `wl sta_info`.
  * Prefers unicast counters when provided by the driver. Fallback total counters and station-summed VAP counters require care across client churn; stable per-interface counters remain pending.
  * Aggregates counters per-client in `sta_table` and per-SSID in `vap_table`, populating UniFi real-time client throughput and bandwidth utilization graphs.
* **Ultra-Low Resource Footprint:**
  * **RAM:** approximately 9.7 MB RSS per AP in the v1.2.5 rollout sample; whole-router memory includes other services and RAM-backed files.
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
  * GitHub Actions automated builds producing static ARMv7 (`armv7l`) and ARM64 (`aarch64`) tarballs and SHA-256 manifests after Go, cluster regression, and shell syntax checks. Normal release-tag and `dev` pushes trigger builds; documentation-only commits can skip CI. The deployed v1.2.4 updater does not enforce the manifest; its package was manually verified. v1.2.5 source enforces checksum, archive, version and fresh runtime/heartbeat checks.

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
│   ├── install.sh           # Installer requiring matching repository assets
│   ├── start.sh             # Tracked startup; JSON-aware enabled check
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
Installation requires matching installer, startup scripts, WebUI and architecture-specific binary assets. The standalone installer URL is not a complete package; fresh-install packaging and stable-release acceptance remain pending.

The first upgrade from v1.2.4 must explicitly bootstrap the reviewed v1.2.5 scripts/binary, verify checksums and install the tracked startup asset. The legacy one-click updater cannot enforce the new checks before this migration. See the [rollout boundary](docs/AUDIT_STATE.md#verification-and-deployment-boundary). Existing operational commands below are not a recommendation to bypass that gate.

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
   * **Update Now** installs the addon and requests cluster propagation. The v1.2.5 migration gate above must be completed first; local success does not verify completion on every node. UniFi-triggered updates and router firmware updates remain deferred.

### Via CLI
```bash
# Check for updates on official release track
unifi-bridge update check main

# Check for updates on development track
unifi-bridge update check dev

# Installation syntax; use only after reviewed migration preparation
# unifi-bridge update apply dev

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

### Implemented features (source and deployment distinguished above)
* [x] **Zero-Dependency Architecture:** Static Go binary running directly on clean Asuswrt-Merlin firmware.
* [x] **UniFi Inform Engine:** Wire protocol implementation supporting AES-128-CBC bootstrap and AES-128-GCM adopted encryption.
* [x] **Broadcom Wireless Telemetry:** Physical/virtual discovery across 2.4/5 GHz; v1.2.5 adds complete tri-radio identity and configured VIF discovery, verified in the live UniFi overview on both APs.
* [x] **Wi-Fi 6 (`802.11ax`) & Wi-Fi 4 (`802.11n`) Classification:** Generation mapping covered by parser tests; unknown driver formats require validation.
* [x] **Physical Switch Port Telemetry & Bridge FDB Client Discovery (`U6-IW` Emulation) (v1.2.2):** 2.5G WAN uplink and 1G LAN ports reporting live carrier state, speed, full duplex, traffic counters, and learned client MAC tables (`mac_table`) directly into the UniFi Port Manager.
* [x] **LLDP Topology Telemetry & Conflict Prevention (v1.2.2):** Discovers upstream neighbors via `lldpcli`, preserving distinct remote ports (Port 1 for Main, Port 3 for Node) and aligning with gateway `downlink_table` entries.
* [x] **Router CPU/Memory Stats & WiFi Experience Scoring (v1.2.0):** The deployed v1.2.5 build uses CPU counter deltas and avoids invented noise/experience when measurements are missing; remaining quality improvements are tracked below.
* [x] **RF Channel Utilization & Airtime Telemetry (v1.1.0):** Broadcom `chanim_stats` streaming `cu_total`, `cu_self_rx`, and `cu_self_tx` into UniFi RF graphs and router WebUI.
* [x] **Cumulative Client Traffic Usage Counters (v1.1.0):** Parsing `tx/rx` unicast bytes, packets, retries, rates, and association uptime from `wl sta_info`.
* [x] **Native Asuswrt WebUI Integration & Form Isolation (v1.2.2):** Advanced Settings > Wireless tab with dynamic slot discovery (`user1`–`user20.asp`), zero-refresh background polling, and form preservation.
* [x] **Multi-Arch Automated CI/CD:** GitHub Actions release workflow building ARMv7 and ARM64 binary packages.
* [x] **Automated In-WebUI Updates & Dual Tracks:** Instant release/dev update checks and one-click upgrades.
* [x] **Cluster Mesh Synchronization:** Centralized AP configuration and binary propagation over SSH.

### Parallel wireless-feature batch (deployed to Main)

The current dev batch adds semantic no-op settings saves (no bridge restart or
node sync), bounded AP-local station-change history and passive RF trends in the
Asus UI, host-resolved WLAN mapping refresh, default-off authenticated reboot
dispatch, and an explicitly opted-in manual passive scan. These changes are
deployed to Main as `cd7bd64`; runtime, passive metrics, signed-in diagnostics and
unchanged settings saves are verified. Changed saves, WLAN refresh setup and
disruptive command/scan acceptance remain pending.
See [the current feature batch](docs/AUDIT_STATE.md#20-parallel-wireless-feature-batch).

WLAN discovery runs on a trusted host using `scripts/ops/wlan_mapping.py`.
Publish its allowlisted JSON atomically to a private RAM file on the AP and set
`wlan_mapping_file` to that path during an approved configuration change.
The daemon refreshes the file without restarting; explicit `wlan_mapping`
entries take precedence, and malformed refreshes preserve the last good cache.
No controller credential belongs on the AP. Refresh scheduling/publication is a
separate operational setup, not automatically installed by this source change.

Reboot support requires `controller_reboot_enabled: true` and an adopted,
non-bootstrap authenticated GCM session. Only the supported `cmd/reboot` shape
with a request ID can run the fixed `/sbin/reboot` operation. A private command-ID
state file alongside adoption state prevents repeated IDs across process/AP
restarts. Default is off; live command/firmware acceptance is pending. Locate
and legacy command shapes can be observed as sanitized metadata but do not
control LEDs or execute arbitrary commands.

The manual passive scan is never scheduled automatically. After explicit
authorization for a potentially disruptive serving-radio scan, its command is:

```sh
unifi-bridge -rf-scan-interface eth4 -rf-scan-channels 6 -allow-disruptive-rf-scan
```

The operation uses 100 ms passive/home dwell, an explicit channel list, a
10-second command deadline and a 16 KiB output cap. It prints only the path to a
mode-0600 JSON result in RAM. The driver starts scans asynchronously, so the
deadline bounds command execution, not proven firmware completion; returned
scan-results freshness is explicitly unknown. No scan or packet capture has
been run for acceptance. Packet capture remains unavailable without a supported
capture tool; Main's read-only inventory did not find `tcpdump`.

### Prioritized backlog and operator decisions

Use [the current feature batch](docs/AUDIT_STATE.md#20-parallel-wireless-feature-batch) for the newly prioritized wireless features and [the audit follow-up queue](docs/AUDIT_STATE.md#17-audit-reconciliation-and-accepted-follow-up-tasks) for the remaining operational/evidence gaps. Earlier worker packets remain dated history; the Main rollout is accepted and no further live operation is implicitly authorized.

Main deployment and its preserved wrapper result are documented in [Section 19](docs/AUDIT_STATE.md#19-approved-main-launch-and-live-acceptance--2026-10-26-2325-utc). Older rollout entries are historical evidence. Source completion, hosted publication, AP deployment and live acceptance are separate milestones.

| Work | Status / next step |
|---|---|
| B1–B2: Reliable bridge-addon updates | Fail-closed download/checksum/archive/version/startup/heartbeat checks implemented and tested in dev. A follow-up source fix serializes checks/installs and preserves active status against overlapping requests; its rollout is pending. Immutable complete release bundles, cross-operation settings/sync locking, rollback and verified per-node outcomes remain pending. Stable release publication is separate from merging main. |
| B3: Preserve settings | JSON merge preserves custom WLAN mappings, node lists, topology and unknown fields; validation, private atomic writes and no-op detection implemented and tested in dev. |
| B4: Respect disabled state | Daemon and shared tracked startup enforce enabled=false for boot, cluster and manual entry points; startup distribution included in addon updates. Local regression tests pass. |
| B5: Accurate tri-band telemetry | Separate physical radio/VAP identities, actual channel widths and per-radio counts implemented and tested in dev. Three-radio rendering under U6IW passed in the actual UniFi overview on both APs; other diagnostic/control surfaces remain outside that check. |
| B6: Exposed controller key | Resolved: replacement installed privately in CT107 and verified; exposed key revoked (old 401, new 200). Temporary secret copies removed. Historical exposure remains documented without key values. |
| B7–B10: Health and telemetry correctness | CPU deltas, unknown noise/experience handling, topology/FDB corrections, UI freshness/unverified labels and watchdog log cap implemented and tested in dev. Sampling deadlines/caching, stable traffic counters, rollback, structured config verification and remaining form transport work stay open. |
| F1: WLAN discovery | `scripts/ops/wlan_mapping.py` now reads the fixed classic WLAN-list endpoint from a host, requires an exact SSID allowlist, rejects duplicate/missing names and UUIDs, and prints only SSID→legacy-ObjectId JSON. Keep the API key on the host; review/export is separate from applying settings to an AP. |
| F2: Guest/VLAN classification | Report actual VAP bridge/VLAN/isolation metadata first. Classification does not enforce network policy. |
| F3: Roaming | New bounded AP-local poll-derived client/radio/BSSID timeline and known signal in Asus UI; source-tested, not deployed. Cross-AP destination, precise event time and disconnect reasons remain unknown unless another verified source supplies them. |
| F4: RF diagnostics | New passive noise/OBSS fields and RF trends plus explicit opt-in manual passive scan source; no automatic scan or live acceptance. Results freshness unknown; packet capture and proprietary spectrum parity remain unresolved. |
| F5: Simple UniFi device commands | Default-off authenticated GCM cmd/reboot dispatcher, fixed executor and persistent request-ID deduplication are source-tested. Real command/firmware acceptance is pending. Locate is observation-only; no firmware/provisioning handler. |
| F6: UniFi provisioning / firmware | **Deferred by operator:** firmware updates, UniFi-triggered bridge upgrades and broad SSID/security/radio/VLAN provisioning. Asus remains the configuration authority. Existing GitHub bridge-addon updater correctness is a separate maintenance fix. |
| F7: Community distribution | amtm/SNBForums submission after installation, release integrity and compatibility gaps are closed. |

Run the WLAN resolver from a trusted host with controller access and a private API-key file. For a local controller using its self-signed certificate, pass `--insecure` explicitly; otherwise provide the controller CA with `--ca-file`.

```sh
python3 scripts/ops/wlan_mapping.py \
  --controller https://10.100.1.1 \
  --credential-file /path/to/private/unifi-api-key \
  --insecure \
  --ssid 'example-2g' --ssid 'example-5g'
```

The command prints only the requested exact-name mappings as JSON. It performs one GET and does not update `wlan_mapping` or write to the controller.


---

## License

GNU General Public License v3.0 (GPL-3.0) — see [LICENSE](LICENSE) for details.  
Copyright (C) 2026 Nithin. All rights reserved.
