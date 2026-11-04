# VLAN Security & Fail-Closed Isolation Architecture

> **Author:** Antigravity  
> **Date:** November 2026  
> **Scope:** Asus ZenWiFi XT8 (Main AP `10.100.1.10`, Node AP `10.100.1.11`), UniFi Cloud Gateway Max (`10.100.1.1`), MerVLAN, and `unifi-ap-bridge`.

---

## 1. Network Topology & Subnet Baseline

```mermaid
graph TD
    UCG["UniFi Cloud Gateway Max<br/>(10.100.1.1 / Gateway & Controller)"]
    
    subgraph "Main AP (ZenWiFi XT8 — 10.100.1.10)"
        MAIN_BR0["br0 (Native LAN / 10.100.1.0/24)<br/>SSID: blueberry-prime (wl0, wl1)"]
        MAIN_BR20["br20 (IoT VLAN 20 / 10.100.20.0/24)<br/>SSID: blueberry (wl0.1, wl1.1)"]
        MAIN_ETH0_20["eth0.20 (802.1Q Tagged Trunk)"]
        MAIN_BR20 --- MAIN_ETH0_20
    end

    subgraph "Node AP (ZenWiFi XT8 — 10.100.1.11)"
        NODE_BR0["br0 (Native LAN / 10.100.1.0/24)<br/>SSID: blueberry-prime (wl0, wl1, wl0.1, wl1.1)"]
        NODE_BR20["br20 (IoT VLAN 20 / 10.100.20.0/24)<br/>SSID: blueberry (wl0.2, wl1.2)"]
        NODE_ETH0_20["eth0.20 (802.1Q Tagged Trunk)"]
        NODE_BR20 --- NODE_ETH0_20
    end

    UCG ---|Port 1 Uplink (Trunk)| MAIN_ETH0_20
    UCG ---|Port 1 Uplink (Native)| MAIN_BR0
    UCG ---|Port 3 Uplink (Trunk)| NODE_ETH0_20
    UCG ---|Port 3 Uplink (Native)| NODE_BR0
```

### Subnet Definitions
* **Primary / Trusted LAN (`10.100.1.0/24`, Native / Untagged):**
  * Hosts routers, infrastructure servers, trusted management devices, and the native SSID (`blueberry-prime`).
  * Bound to default bridge `br0` on both APs.
* **IoT Network (`10.100.20.0/24`, 802.1Q Tagged VLAN 20):**
  * Hosts smart home devices, sensors, smart plugs, and isolated appliances on guest SSID (`blueberry`).
  * Bound to isolated bridge `br20` on both APs and tagged across uplink `eth0.20` to UCG-Max Port 1 & Port 3.

---

## 2. Root Cause Analysis: The Node AP Leak Phenomenon

### 2.1 Asuswrt & Broadcom Default Behavior
Under Asuswrt AP mode, the Broadcom wireless driver creates virtual guest AP interfaces (`wl0.1`, `wl0.2`, `wl1.1`, `wl1.2`) and binds them to the primary native bridge (`br0`) during initial firmware boot or any `restart_wireless` event.

### 2.2 AiMesh Slot Disparity
Due to AiMesh synchronization semantics on the ASUS ZenWiFi XT8:
* **Main AP (`10.100.1.10`):**
  * Primary Radios (`wl0`, `wl1`): `blueberry-prime`
  * Guest Slot 1 (`wl0.1`, `wl1.1`): `blueberry` (IoT SSID)
* **Node AP (`10.100.1.11`):**
  * Primary Radios (`wl0`, `wl1`): `blueberry-prime`
  * Guest Slot 1 (`wl0.1`, `wl1.1`): `blueberry-prime` (AiMesh auto-clones the native network to slot 1)
  * Guest Slot 2 (`wl0.2`, `wl1.2`): `blueberry` (IoT SSID)

### 2.3 The Node AP Boot Omission Defect
Historically, when MerVLAN's `nodeenable` configured satellite nodes, it only injected `TEMPLATE_SERVICES_ADDON` (WebUI mount) and `TEMPLATE_SERVICE_EVENT`, **omitting `TEMPLATE_SERVICES`** (`services-start` invoking `shield`, `manager`, and `cron`).

**Failure Sequence on Standalone Node Reboot:**
1. Node AP rebooted (power cycle, firmware restart, or standalone node reboot).
2. Asus firmware rc initialized `wl0.2` and `wl1.2` directly into `br0` (listed in `br0_ifnames`).
3. Because `services-start` lacked MerVLAN manager and boot shield, MerVLAN **never executed on the node at boot**.
4. IoT devices reconnected to `blueberry` on the Node AP and received DHCP leases in `10.100.1.0/24` directly from the primary native network.
5. The leak persisted indefinitely until an operator manually clicked "Sync Nodes" from the Main AP.

---

## 3. Multi-Layer Defense-in-Depth Implementation

To guarantee strict fail-closed isolation under all circumstances (cold reboots, wireless restarts, or complete MerVLAN process failure), six complementary protection layers were deployed:

```
+-------------------------------------------------------------------------+
| Layer 1: Firmware-Level Dynamic Filter (/jffs/scripts/firewall-start)  |
|          ebtables --logical-in br0 -i <guest_if> -j DROP                |
+-------------------------------------------------------------------------+
                                    |
+-------------------------------------------------------------------------+
| Layer 2: Early Boot Shield (mervlan_boot_wrap.sh shield)                |
|          ebtables MERV_DHCP_HOLD (drops UDP port 67 on br0 at boot)     |
+-------------------------------------------------------------------------+
                                    |
+-------------------------------------------------------------------------+
| Layer 3: Autonomous Node Startup (mervlan_boot.sh nodeenable)           |
|          Upstream PR #56: installs services-start with fail-closed      |
+-------------------------------------------------------------------------+
                                    |
+-------------------------------------------------------------------------+
| Layer 4: Dynamic Bridge Migration & NVRAM Scrub (lib_br0_guard.sh)      |
|          Creates br20 (eth0.20), evicts guest VAPs from br0_ifnames     |
+-------------------------------------------------------------------------+
                                    |
+-------------------------------------------------------------------------+
| Layer 5: Persistent Kernel MAC Shield (MERV_MAC chain)                  |
|          ebtables -s <mac> --logical-in br0 -j DROP (51 IoT devices)    |
+-------------------------------------------------------------------------+
                                    |
+-------------------------------------------------------------------------+
| Layer 6: Self-Healing Health Cron (heal_event.sh cron)                  |
|          Runs every 5 minutes on both APs to detect/correct drift       |
+-------------------------------------------------------------------------+
```

### Layer 1: Firmware-Level Dynamic Kernel Safety Net (`/jffs/scripts/firewall-start`)
A permanent shell script deployed to `/jffs/scripts/firewall-start` on both APs. 
- **Dynamic NVRAM Inspection:** Inspects `nvram get <iface>_ssid` at boot to find whichever interface hosts `blueberry`. Never hardcodes interfaces, ensuring AiMesh slot shifts can never block the main network.
- **Idempotent Delete-Before-Insert:** Older Asuswrt BusyBox `ebtables v2.0.10` lacks `-C` (check). The script executes an idempotent delete (`-D ... 2>/dev/null || true`) before `-I` to prevent rule accumulation across firewall restarts.
- **Explicit Logical Matching:** Uses `--logical-in br0`. When MerVLAN moves the interface to `br20`, the rule evaluates to false and legitimate IoT traffic passes to `eth0.20` unimpeded.

```sh
#!/bin/sh
IOT_SSID="blueberry"

for iface in wl0.1 wl0.2 wl0.3 wl1.1 wl1.2 wl1.3; do
  ssid=$(nvram get "${iface}_ssid" 2>/dev/null)
  if [ "$ssid" = "$IOT_SSID" ]; then
    ebtables -t filter -D FORWARD --logical-in br0 -i "$iface" -j DROP 2>/dev/null || true
    ebtables -t filter -I FORWARD --logical-in br0 -i "$iface" -j DROP
    ebtables -t filter -D INPUT --logical-in br0 -i "$iface" -j DROP 2>/dev/null || true
    ebtables -t filter -I INPUT --logical-in br0 -i "$iface" -j DROP
  fi
done
exit 0
```

### Layer 2: Early Boot Shield (`mervlan_boot_wrap.sh shield`)
Arms `MERV_DHCP_HOLD` (`-p IPv4 --ip-proto udp --ip-dport 67 -j DROP`) within the first 50ms of boot. This closes the race window where IoT clients attempt DHCP discovery before Asuswrt rc settles and MerVLAN moves interfaces.

### Layer 3: Autonomous Node Startup (`mervlan_boot.sh nodeenable` / PR #56)
Patched `functions/mervlan_boot.sh` so that when `BOOT_ENABLED=1`, `nodeenable` installs `TEMPLATE_SERVICES` (`services-start` invoking `shield`, `manager`, and `cron`) onto the Node AP.
- Includes strict fail-closed removal handling: if removing the boot block fails during a disable action (`BOOT_ENABLED=0`), or if template markers linger, it aborts with exit code `1` rather than reporting false success.
- Submitted upstream in [r80xcore/mervlan#56](https://github.com/r80xcore/mervlan/pull/56) with 5 automated contract test suites.

### Layer 4: Dynamic Bridge Migration & Live NVRAM Scrub (`lib_br0_guard.sh`)
- Builds `br20` with STP enabled and binds tagged subinterface `eth0.20`.
- Moves `wl0.1`/`wl1.1` (Main) or `wl0.2`/`wl1.2` (Node) into `br20`.
- Performs live runtime scrubbing of `br0_ifnames` without committing to flash, preventing Broadcom driver restarts from re-adding managed interfaces to `br0`.

### Layer 5: Persistent Kernel MAC Shield (`MERV_MAC`)
51 verified IoT client MAC addresses are registered in kernel `ebtables`:
`-s <MAC> --logical-in br0 -j DROP`
Even if an IoT device connects through an unmanaged port or transient interface, kernel ebtables unconditionally drops any frame originating from that MAC if it enters `br0`.

### Layer 6: Self-Healing Health Cron (`heal_event.sh cron`)
Active in `crontab` on both Main AP and Node AP (`*/5 * * * * /jffs/addons/mervlan/functions/heal_event.sh cron`). Verifies bridge topology every 5 minutes and immediately heals any interface placement drift.

---

## 4. Verification Evidence & Live Test Results

### 4.1 Node AP Cold Reboot Verification (2026-11-04)
The Node AP (`10.100.1.11`) was rebooted independently via SSH:
1. **01:47:33:** Reboot triggered.
2. **01:49:08:** ICMP and SSH returned. `firewall-start` drops engaged immediately.
3. **01:49:10:** `mervlan_boot_wrap.sh shield` held DHCP port 67 on `br0`.
4. **01:49:22:** `mervlan_manager.sh boot` executed autonomously:
   - Resolved `blueberry` &rarr; `wl0.2` and `wl1.2`.
   - Created `br20` and `eth0.20`.
   - Evicted `wl0.2` and `wl1.2` from `br0` into `br20`.
   - Preserved `wl0.1` and `wl1.1` (`blueberry-prime`) on `br0`.
5. **01:50:24:** DHCP hold released upon stable observation.
6. **Result:** All IoT clients reconnected directly to `br20` (`10.100.20.x`) with active traffic aging < 8s. **Zero packets or DHCP leases escaped to `br0`.**

### 4.2 Current Live Verification Status
```bash
# Main AP (10.100.1.10)
REPORT hw=RT-AX95Q boot=1 addon=active  event=active cron=present is_node=no  mac_shield=on(51)

# Node AP (10.100.1.11)
REPORT hw=RT-AX95Q boot=1 addon=node-on event=active cron=present is_node=yes mac_shield=on(51)
```
EOF
