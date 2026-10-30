# UniFi AP Bridge — Operational State & Status

> **Current Delivery Status (October 2026):**
> - **Main AP (`10.100.1.10`):** Running commit `d9b6674` (v1.2.8 dev).
> - **Node AP (`10.100.1.11`):** Running commit `d9b6674` (v1.2.8 dev).
> - **Published dev Release:** `dev-c0dd00fe26d7cd0ac4f6e0fe7bb7ce2b28088ad0` ([Release Assets](https://github.com/agnithin/unifi-ap-bridge/releases)).
> - **Controller Integration:** UniFi Cloud Gateway Max (`10.100.1.1`), Network Application 10.6.109. Both APs adopted as U6IW via GCM encryption.
> - **Historical Evidence:** Detailed day-by-day deployment logs from October 24–28, 2026 are archived in [docs/archive/2026-10-28-audit-state-history.md](archive/2026-10-28-audit-state-history.md). Independent review report is in [docs/audit/2026-10-29-unifi-ap-bridge-homelab-review.md](audit/2026-10-29-unifi-ap-bridge-homelab-review.md).

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
The bridge caches SSID-to-WLAN ObjectId mappings at `/jffs/addons/unifi-bridge/wlan_mapping.json`.
- The cache merges valid partial updates into previous entries and retains last-good lookups during transient network outages.
- If an SSID is renamed, deleted, or reassigned in the UniFi controller, remove the cached file on both APs to trigger a clean re-resolution:
  ```bash
  rm -f /jffs/addons/unifi-bridge/wlan_mapping.json
  ```
- The next periodic sample or daemon restart will query the trusted controller host helper and construct a fresh mapping file atomically.

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

## 4. Next Deployment Actions (When On Local Network)

1. **Verify Staged Build:** Check published dev bundle containing Locate worker completion synchronization, station departure guards, JSON projection resilience, and static fork reduction.
2. **Canary Rollout to Node AP:** Update satellite node (`10.100.1.11`) first via `/jffs/addons/unifi-bridge/update.sh apply dev`, verify clean adoption and station counters.
3. **Rollout to Main AP:** Update Main router (`10.100.1.10`), verify cluster synchronization panel in WebUI and UniFi Controller topology.
