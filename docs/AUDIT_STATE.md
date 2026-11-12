# UniFi AP Bridge — Operational State & Status

> **Current delivery status — 2026-11-12 PST:**
> - **Main and Node:** Both APs actively running `v1.2.13` (Git `ce0ca58`), adopted and `Active (GCM)`, with fresh successful informs and zero sample-stage errors.
> - **Modern UniFi RFC 4122 UUID Support (F8):** Fully compliant with UniFi Network 11 UUID WLAN identifiers; controller UUID validation warnings permanently eliminated.
> - **Automated WLAN Mapping Sync (F8):** `watchdog.sh` polls `http://10.100.1.107:8000/api/v1/network/aps/wlan-mapping` every 5 minutes, validates, compares via `cmp -s`, and replaces cache atomically only on change (zero flash wear when unchanged).
> - **Hardening Delivered:** Includes F1 (bounded timeouts, dynamic interface detection), F2 (filter snapshot/rollback on failure), F3 (formalized provisioning boundary & loop avoidance), F4 (watchdog stale lock eviction), F5 (non-destructive transport retries), F6 (atomic sync durability), F7 (radio liveness probe), F8 (UUID WLAN mapping + automated pull), and F9 (natural 802.11 roaming).
> - **Controller:** Cloud Gateway Max (`10.100.1.1`). Both APs report adopted GCM status.
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

### WLAN Mapping Cache Invalidation & Automated Refresh
The cache is stored at `/jffs/addons/unifi-bridge/config/wlan-mapping.json` (mode 0600).
- **Automated Pull (Primary):** When `"wlan_mapping_url"` is configured in `config.json` (e.g., `http://10.100.1.107:8000/api/v1/network/aps/wlan-mapping`), `watchdog.sh` polls the endpoint every 5 minutes via cron. It compares the downloaded payload against the active cache (`cmp -s`), updating atomically via rename only when mappings change (zero flash wear when unchanged).
- **On-Demand Push (Fallback):** The Mac helper `python3 -B scripts/ops/mac_wlan_mapping.py` remains available to manually push exact SSIDs to Main and Node without exposing controller credentials outside CT107.
- **Merge Behavior:** Published entries merge into the running last-known-good map, so removing an SSID from the file does not necessarily remove it from memory until a reviewed restart. Do not delete a different cache path or assume the daemon will query the controller directly.

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

1. **ASUS / MerVLAN Owns Wi-Fi Radio Operation & 802.1Q VLANs (Finding F3):** Broadcom FullMAC firmware handles 802.11 beacons, channel switching, association states, and client frames. MerVLAN handles trunking and virtual AP tagging. The bridge daemon is strictly an observational telemetry provider; it does not reflash firmware, overwrite NVRAM radio configs, or perform continuous disruptive RF spectrum sweeps.
2. **Provisioning Loop Avoidance via `cfgversion` Echoing (Finding F3):** UniFi controller inform responses include a `cfgversion` token whenever site wireless settings change. The bridge echoes `cfgversion` in subsequent inform heartbeats (`stateMgr.GetCfgVersion()`), satisfying UniFi's internal state machine that the AP is converged. Without this acknowledgment, UniFi enters an infinite loop of attempting to provision the AP ("Provisioning..."). Destructive incoming configuration payloads (e.g. system configs or interface restructuring) are intentionally disregarded to protect router integrity.
3. **Hardware & Interface Invariant:** The daemon never executes destructive PHY resets (`ethctl phy-reset` or `ifconfig eth0 down` are strictly prohibited) to prevent Broadcom SerDes traffic blackholes.
4. **Read-Only Non-Disruptive Diagnostics:** Locate LED blinking and private interface frame captures are bounded in duration (<= 15 seconds) and run with deterministic cleanup to prevent resource exhaustion.
5. **Hardware Process-Fork Bounds:** Static hardware identifiers (`lan_hwaddr`, `et0macaddr`), antenna chains (`txchain`), and VAP metadata (`ssid`, `bssid`, `wlX_bss_enabled`) are cached in memory with a 5-minute TTL, eliminating 25–30 process forks per 10-second collection cycle.

---

## 4. Completed Architecture & Hardening Status

- **Lock to AP Architecture (Finding F9):** Evaluated and confirmed Option 3 (natural 802.11 roaming, zero maintenance, zero outage risk). The UniFi toggle remains visible without reverse API driver scraping.
- **Filter Safety Hardening (F1 & F2):** Snapshot/rollback and driver readback verification implemented in `cmd/unifi-bridge/controller_commands.go` and verified by `TestExecutePlatformMacFilterRollbackOnFailure`.
- **Radio Liveness Probe (F7):** Implemented `wl isup` validation in `providers/broadcom/asus.go` so inactive radios are cleanly skipped during telemetry collection; verified by `TestGetRadios_SkipsDownInterfacesWhenIsupIsZero`.
- **State and Recovery Hardening (F4, F5, F6):** Generational backup `state.json.bak`, directory sync, and stale lock eviction (>10m) verified.
- **Product Scope & Provisioning Boundary (F3):** Formally documented division of ownership: UniFi owns topology, client telemetry, port statistics, and locate; Asuswrt/MerVLAN owns physical radios, SSIDs, WPA credentials, and 802.1Q VLANs. Infinite provisioning loop eliminated via deterministic `cfgversion` echoing.
- **WLAN Mapping Automation (F8):** Modern RFC 4122 UUID support in `v1.2.13`. Automated 5-minute watchdog pull from CT107 Ops Broker (`/api/v1/network/aps/wlan-mapping`) active on both APs with atomic zero-wear `cmp -s` updates.

### Session Handoff Summary
- **Current Live Deployment:** `v1.2.13` (commit `ce0ca58`), adopted and active GCM on both APs (`10.100.1.10` and `10.100.1.11`).
- **Live Cleanup Completed:** Verified all wireless interfaces on both APs have `macmode: 0` (clean unhindered Broadcom operation).
- **All Phase 1 & Phase 2 Hardening Deployed:** Filter snapshot/rollback (F2), radio `isup` probe (F7), watchdog stale lock eviction (F4), state sync durability (F6), transport error safety (F5), UUID WLAN mapping with automated sync (F8), and formalized provisioning boundary (F3). All Go and Python tests passing 100%.
