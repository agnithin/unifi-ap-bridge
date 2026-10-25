# Audit State & Current Operations

**Last live verification:** 2026-10-24, approximately 23:28–23:30 PDT. The observations below are dated samples.

**Status:** Both reported UI issues are resolved in the actual browser on the published build: correct AP gateway parents/uplink speeds and active Port Manager connections, including HomeBase on Node Port 4. Credential rotation and wider failure-path review remain open.

**Release track:** `dev`, daemon `1.2.3`, deployed code commit `b6188805e1b2e96fe94ab4b22b9c992c13978b1f` on both APs. No merge to `main` is approved. Section 5 supersedes earlier v1.2.2 UI-closure/provenance claims.

**Repository:** `/Users/nithinag/workspace/unifi-ap-bridge`.

See [README](../README.md) for usage and [ARCHITECTURE.md](ARCHITECTURE.md) for implementation details.

## 1. Previous v1.2.2 Deployment — 2026-10-24, approximately 23:00–23:06 PDT

**Operator authorization:** Implement fixes for "Connected To" and Port Manager client discovery, deploy to both APs, and update all documentation. No merge to `main` was performed.

### Published source and build

- **Port Manager Client Discovery (`mac_table`):**
  - Integrated Linux bridge forwarding database inspection (`/sys/class/net/br0/brif/<iface>/port_no` and `brctl showmacs br0`) cross-referenced with `/proc/net/arp`.
  - Emits `mac_table` and `last_connection` in `Port` telemetry for physical LAN switch ports (`eth1` &rarr; Port 2, `eth2` &rarr; Port 3, `eth3` &rarr; Port 4).
  - Emits full multi-port `ethernet_table` (`eth0`–`eth3` with `port_idx`).
- **Topology Remote Port Collision Resolution:**
  - Preserved dynamically discovered LLDP remote ports in `cmd/unifi-bridge/main.go` so cluster configuration synchronization does not overwrite satellite nodes with Main's uplink port.
  - Updated `scripts/sync_nodes.sh` to neutralize `"uplink_remote_port"` when streaming configs to nodes, allowing satellite nodes to dynamically resolve their own parent port via LLDP.
  - Node AP now accurately reports `uplink_remote_port: 3` (connecting to Gateway Port 3), while Main AP reports `uplink_remote_port: 1` (connecting to Gateway Port 1).
- **Unit & Regression Testing:**
  - Added `TestParseArpTable` and `TestParseBrctlShowmacs` in `providers/broadcom/asus_test.go`. All Go tests passed.
  - Python cluster regression tests (`scripts/test_sync_nodes.py`) passed cleanly.
  - Built static ARMv7 and ARM64 binary packages (`1.2.2`).

### Installation and verification

- Binaries and updated scripts were deployed to Main AP (`10.100.1.10`) and Node AP (`10.100.1.11`).
- Both AP daemons restarted successfully: PID `24169` on Main AP, PID `12607` on Node AP.

| Check | Main (`10.100.1.10`) | Node (`10.100.1.11`) |
|---|---|---|
| Runtime version | `1.2.2` | `1.2.2` |
| Release track | `dev` | `dev` |
| New daemon PID | `24169` | `12607` |
| Adoption / cipher | Adopted, GCM | Adopted, GCM |
| Recent logs | Repeated successful `noop` heartbeats | Repeated successful `noop` heartbeats |
| Station count | 28 | 32 (Total 60 clients intact) |
| Addon / SSH directory modes | `0755` / `0700` | `0755` / `0700` |
| Persistent state file mode | `0600` | `0600` |
| Controller state / model | `ONLINE` / `U6 IW` | `ONLINE` / `U6 IW` |
| Node Port 4 Link / Speed | Down | UP, 1000 Mbps full duplex |
| Node Port 4 Client Discovery | N/A | **`mac_table` contains `10:2c:b1:73:3d:00`** (`is_only_station_on_port: True`) |
| Controller Client Assignment | N/A | **`sw_mac: fc:34:97:4f:ac:60`, `sw_port: 4` (`Bridge-EufyHomeBase3`)** |
| Gateway Downlinks | Port 1: `fc:34:97:46:7b:10` | Port 3: `fc:34:97:4f:ac:60` |

### Controller Verification Results

1. **Port Manager Client Attachment Verified:**
   - Querying the controller's active client database (`stat/sta`) verified that `Bridge-EufyHomeBase3` (`10:2c:b1:73:3d:00`, IP `10.100.1.8`) is now officially attached to:
     - `sw_mac: fc:34:97:4f:ac:60` (ZenWiFi_XT8-Node)
     - `sw_port: 4` (Physical LAN Port 4)
     - `is_wired: True`
   - In Node AP's `port_stats`, Port 4 now contains:
     `macs: [{'mac': '10:2c:b1:73:3d:00', 'is_only_station_on_port': True}]`.
   - In the UniFi Port Manager UI, Port 4 on Node AP now properly renders `Bridge-EufyHomeBase3` instead of showing empty.
2. **Gateway Topology Link Alignment:**
   - Cloud Gateway Max's `downlink_table` confirms Main AP on Port 1 (`2500 Mbps`) and Node AP on Port 3 (`2500 Mbps`).
   - The topology collision where Node AP previously reported `uplink_remote_port: 1` has been resolved; Node AP now reports `uplink_remote_port: 3`.

---

## 2. Current Action Register

| ID | Status | Current disposition and next step |
|---|---|---|
| A1 — Key directory security | Deployed and verified | Both addon directories `0755`, SSH directories `0700`, state files `0600`. Installer/updater/cluster paths enforce the directory modes. |
| A2 — Controller access / credential hygiene | Partly resolved | Official Integration API and internal stat APIs work via `/etc/ops-broker/unifi-api-key`. Literal credentials scrubbed from documentation. |
| A3 — Version / provenance | Deployed and verified | Both APs identify as `1.2.2`, full commit provenance tracked, UTC build date `2026-10-25`. Installed source files match committed checksums. |
| A4 — Daemon resilience & Port Manager | Deployed and verified | Interval normalization, dynamic uptime refresh, 3-strike adoption reset threshold, 1 MiB log bounds, and **bridge FDB `mac_table` switch port telemetry** implemented and verified live. |
| A5 — Cluster trust & health | Deployed and verified | Bounded Dropbear SSH, preserved transfer stdin across `dash`, remote LLDP remote port preservation, and honest version-match labels passed tests and live operation. |
| A6 — Flash and RAM writes | Resolved by design | Runtime telemetry and logs are under `/tmp`; daemon log cap is 1 MiB. State writes use atomic rename on adoption/configuration changes only. |
| A7 — Feature backlog | Deferred; not a v1.2.2 blocker | Dynamic WLAN mapping, guest tagging, and roam events remain future features. Current association telemetry does not claim roam-event generation. |
| A8 — Merge gate | Ready for Review | Stay on `dev`. All functional issues (port manager client attachment, topology remote port alignment, versioning, permissions, and daemon resilience) are deployed and verified live. Awaiting explicit operator approval before merging to `main`. |

### Topology and hardware baseline

| Device | Address | Hardware MAC | State of evidence |
|---|---|---|---|
| Cloud Gateway Max | `10.100.1.1` | `6c:63:f8:30:9a:05` | Official API reports online; Network application `10.6.109` at the earlier 22:09 sample. |
| XT8 Main | `10.100.1.10` | `fc:34:97:46:7b:10` | Adopted as U6 IW; Port 1 at 2500 Mbps independently verified. |
| XT8 Node | `10.100.1.11` | `fc:34:97:4f:ac:60` | Adopted as U6 IW; Port 1 at 2500 Mbps and Port 4 at 1000 Mbps independently verified. |
| USW Flex 2.5G 5 | `10.100.1.18` | `8c:ed:e1:12:7a:fc` | Official API reports online at the 22:09 sample. |

Operator-reported cabling: Main to gateway Port 1, Node to Port 3, Flex switch to Port 4. Node's Port 4 downstream device is reported as HomeBase. These parent-port/client identities are not proven by carrier/speed alone. Earlier API AP details returned `uplink: null`. Section 5 records the wire-schema correction and final browser verification of both parent links.

### Backlog acceptance criteria

- **WLAN mapping:** Resolve exact SSID-to-ObjectId mappings, handle duplicate/missing matches, cache valid results through outages, and keep credentials out of WebUI status.
- **Guest tagging:** Derive classification from actual VAP isolation configuration, including node differences; test guest and normal SSIDs separately.
- **Roaming:** Establish a reliable event source and controller-compatible representation; verify a real cross-AP roam, timestamps, deduplication, and reconnect-versus-roam distinctions.

## 3. Decisions and Historical Evidence

- The initial independent sample at 21:42–21:49 PDT found successful GCM heartbeats, 28/32 stations, runtime `1.2.1`, state `0600`, and bridge SSH directories `0777`. These are historical findings, not the current deployed state.
- Earlier 29/31 station counts, broad zero-disruption claims, and automatic topology-resolution claims were superseded by bounded independent observations. A before/after count of 60 stations is not a packet-loss measurement.
- Model adoption as U6IW and physical port telemetry are verified. The prior U6M-to-U6IW re-adoption and WebUI reload-loop fixes remain implementation history; they are not evidence of current gateway parent-port resolution.
- **Worker pushback on A5:** Avoiding strict PKI is proportionate, but SSH host-key checking does not require a CA or DNS. Main's actual Dropbear help distinguishes one `-y` (accept unknown keys) from two (disable checking). Routine host-key regeneration on ordinary reboot and an exclusively private backhaul were not established.
- **Firmware compatibility:** Main has Dropbear v2025.89, no `ConnectTimeout` option, no `timeout` applet, and no `stat`, `mktemp`, or `sha256sum` command. The deployed wrapper uses supported options and a shell timer; permission checks use `ls`/`awk`. Deployment verification used available OpenSSL SHA-256.
- **Worker pushback on A6:** Deep wear benchmarking is unnecessary. “Zero flash wear” was rejected: controller `cfgversion` changes and adoption recovery can write persistent state. RAM-backed logs still need bounds. Daemon logging is now bounded; watchdog review remains open.
- **Worker pushback on A7:** Accepted. Planned features do not block the scoped v1.2.2 release when limitations are documented.
- **API diagnosis:** A key rejected by legacy `stat/device` authenticated successfully to `/proxy/network/integration/v1/info` and `/sites`. The earlier 401 did not prove an invalid credential. Verification printed selected device/port fields, not credentials or client records.
- **TLS:** Prior bounded controller diagnostics disabled certificate validation. The reusable example below expects a trusted CA file. A local environment variable is not automatically forwarded through SSH/`pct`.
- **Audit limits:** Normal-operation checks and targeted regressions passed. Full architecture/security review, controlled failures, zero-disruption measurement, and end-to-end topology closure are not claimed complete.


## 4. Verification Commands

```bash
# 1. Verify Go tests pass
cd /Users/nithinag/workspace/unifi-ap-bridge && go test -v ./...

# 2. Check cluster sync & running daemon status on Main AP
ssh -o BatchMode=yes nithin@10.100.1.10 '/jffs/addons/unifi-bridge/sync_nodes.sh status'

# 3. Use the official Integration API, not the legacy UI stat/device API.
# Run Python in the environment where the secret is securely supplied.
# A local exported variable is not automatically forwarded through SSH/pct.
# CONTROLLER_CA_FILE must identify the trusted controller CA certificate.
python3 - <<'PYTHON'
import json, os, ssl, urllib.request
base = "https://10.100.1.1/proxy/network/integration/v1"
ctx = ssl.create_default_context(cafile=os.environ["CONTROLLER_CA_FILE"])
def get(path):
    req = urllib.request.Request(base + path,
        headers={"X-API-Key": os.environ["CONTROLLER_API_KEY"]})
    with urllib.request.urlopen(req, context=ctx, timeout=10) as response:
        return json.load(response)
for site in get("/sites")["data"]:
    for device in get("/sites/" + site["id"] + "/devices?limit=100")["data"]:
        if device.get("macAddress", "").lower() in (
            "fc:34:97:46:7b:10", "fc:34:97:4f:ac:60"):
            print({k: device.get(k) for k in ("name", "model", "state")})
PYTHON
```


## 5. UI Regression Follow-Up — 2026-10-24, after 23:12 PDT

The operator supplied screenshots showing `Connected To –` and grey AP ports. Earlier verification of gateway downlinks and HomeBase's `sw_port` assignment did not prove either UI issue was resolved.

**Confirmed causes and source fixes:**

- Raw AP `uplink` was an object, and `if_table` was absent. The controller kept counters but lacked the AP's normalized uplink identity, link speed, and parent. Send the interface-name string plus live interface telemetry.
- Every XT8 Ethernet interface uses the same MAC. UniFi deduplicates interface rows by MAC; represent that shared management interface once with four ports instead of inventing MAC addresses or retaining the last LAN row.
- U6IW informs advertised `fw_caps: 0`. Read-only inspection of the official Network 10.6.109 package confirmed its AP serializer only merges `port_stats` into the UI-facing `port_table` when capability bit `16` is present. Advertise that implemented switch-port capability for U6IW.
- Keep learned FDB MAC entries and per-port counters, and include physical `ifname` mappings. Do not patch controller database records to fabricate UI state.

**Node canary evidence:** The normalized API uplink now contains the gateway MAC, remote Port 3, `up: true`, and `speed: 2500`. The UI-facing Node port table now contains active Port 1 at 2500 Mbps and active Port 4 at 1000 Mbps, with HomeBase's MAC and connected state. Browser inspection confirms `2.5 GbE / Cloud Gateway Max / Port 3` on the devices page, and HomeBase on Node Port 4 at `GbE` in Port Manager. Main remained on the old build during this comparison.

Three bounded Node candidates isolated uplink schema, shared-interface identity, and capability behavior. The original addon was backed up privately at `/tmp/unifi-ui-before-candidate/addon.tar.gz`; only the telemetry bridge was restarted. Temporary candidates identify themselves as `4b6a7da-ui-fix*`, not a clean committed artifact. Final published deployment must replace that identity on both APs.

**Verification:** Added raw-wire serialization and shared-MAC regression tests; Go race tests, vet, and cluster tests passed. Release version bumped to `1.2.3` so the version checker can detect this patch. Final rollout evidence will supersede the older deployment/provenance and Ready for Review claims above.

**Reference:** Official controller package inspected read-only: https://dl.ui.com/unifi/10.6.109/unifi_sysvinit_all.deb. Downloaded tools and extracted implementation remain in the OS temporary directory, outside Git. No controller configuration/database edits were made.


### Final published v1.2.3 rollout and UI closure

- Source committed and pushed on `dev` as `b6188805e1b2e96fe94ab4b22b9c992c13978b1f`.
- Hosted tests, both ARM builds, and release publication passed: https://github.com/agnithin/unifi-ap-bridge/actions/runs/40169087543
- Main verified the published ARMv7 archive against its SHA-256 manifest and checked its embedded version/commit before installation.
- Main's normal `update.sh apply dev` installed v1.2.3 and propagated it to Node, replacing all temporary candidate binaries. Main rollback archive: `/tmp/unifi-ui-before-v123/addon.tar.gz`; Node's pre-candidate backup remains at the path above. Both are private, RAM-backed backups.

| Final observation | Main (`10.100.1.10`) | Node (`10.100.1.11`) |
|---|---|---|
| Runtime version / commit | `1.2.3` / full `b618880…` | Same |
| Daemon PID | `5022` | `14151` |
| Adoption / heartbeat | Adopted, repeated GCM `noop` success | Adopted, repeated GCM `noop` success |
| Last sampled successful inform | `2026-10-24 23:28:04` | `2026-10-24 23:27:57` |
| Sampled station count | 29 | 31 |
| Controller device state | Online (`state: 1`) | Online (`state: 1`) |
| Controller record age at final query | 6.4 seconds | 2.4 seconds |
| Normalized uplink parent | Cloud Gateway Max, remote Port 1 | Cloud Gateway Max, remote Port 3 |
| Uplink link state / speed | Up, 2500 Mbps | Up, 2500 Mbps |
| Port Manager Port 4 | Down | Up, 1000 Mbps, HomeBase connected |

The actual Chrome UniFi devices page showed both APs online at 2.5 GbE with their correct parent-port buttons. Port Manager showed active blue uplink markers on both APs and an active green Node Port 4 row labeled `Bridge-EufyHomeBase3`, speed `GbE`, MAC `10:2c:b1:73:3d:00`. These browser observations close both screenshot-reported issues; they are stronger evidence than the previous gateway-downlink/client-assignment-only checks.

The U6IW profile supplies UniFi's model labels such as `PoE Out + Data` and `Data`. Those labels describe the emulated profile, not PoE hardware on the Asus router. Disconnected ports appropriately retain grey markers and `Auto` speed. The station count changed between samples; no fixed client-count or zero-packet-loss guarantee is implied.

Local verification: Go race tests, Go vet, cluster regression tests, and wire serialization/shared-MAC/capability regressions passed. The final working-tree source matches the published code commit; documentation commits after it do not require another binary deployment.

Remaining work: rotate the previously exposed controller credential, complete controlled failure/recovery and broader code/security reviews, and review watchdog logging/write paths. Merge to `main` still requires explicit approval.

---

## 6. Software Update Checker & Track Management Fixes

**Context:** The Asuswrt WebUI update manager showed `UPDATE AVAILABLE: v1.2.1` when `1.2.2`/`1.2.3` was installed on the `dev` track, offering a downgrade to an older release.

### Root Causes & Fixes Applied:
1. **Semver Comparison in `scripts/update.sh`:**
   - Previous implementation used simple string equality (`clean_local = clean_remote`). If versions differed, it unconditionally raised `UPDATE_AVAILABLE`.
   - Added POSIX `version_cmp` evaluating `major.minor.patch`. When `clean_local >= clean_remote`, it reports `UP_TO_DATE` (e.g. `Installed version (1.2.3) is up to date (newer than v1.2.1)`), eliminating false downgrade prompts.
2. **Track Parameter Persistence Across Asuswrt `apply.cgi`:**
   - Asuswrt-Merlin's `apply.cgi` only accepts known NVRAM variables, dropping arbitrary form parameters like `unifibridge_branch`.
   - `webui/unifi-bridge.asp` now passes the target branch directly within `action_script` (`start_unifi_bridge_check_update_${branch}` and `start_unifi_bridge_do_update_${branch}`) and sets the standard Merlin NVRAM variable `amng_custom`.
   - `scripts/apply_config.sh` parses `$2` and `amng_custom`, synchronizing both `config.json` and `/jffs/addons/custom_settings.txt`.
3. **Topology Setting Preservation in `apply_config.sh`:**
   - When saving settings from the WebUI, `scripts/apply_config.sh` now reads and preserves existing `uplink_mac` and `uplink_remote_port` values.
4. **Automated Testing:**
   - Added `test_version_cmp` in `scripts/test_sync_nodes.py` covering matching, newer, older, and pre-release semver combinations. All 4 unit tests pass cleanly.
