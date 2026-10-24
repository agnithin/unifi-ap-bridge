# unifi-ap-bridge

A high-performance, ultra-lightweight standalone daemon and WebUI addon for **Asuswrt-Merlin** and Linux routers that bridges Broadcom Wi-Fi telemetry into the **Ubiquiti UniFi Inform Protocol** (`:8080/inform`).

It emulates a genuine UniFi 6 Access Point (e.g., UniFi 6 Mesh `U6M`, UniFi 6 Pro `U6P`, UniFi 6 Lite `U6L`), allowing non-UniFi access points to seamlessly report native wireless client classification, roaming, Wi-Fi 6 (`802.11ax`) connection rates, and live RSSI into the **UniFi Network Controller**.

---

## Key Features

* **Zero Dependencies & Completely Standalone:**
  * Compiles to a single static binary (~6.9 MB stripped).
  * Requires **no Python**, **no Entware**, **no USB drive**, and **no third-party addons (no scMerlin needed)**.
  * Runs directly on clean Asuswrt-Merlin firmware.
* **Native Wireless Classification:**
  * Clients switch from generic wired switch port attachments to true wireless clients with live signal bars, SNR, and frequency bands (2.4 GHz vs 5 GHz).
  * IoT devices (ESP32, Sonoff, Wyze, smart appliances) correctly classify as **Wi-Fi 4 (`802.11n`)** rather than Wi-Fi 3.
  * Modern mobile and laptop devices classify as **Wi-Fi 6 (`802.11ax`)** or **Wi-Fi 5 (`802.11ac`)**.
* **Ultra-Low Resource Footprint:**
  * **RAM:** ~3–5 MB RSS (< 1% of 512 MB router RAM).
  * **CPU:** Virtually 0% (sleeps 99.9% of the time, waking every 10 seconds for a $<1$ ms local query).
  * **Flash Wear Prevention:** Runtime caches, logs, and WebUI status state reside in `/tmp/` (RAM disk); persistent adoption keys are stored safely with `0600` permissions.
* **Native Asuswrt-Merlin WebUI Integration:**
  * Integrated directly into **Advanced Settings > Wireless** tab bar (e.g. `http://<router-ip>/user3.asp`).
  * Uses Asuswrt-Merlin dynamic user slot allocation (`user1`–`user20.asp`), preventing collisions with YazFi, MerVLAN, or scMerlin.
  * Native Asuswrt styling with auto-detect gateway, custom inform URL, AP model selector, and live heartbeat telemetry log.
* **Cluster Mesh Synchronization:**
  * Auto-discovers Satellite Nodes directly from Asuswrt NVRAM (`cfg_device_list`) or MerVLAN.
  * Centralized management: configuring settings on the Main AP WebUI automatically pushes configurations to all Satellite Nodes over SSH.
  * Preserves per-node cryptographic adoption identities (`state.json`), preventing adoption collisions.
  * Real-time Cluster Synchronization table in WebUI with live client distribution and manual "Sync Now" trigger.
* **Built-in CLI Companion (`unifi-bridge`):**
  * Provides quick command-line diagnostics: `unifi-bridge status`, `sync`, `start`, `stop`, `restart`, `check`, `logs`.
* **Automated Health Watchdog:**
  * Automatic cron watchdog via Asuswrt `cru` checking process health every 5 minutes and respawning on failures.
* **Multi-Arch Automated CI/CD:**
  * GitHub Actions automated builds producing static ARMv7 (`armv7l`) and ARM64 (`aarch64`) tarballs with checksum verification.

---

## Architecture

```
unifi-ap-bridge/
├── .github/workflows/
│   └── release.yml          # GitHub Actions multi-arch release CI/CD (ARMv7 & ARM64)
├── cmd/
│   └── unifi-bridge/        # Standalone persistent daemon binary entrypoint
├── pkg/
│   ├── inform/              # UniFi Inform wire protocol (binary header, AES-CBC, AES-GCM, Zlib)
│   ├── models/              # Device capabilities catalog (U6M, U6P, U6L)
│   └── telemetry/           # Schema definitions (VAPs, Stations, Radios, Stats)
├── providers/               # Pluggable radio telemetry providers
│   ├── provider.go          # Interface: GetRadios(), GetVAPs(), GetRadioStats()
│   ├── broadcom/            # Broadcom Asuswrt provider (nvram, wl, chanspec, sta_info)
│   └── profile/             # Hardware profiles (temperature, memory, system load)
├── scripts/
│   ├── install.sh           # One-line installer and startup hook
│   ├── uninstall.sh         # Clean uninstaller and mount rollback
│   ├── unifi-bridge.sh      # CLI manager companion (/jffs/scripts/unifi-bridge)
│   ├── watchdog.sh          # Self-healing health cron watchdog
│   └── apply_config.sh      # Native /start_apply.htm WebUI backend handler
└── webui/
    └── unifi-bridge.asp     # Asuswrt-Merlin Wireless WebUI page
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

## CLI Management Companion

The `unifi-bridge` command is available directly on the router CLI:

```bash
unifi-bridge status     # Live PID, model, 2.4G/5G client counts, CPU temp, and cluster sync status
unifi-bridge sync       # Immediately synchronize configuration to all Satellite Nodes
unifi-bridge check      # Probe UniFi controller inform endpoint connectivity
unifi-bridge logs       # Follow live daemon logs (tail -f /tmp/unifi-bridge.log)
unifi-bridge restart    # Gracefully restart the bridge daemon
unifi-bridge stop       # Stop the daemon
unifi-bridge start      # Start the daemon
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
# Run all unit tests (wire vectors, Broadcom chanspec, mock provider)
go test -v ./...

# Build local binary
go build -o bin/unifi-bridge ./cmd/unifi-bridge

# Cross-compile static ARMv7 binary for Asuswrt-Merlin (RT-AX86U, XT8, etc.)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o bin/unifi-bridge-armv7 ./cmd/unifi-bridge

# Cross-compile static ARM64 binary (GT-AXE16000, RT-AX89X, etc.)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/unifi-bridge-arm64 ./cmd/unifi-bridge
```

---

## License
GNU General Public License v3.0 (GPL-3.0) — see [LICENSE](LICENSE) for details.
Copyright (C) 2026 Nithin. All rights reserved.
