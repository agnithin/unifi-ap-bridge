# Full-System Audit & Current Operational State

**Audit Date:** 2026-10-24 21:38 PDT  
**System Status:** **HEALTHY & FULLY OPERATIONAL**  
**Active Release Track:** `dev` (`v1.2.2`, commit `53153bb`)  
**Repository Path:** `/Users/nithinag/workspace/unifi-ap-bridge`

---

## 1. Network Topology & Hardware Infrastructure

```
                                [ WAN Internet ]
                                       │
                         ┌─────────────┴─────────────┐
                         │   Cloud Gateway Max       │
                         │   (UCGMAX, 10.100.1.1)    │
                         │   MAC: 6c:63:f8:30:9a:05  │
                         └──────┬──────┬──────┬──────┘
                   Port 1 (2.5G)│      │      │Port 4 (2.5G)
            ┌───────────────────┘      │      └─────────────────┐
            │             Port 3 (2.5G)│                        │
            ▼                          ▼                        ▼
 ┌──────────────────────┐   ┌──────────────────────┐   ┌─────────────────┐
 │ ZenWiFi_XT8-Main     │   │ ZenWiFi_XT8-Node     │   │ USW Flex 2.5G 5 │
 │ IP: 10.100.1.10      │   │ IP: 10.100.1.11      │   │ IP: 10.100.1.18 │
 │ MAC: fc:34:97:46:7b:10│   │ MAC: fc:34:97:4f:ac:60│   │ MAC: 8c:...     │
 │ Emulated Model: U6IW │   │ Emulated Model: U6IW │   └─────────────────┘
 ├──────────────────────┤   ├──────────────────────┤
 │ Port 1 (2.5G): UCG-1 │   │ Port 1 (2.5G): UCG-3 │
 │ Port 2 (1.0G): Down  │   │ Port 2 (1.0G): Down  │
 │ Port 3 (1.0G): Down  │   │ Port 3 (1.0G): Down  │
 │ Port 4 (1.0G): Down  │   │ Port 4 (1.0G): Up    │
 │                      │   │  └─ Wired HomeBase   │
 ├──────────────────────┤   ├──────────────────────┤
 │ 2.4 GHz: 24 clients  │   │ 2.4 GHz: 18 clients  │
 │ 5.0 GHz: 5 clients   │   │ 5.0 GHz: 13 clients  │
 └──────────────────────┘   └──────────────────────┘
        Total Active Wi-Fi Clients: 60 (Zero Disruption)
```

---

## 2. Live Verified State & Controller Ingestion

### 2.1 Access Point Adoption & Telemetry
| Property | `ZenWiFi_XT8-Main` (`.10`) | `ZenWiFi_XT8-Node` (`.11`) | Verification Method |
|---|---|---|---|
| **UniFi Controller Name** | `ZenWiFi_XT8-Main` | `ZenWiFi_XT8-Node` | Controller REST API (`PUT /rest/device/{id}`) |
| **Emulated Model** | `U6IW` (UniFi 6 In-Wall) | `U6IW` (UniFi 6 In-Wall) | Controller `stat/device` |
| **Status** | Online (AES-GCM encrypted) | Online (AES-GCM encrypted) | Heartbeat `:8080/inform` every 10s |
| **Associated Clients** | 29 stations (24 on 2.4G, 5 on 5G) | 31 stations (18 on 2.4G, 13 on 5G) | Broadcom `wl assoclist` & controller |
| **Airtime Utilization** | 2.4G: ~75% load, 5G: ~14% load | 2.4G: ~66% load, 5G: ~26% load | `wl chanim_stats` & `radio_table_stats` |
| **CPU Temperature** | 65.1 °C | 64.6 °C | `/sys/class/thermal/thermal_zone0/temp` |
| **Daemon Version** | `1.2.2` (`dev` track) | `1.2.2` (`dev` track) | Daemon runtime & `cluster.js` |
| **Cluster Sync** | `In Sync (Local)` | `In Sync` | `/jffs/addons/unifi-bridge/sync_nodes.sh` |

### 2.2 Physical Switch Port Telemetry (UniFi Port Manager)
* **Main AP (`10.100.1.10`):**
  * **Port 1 (Uplink):** `up: true`, `speed: 2500` (2.5 GbE full-duplex wire link to UCG Max Port 1).
  * **Ports 2–4:** `up: false`, `speed: 0`, `stp_state: "disabled"`.
* **Node AP (`10.100.1.11`):**
  * **Port 1 (Uplink):** `up: true`, `speed: 2500` (2.5 GbE full-duplex wire link to UCG Max Port 3).
  * **Ports 2–3:** `up: false`, `speed: 0`, `stp_state: "disabled"`.
  * **Port 4 (LAN 3):** `up: true`, `speed: 1000` (1 GbE full-duplex wire link active to physical HomeBase hub).
* **Metadata Emitted:** `op_mode: "switch"`, `forward: "all"`, `autoneg: true`, `enabled: true`, `stp_state: "forwarding"`, `stp_pathcost: 8000/20000`, and live byte/packet counters from `/proc/net/dev`.

### 2.3 LLDP Topology & Upstream Gateway Linking
* **Gateway Chassis MAC:** `6c:63:f8:30:9a:05` (Cloud Gateway Max hardware MAC).
* **Downlink Table on Cloud Gateway Max:**
  * Port 1: `fc:34:97:46:7b:10` (`ZenWiFi_XT8-Main`) at 2500 Mbps.
  * Port 3: `fc:34:97:4f:ac:60` (`ZenWiFi_XT8-Node`) at 2500 Mbps.
  * Port 4: `8c:ed:e1:12:7a:fc` (`USW Flex 2.5G 5`) at 2500 Mbps.
* **AP LLDP Reporting:**
  * Both APs dynamically execute `lldpcli -f json show neighbors` to resolve parent switch ports without static hardcoding.

### 2.4 Asuswrt-Merlin WebUI State (`http://10.100.1.10/user3.asp`)
* **Zero-Reload Architecture:** Neutralized Asuswrt's `window.refreshpage` and cleared form target redirects on background submits.
* **Form Preservation:** Inputs are populated once on page load via `initFormOnce()`. Background telemetry updates do not overwrite modified fields or reset cursor position.
* **Reload Guard:** Full-page reloads only fire when the user actively triggers a cluster update via the "Update Now" button.

---

## 3. Key Findings Resolved During This Audit Cycle

1. **Model Immutability on Adoption:**
   * *Problem:* Switching model code from `U6M` to `U6IW` in daemon configs was ignored by the controller because adopted device models are immutable in UniFi MongoDB.
   * *Resolution:* APs were forgotten ("Removed") in UniFi UI and cleanly re-adopted as native `U6IW`, unlocking the multi-port switch capabilities.
2. **Topology Resolution Failure ("Connected To" Empty):**
   * *Problem:* Asuswrt ARP cache reported `6e:63:f8:30:9a:06` for `10.100.1.1` (the Linux bridge MAC with locally-administered bit set), which did not match any adopted device in the controller.
   * *Resolution:* Switched to native LLDP queries (`lldpcli`), extracting the true hardware Chassis ID (`6c:63:f8:30:9a:05`) and remote port mapping (`eth0` &rarr; Port 1, `eth2` &rarr; Port 3).
3. **Port Manager Reporting Disconnected Links:**
   * *Problem:* Raw `port_table` lacked canonical port names and switch metadata (`op_mode: "switch"`, `forward: "all"`, `autoneg: true`, `stp_state: "forwarding"`).
   * *Resolution:* Standardized port telemetry and emitted `sw_port_table` alongside `port_table`. Active wire links on Port 1 (2.5G) and Port 4 (1G) now report active link status and traffic.
4. **Endless WebUI Refresh Loop:**
   * *Problem:* Successful updates left `"status": "SUCCESS"` in `update_status.js`. On load, `initial()` read `SUCCESS` and queued `window.location.reload()` every 3 seconds indefinitely.
   * *Resolution:* Gated `window.location.reload()` behind `updatePollingTimer !== null`, and automatically transition update status to `UP_TO_DATE`.

---

## 4. Verification Commands (30-Second Auditor Check)

```bash
# 1. Verify Go tests pass
cd /Users/nithinag/workspace/unifi-ap-bridge && go test -v ./...

# 2. Check cluster sync & running daemon status on Main AP
ssh -o BatchMode=yes nithin@10.100.1.10 '/jffs/addons/unifi-bridge/sync_nodes.sh status'

# 3. Query UniFi Controller API for adopted AP state & port tables (via CT107)
ssh -o BatchMode=yes root@10.100.1.100 "pct exec 107 -- python3 -c '
import urllib.request, ssl, json
ctx = ssl._create_unverified_context()
req = urllib.request.Request(\"https://10.100.1.1/proxy/network/api/s/default/stat/device\", headers={\"X-API-KEY\": \"35o1Ufz9SMoQRKqwtncxZoyy_qmte3OD\"})
data = json.loads(urllib.request.urlopen(req, context=ctx).read())[\"data\"]
for d in data:
    if d.get(\"mac\") in [\"fc:34:97:46:7b:10\", \"fc:34:97:4f:ac:60\"]:
        print(d[\"name\"], d[\"model\"], \"Adopted:\", d[\"adopted\"], \"NumSta:\", d[\"num_sta\"])
'"
```
