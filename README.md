# unifi-ap-bridge

[![CI / Release](https://github.com/agnithin/unifi-ap-bridge/actions/workflows/release.yml/badge.svg)](https://github.com/agnithin/unifi-ap-bridge/actions)
[![Current Version](https://img.shields.io/badge/version-1.2.15-blue.svg)](version.txt)
[![Target](https://img.shields.io/badge/target-Asuswrt--Merlin%20%7C%20Broadcom-green.svg)](docs/ARCHITECTURE.md)

A high-performance, ultra-lightweight standalone daemon and WebUI extension for **Asuswrt-Merlin** and Broadcom Linux routers that bridges wireless and switch telemetry into the **Ubiquiti UniFi Inform Protocol** (`:8080/inform`).

By emulating a **UniFi 6 In-Wall (`U6-IW`)** access point, `unifi-ap-bridge` allows non-UniFi hardware (such as the ASUS ZenWiFi XT8 cluster) to seamlessly report native wireless classification, 802.11ax connection bitrates, RF airtime utilization, physical switch port carrier states, and bridge topology directly into the **UniFi Network Application** (Cloud Gateway Ultra / Max / Enterprise).

---

## Unique Features

### 1. Native UniFi Switch Port Manager Emulation
* **Multi-Port Physical Mapping:** Emulates the `U6-IW` managed switch personality. The 2.5 Gbps WAN/LAN port and all 1 Gbps Gigabit LAN ports appear directly in the UniFi Port Manager (`/network/default/ports`) with real-time link speeds, duplex states, and byte/packet counters.
* **Bridge FDB Client Attribution:** Inspects the Linux bridge forwarding database (`brctl showmacs`) and ARP tables. Downstream wired clients (such as Philips Hue hubs, Home Assistant appliances, or desktop PCs) plugged into the router's LAN ports are automatically attributed to their physical switch port in UniFi (`sw_mac` and `sw_port`), avoiding ambiguous gateway dumps.

### 2. Multi-Radio Broadcom RF Telemetry
* **Tri-Band RF Normalization:** Maps physical Broadcom radios (`eth4`, `eth5`, `eth6`) into normalized UniFi bands: `wifi-ng` (2.4 GHz), `wifi-na` (5 GHz-1), and `wifi-na-1` (5 GHz-2).
* **Live Channel Airtime Utilization:** Continuously polls Broadcom `chanim_stats` to stream channel load (`cu_total`), AP transmit airtime (`cu_self_tx`), and receive airtime (`cu_self_rx`) directly into UniFi RF environment graphs.
* **Granular Wi-Fi 6 Experience:** Queries `wl sta_info` for unicast byte/packet counters, RSSI attenuation, negotiated MCS rates (up to 1201 Mbps on 80 MHz 2ss), packet retry percentages, and dynamic satisfaction scoring.
* **Radio Liveness Probing (v1.2.12):** Validates hardware driver state (`wl isup`) before querying channel metrics, cleanly skipping disabled or offline radios without throwing telemetry collection errors.

### 3. Controller Command Execution & Safety Hardening
* **Locate LED Blinking:** Responds to controller Locate requests by blinking the router's front panel white LED for a bounded 15-second window, restoring prior brightness automatically upon completion.
* **Client Kick / Deauth (`kick-sta`):** Translates controller "Reconnect" clicks into immediate, bounded Broadcom deauthentication frames (`wl deauthenticate <mac>`), triggering client re-roaming.
* **Verified MAC Filter Updates:** Reads every interface policy before applying controller blocklists and refuses to replace ASUS allow lists. Mode and MAC-list readback must match; failures trigger a bounded restoration attempt on every touched interface. Driver failures can also prevent restoration, so this is not a network isolation guarantee.

### 4. Zero Flash Wear & Autonomous Resilience
* **Power-Loss Durability:** Employs atomic temp-write, `fsync()`, directory sync, and generational backup files (`state.json.bak`) for persistent AES-GCM adoption keys.
* **Non-Destructive Transport Handshake:** Transient HTTP 401/404 responses preserve adoption credentials for subsequent exchanges. Cryptographic unadoption only occurs upon verified AES-GCM reset packets.
* **Self-Healing Watchdog:** An autonomous 5-minute cron watchdog (`watchdog.sh`) monitors daemon freshness (`sample_fresh`) and evicts stale PID locks (>10m) automatically.

### 5. Seamless MerVLAN & AiMesh Coexistence
* **Zero PHY Resets:** Operates without calling disruptive hardware resets (`ethctl` or `ifconfig eth0 down`), protecting dedicated 5 GHz-2 AiMesh wireless backhauls and active client streams.
* **VLAN Trunking Harmony:** Operates alongside [MerVLAN](docs/VLAN_SECURITY_ARCHITECTURE.md) 802.1Q subinterfaces (`wl0.1`, `wl1.1` on Main vs `wl0.2`, `wl1.2` on Node) without interfering with Layer-2 guest isolation boundaries.

---

## Architectural Boundary

To ensure complete stability and prevent router configuration corruption, `unifi-ap-bridge` maintains a strict division of responsibility:

```
┌───────────────────────────────────────┐      ┌───────────────────────────────────────┐
│     UniFi Network Controller Owns     │      │        Asuswrt / MerVLAN Owns         │
├───────────────────────────────────────┤      ├───────────────────────────────────────┤
│ • Network Inventory & Device Topology │      │ • Physical Wi-Fi Radios & Frequencies │
│ • Client Classification & Signal Bars │      │ • SSIDs & WPA2/WPA3 Passphrases       │
│ • Switch Port Link States & Stats     │      │ • 802.1Q VLAN Trunking & IP Subnets   │
│ • Airtime Utilization & Satisfaction  │      │ • Client Routing, DHCP, & NAT         │
│ • Controller Commands (Locate, Kick)  │      │ • Firewall Rules & Guest Isolation    │
└───────────────────────────────────────┘      └───────────────────────────────────────┘
```

> [!NOTE]
> Client roaming decisions are 802.11 RSSI-driven by the client hardware. In accordance with operational stability guidelines, clients associate with the strongest AP signal; fragile driver-level MAC pinning is avoided to prevent client stranding during AP reboots.

---

## Hardware & Environment Baseline

| Device | Role | Platform / Model | IP Address | Bridged Personality |
|---|---|---|---|---|
| **UniFi Gateway** | Controller | Cloud Gateway Max (UCG-Max) | `10.100.1.1` | Native Controller (`:8080/inform`) |
| **ZenWiFi XT8 Main** | Root AP / Switch | BCM4908 (Tri-Band AX6600) | `10.100.1.10` | `U6-IW` (Port 1 Uplink to GW Port 1) |
| **ZenWiFi XT8 Node** | Mesh AP / Switch | BCM4908 (Tri-Band AX6600) | `10.100.1.11` | `U6-IW` (Port 1 Uplink to GW Port 3) |

---

## Quick Start & Operations

### Service Lifecycle
```bash
# Check service status and telemetry freshness
/jffs/scripts/unifi-bridge status

# Stop / Start / Restart daemon
/jffs/scripts/unifi-bridge restart

# Check daemon inform heartbeats
tail -n 25 /tmp/unifi-bridge.log
```

### Upgrading via Release Tracks
The built-in updater supports atomic cluster upgrades directly from GitHub releases:
```bash
# Check for updates on dev or main branch
/jffs/addons/unifi-bridge/update.sh check dev

# Apply latest release
/jffs/addons/unifi-bridge/update.sh apply dev
```

---

## Verification & Testing

The repository maintains full regression suites across Go, Python, and shell backends:
```bash
# Run Go unit and mock driver tests
go test ./...

# Run watchdog recovery and lock eviction tests
python3 scripts/test_watchdog.py

# Run cluster SSH synchronization tests
python3 scripts/test_sync_nodes.py
```

---

## Documentation Index

- [Live Operational Ledger & Status](docs/AUDIT_STATE.md)
- [System Architecture Specification](docs/ARCHITECTURE.md)
- [Layer-2 VLAN & Security Architecture](docs/VLAN_SECURITY_ARCHITECTURE.md)
- [Code Security & Reliability Audit (November 13, 2026)](docs/audit/2026-11-13-code-security-and-reliability-audit.md)
- [Product & Reliability Audit (November 2026)](docs/audit/2026-11-04-unifi-ap-bridge-product-and-reliability-audit.md)
