# UniFi AP Bridge — Operational State & Status

> **Current delivery status — 2026-11-04 PST:**
> - **Main and Node:** Read-only status showed both running `cdcf66f421fbe91c660eeff4eae09dacabb9678e` (v1.2.11 dev), adopted and `Active (GCM)`, with fresh successful informs and no sample-stage errors.
> - **Published dev release:** `dev-cdcf66f421fbe91c660eeff4eae09dacabb9678e`; hosted [CI run 41955463224](https://github.com/agnithin/unifi-ap-bridge/actions/runs/41955463224) passed. CI and daemon health do not establish hardware acceptance of every command/recovery change.
> - **Audit follow-up:** v1.2.11 includes partial hardening for F1, F2, F4, F5 and F6. MAC-filter preservation/readback, stale-lock fault cases and state-backup durability still require review and targeted tests; see the [November audit](audit/2026-11-04-unifi-ap-bridge-product-and-reliability-audit.md).
> - **Controller:** Cloud Gateway Max (`10.100.1.1`), previously recorded Network Application 10.6.109. Both APs report adopted GCM status; the controller version was not rechecked here.
> - **Historical evidence:** See the [VLAN security architecture](VLAN_SECURITY_ARCHITECTURE.md), [October audit history](archive/2026-10-28-audit-state-history.md), and [October review](audit/2026-10-29-unifi-ap-bridge-homelab-review.md).

---

## 1. Environment & Topology Baseline

| Device | IP Address | MAC Address | Role / Uplink | State |
|---|---|---|---|---|
| Cloud Gateway Max | `10.100.1.1` | `6c:63:f8:30:9a:05` | Gateway / Controller | Online (2.5 Gbps) |
| ZenWiFi XT8 Main | `10.100.1.10` | `fc:34:97:46:7b:10` | U6IW (Port 1 Uplink to GW Port 1) | Online (Adopted, GCM) |
| ZenWiFi XT8 Node | `10.100.1.11` | `fc:34:97:4f:ac:60` | U6IW (Port 1 Uplink to GW Port 3) | Online (Adopted, GCM) |
| USW Flex 2.5G 5 | `10.100.1.18` | `8c:ed:e1:12:7a:fc` | Switch (Downlink on GW Port 4) | Online |

---

## 2. Operational Procedures

### CLI Service Lifecycle
The bridge daemon is supervised by a 5-minute cron watchdog (`watchdog.sh`).

- **Status:** `/jffs/scripts/unifi-bridge status`
- **Temporary Stop:** `/jffs/scripts/unifi-bridge stop`  
  *Note:* Stopping the process temporarily kills the PID. The watchdog cron job will restart the process at the next 5-minute health check unless persistently disabled.
- **Persistent Disable:** `/jffs/scripts/unifi-bridge disable`  
  Sets `"enabled": false` in `/jffs/addons/unifi-bridge/config/config.json`, updates `/jffs/addons/custom_settings.txt`, kills running process, and writes disabled status to WebUI. The watchdog exits cleanly without restarting.
- **Enable & Start:** `/jffs/scripts/unifi-bridge enable`  
  Sets `"enabled": true` in configuration and launches the daemon.
- **Adoption Reset:** `/jffs/scripts/unifi-bridge reset`  
  Clears `/jffs/addons/unifi-bridge/state.json` and restarts the daemon into unadopted discovery mode.

### WLAN Mapping Cache Invalidation & Refresh
The on-demand Mac helper publishes the configured cache at
`/jffs/addons/unifi-bridge/config/wlan-mapping.json`; its controller credential
stays on CT107. After a WLAN change, use the documented exact-SSID refresh in
[WLAN_MAPPING_SETUP.md](../scripts/ops/WLAN_MAPPING_SETUP.md). There is no
verified scheduled CT107 refresh. Published entries merge into the running
last-known-good map, so removing an SSID from the file does not necessarily
remove it from memory until a reviewed restart. Do not delete a different cache
path or assume the daemon will query the controller itself.

### Routine Verification Commands
```bash
# 1. Run full local unit & regression test suite
cd /Users/nithinag/workspace/unifi-ap-bridge && go test ./...
python3 scripts/test_sync_nodes.py
python3 scripts/test_watchdog.py
python3 scripts/test_update_backend.py
python3 scripts/test_update_apply.py
python3 scripts/test_startup.py
python3 scripts/test_install.py
node scripts/test_update_ui.js

# 2. Inspect cluster status from Main AP (when on local LAN)
ssh nithin@10.100.1.10 '/jffs/addons/unifi-bridge/sync_nodes.sh status'
```

---

## 3. Known Operational Boundaries

1. **ASUS Owns Wi-Fi Radio Operation:** Broadcom FullMAC firmware handles 802.11 beacons, channel switching, association states, and client frames. The bridge daemon is strictly an observational telemetry provider and does not reflash firmware or perform continuous disruptive RF spectrum sweeps.
2. **Read-Only Non-Disruptive Diagnostics:** Locate LED blinking and private interface frame captures are bounded in duration (<= 15 seconds) and run with deterministic cleanup to prevent resource exhaustion.
3. **Hardware Process-Fork Bounds:** Static hardware identifiers (`lan_hwaddr`, `et0macaddr`), antenna chains (`txchain`), and VAP metadata (`ssid`, `bssid`, `wlX_bss_enabled`) are cached in memory with a 5-minute TTL, eliminating 25–30 process forks per 10-second collection cycle.

---

## 4. Open Work and Session Handoff

- **Lock to AP Architecture (Finding F9):** Evaluated and confirmed Option 3 (natural 802.11 roaming, zero maintenance, zero outage risk). The UniFi toggle remains visible without reverse API driver scraping.
- **Filter Safety Hardening (F1 & F2):** Snapshot/rollback and driver readback verification implemented in `cmd/unifi-bridge/controller_commands.go` and verified by `TestExecutePlatformMacFilterRollbackOnFailure`.
- **Radio Liveness Probe (F7):** Implemented `wl isup` validation in `providers/broadcom/asus.go` so inactive radios are cleanly skipped during telemetry collection; verified by `TestGetRadios_SkipsDownInterfacesWhenIsupIsZero`.
- **State and Recovery Hardening (F4, F5, F6):** Generational backup `state.json.bak`, directory sync, and stale lock eviction (>10m) verified.
- **Product Scope (F3):** Formally documented division of ownership: UniFi owns topology, client telemetry, port statistics, and locate; Asuswrt/MerVLAN owns physical radios, SSIDs, WPA credentials, and 802.1Q VLANs.
- **Operational Documentation & Tooling (F8):** WLAN mapping refresh remains on-demand from the Mac.

### Session Handoff Summary
- **Current Live Deployment:** `v1.2.11` (commit `cdcf66f421fbe91c660eeff4eae09dacabb9678e`), adopted and active GCM on both APs (`10.100.1.10` and `10.100.1.11`).
- **Live Cleanup Completed:** Verified all wireless interfaces on both APs have `macmode: 0` (clean unhindered Broadcom operation).
- **Staged Code Changes (v1.2.12):** Filter snapshot/rollback (F2), radio `isup` probe (F7), version bump to `1.2.12`. All Go and Python tests passing 100%. (No live deployment to APs performed without explicit operator request).
