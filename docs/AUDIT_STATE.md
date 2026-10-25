# Audit State & Current Operations

**Last live verification:** 2026-10-25, approximately 01:50–01:55 PDT. Section 7 is the current code audit and prioritized backlog; earlier sections retain dated rollout evidence.

**Status:** Both APs remain healthy on v1.2.4 dev. Prior browser fixes remain recorded in Sections 5–6. The new review found update integrity, configuration preservation, disabled-state propagation, and tri-band radio reporting gaps; see Section 7. Controller credential rotation is complete (Section 8); controlled outage testing remains open.

**Release track:** `dev`, daemon `1.2.4`, deployed code commit `f77fafbc08688faca4b0fd9a286b73790d5a25da` on both APs. Promotion of all current changes to `main` was approved by the operator on 2026-10-25. Section 5 supersedes earlier v1.2.2 UI-closure/provenance claims.

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
| A2 — Controller access / credential hygiene | Rotated and verified | Replacement installed at `/etc/ops-broker/unifi-api-key` (0600) on CT107; exposed key revoked. On 2026-10-25 at 16:07 UTC: old key HTTP 401, replacement info/sites HTTP 200; broker active/running. Historical exposure retained as evidence. |
| A3 — Version / provenance | Runtime verified; release packaging open | Both APs run `1.2.4` / `f77fafb…` on dev. Main was promoted to `c8ebaa4…`; stable release remains v1.2.1. See B1–B2. |
| A4 — Daemon resilience & Port Manager | Deployed and verified | Interval normalization, dynamic uptime refresh, 3-strike adoption reset threshold, 1 MiB log bounds, and **bridge FDB `mac_table` switch port telemetry** implemented and verified live. |
| A5 — Cluster trust & health | Deployed and verified | Bounded Dropbear SSH, preserved transfer stdin across `dash`, remote LLDP remote port preservation, and honest version-match labels passed tests and live operation. |
| A6 — Flash and RAM writes | Partly resolved | Runtime telemetry and daemon logs use `/tmp`; daemon log cap is 1 MiB. State saves use atomic rename. Watchdog log bounds and recovery behavior remain open (B7). |
| A7 — Feature backlog | Deferred; not a v1.2.2 blocker | Dynamic WLAN mapping, guest tagging, and roam events remain future features. Current association telemetry does not claim roam-event generation. |
| A8 — Merge gate | Completed | Operator approved promotion; main and dev were pushed to `c8ebaa4…`. This did not publish a stable tag or change AP tracks. Newly found gaps remain open in Section 7. |

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

Remaining work: rotate the previously exposed controller credential, complete controlled failure/recovery and broader code/security reviews, and review watchdog logging/write paths. The operator approved promotion to `main` on 2026-10-25; these remaining audit tasks stay open.

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
   - `scripts/apply_config.sh` parses `$2` and `amng_custom`, dispatching the selected operation. In v1.2.4, checking a track does not persist a track change.
3. **Topology Setting Preservation in `apply_config.sh`:**
   - When saving settings from the WebUI, `scripts/apply_config.sh` now reads and preserves existing `uplink_mac` and `uplink_remote_port` values.
4. **Automated Testing:**
   - Added `test_version_cmp` in `scripts/test_sync_nodes.py` covering matching, newer, older, and pre-release semver combinations. All 4 unit tests pass cleanly.

### v1.2.4 update-check overlay and version labels

The native hidden-iframe submit executed firmware loading callbacks during a background update check, leaving the global Complete overlay visible. Update operations now use background XHR, correlate status by request ID/operation/track, and stop polling on completion, error, or timeout. A stale installation SUCCESS cannot reload a check. Version labels consistently show `v` and the installed track; checking main keeps an installed dev build labeled dev and explicitly reports when stable is older.

Candidate verification on Main in the signed-in Asus browser: dev check completed with UP TO DATE and an enabled check button; main comparison completed with INSTALLED IS NEWER, installed `v1.2.3 (dev)`, latest `v1.2.1 (main)`, and the firmware overlay hidden. JavaScript regressions and four Python backend tests passed. Published rollout verification follows below.

#### Published rollout verified — 2026-10-25, approximately 01:45 PDT

- Hosted release run [40177753409](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40177753409) passed Go tests, cluster tests, update backend/UI regressions, and shell syntax checks. Published ARMv7 archive SHA-256 matched its manifest; binary version and full source commit were verified before rollout.
- Main `update.sh apply dev` installed v1.2.4 and propagated it to Node. Both report commit `f77fafbc08688faca4b0fd9a286b73790d5a25da`, branch dev, running/adopted, Active (GCM). Main PID 13260 / 31 clients; Node PID 17053 / 29 clients. Successful inform samples: Main 01:45:30, Node 01:45:34.
- Published Main Asus UI check: installed `v1.2.4 (dev)`, latest `v1.2.1 (main)`, badge INSTALLED IS NEWER, check button enabled, firmware overlay hidden. Dev check then completed with UP TO DATE and latest `v1.2.4 (dev)`. Selector restored to dev. Browser validation was on Main; Node runtime deployment was verified by SSH.
- Private RAM-backed rollback archives on each AP: `/tmp/unifi-update-before-v124/addon.tar.gz`. Main archive contains the already-tested candidate UI/scripts and previous v1.2.3 daemon; its original UI/script backup also remains at `/tmp/unifi-update-ui-before-fix/assets.tar.gz`.
- Existing open audit work remains open. No merge to main was performed.

### Main promotion authorization — 2026-10-25

The operator requested pushing all current changes to main. Promote the complete tested dev history by fast-forward, preserving prior commits. This supersedes earlier restrictions on merging to main. This repository promotion does not change the APs from their verified v1.2.4 dev deployment or create a stable release tag.


## 7. Code Re-audit and Native UniFi Feature Gaps — 2026-10-25

**Reviewed source:** `c8ebaa49255778425b4a23353f83adea54b43d12` (main and dev at audit start). Reviewed daemon loop, inform state/client/wire handling, Broadcom extraction, telemetry schema, installer/startup/watchdog, updater, config/cluster scripts, WebUI, CI, and existing regressions. This is a scoped engineering review, not an exhaustive security certification.

### Current live evidence and limits

- Main and Node both run v1.2.4, commit `f77fafbc08688faca4b0fd9a286b73790d5a25da`, dev; PIDs 13260 / 17053, adopted, Active (GCM). Status age at the first sample: 3 / 9 seconds. Successful informs: 01:50:40 / 01:50:34. Client counts: 31 / 29.
- Both uplinks are 2500 Mbps full duplex. Node port 4 is 1000 Mbps full duplex and has the expected HomeBase FDB entry. This audit did not change radio/network settings, restart services, induce an outage, or exercise client-disrupting controller commands.
- Addon directories 0755; SSH directories 0700; state and configuration files 0600. Main's cluster key is 0600. Node has no `cluster_key` at that path; a satellite does not require Main's private identity merely to accept SSH.
- Daemon logs were 3849 bytes on each AP; Main watchdog log 165 bytes. Small current logs do not establish a bound on every path.
- Both APs report `eth4`: channel 6 / 20 MHz; `eth5`: channel 36 / 40 MHz; `eth6`: channel 161 / 80 MHz. Active client sampling found Main: 24 on wl0.1, 3 on wl1.1, 4 on eth6; Node: 18 on wl0.2, 1 on wl1.1, 7 on wl1.2, 3 on wl2.1. Counts are snapshots, not packet-loss measurements.
- GitHub currently identifies v1.2.1 as latest stable. No v1.2.4 stable tag was created by the main promotion.
- A fresh UniFi browser inspection was blocked by its certificate warning; it was not bypassed. UI gap assessment below combines source behavior, prior browser-verified topology/Port Manager fixes, and current vendor documentation. Do not interpret this as fresh screenshots of every UniFi panel.

### Findings and actionable acceptance criteria

P1 = address before expanding features or recommending the stable installer/update path. P2 = follow-up correctness, diagnostics, or maintainability improvement. No live emergency or demonstrated compromise was established.

| ID / priority | Finding and evidence | Action / completion criterion |
|---|---|---|
| B1 / P1 | **Stable updates mix source generations.** `scripts/update.sh:207–239` downloads scripts/version from mutable main but binary from latest stable, currently v1.2.1. Main source now says 1.2.4. Version checking follows stable tags, so a dev build correctly reports newer, but an actual main install can mix new scripts with an old daemon. | Publish a reviewed stable release and package scripts, startup, WebUI, version, binary, and manifest from one immutable ref. Verify runtime commit/version matches the selected release. Merely merging main does not complete this. |
| B2 / P1 | **False update success, no enforced checksum or rollback.** `scripts/update.sh:243–287` keeps the old binary if downloads fail and ignores startup failure before SUCCESS. Cluster rollout runs asynchronously after local success; remote extraction uses semicolon-separated operations (`sync_nodes.sh:190`). | Require every artifact and checksum; validate archive/architecture and staged binary; fail on copy/extract/start errors; stage atomically, lock updates, preserve rollback, then wait for a new PID, correct runtime commit, and successful inform. Report local and per-node outcomes separately. Include failure/rollback tests. |
| B3 / P1 | **Saving settings loses configuration.** `scripts/apply_config.sh:160–181` claims to retain WLAN mappings but always writes hardcoded homelab ObjectIds; it also rewrites the whole object without preserving explicit `nodes`. | Merge only submitted known fields into validated existing config, preserve WLAN mappings/nodes/other supported fields and node-specific overrides, write atomically with 0600, and leave the old file intact on invalid input. Test custom mappings and explicit node lists. Remove homelab-specific defaults from generic installs. |
| B4 / P1 | **Disable does not reliably survive cluster sync or reboot.** `apply_config.sh:185–205` stops Main then syncs; `sync_nodes.sh:200` always invokes start.sh. The startup template (`install.sh:144–205`) and daemon Config do not enforce enabled=false. Watchdog skips disabled configs but does not undo a daemon started elsewhere. Static-path finding; not exercised against live APs. | Centralize enabled-state enforcement in a tracked startup script, distribute its updates to existing installs, and make sync/update/boot/watchdog agree. Test disable across both nodes, reboot/start simulation, and deliberate re-enable. |
| B5 / P1 | **Tri-band clients mapped to one 5 GHz radio; width ignored.** `asus.go:100–158,363–390` excludes eth5 while active wl1.* clients use it. Both 5 GHz VAP groups use wifi-na; counting in main.go combines them against eth6 channel/airtime. Parsed width is discarded and 5 GHz HT is fixed at 80. Live channels/client counts above and isolated provider probes confirm this. | Discover physical radios and enabled VAPs, retain parent-radio identity, width and streams; count each VAP only on its parent. Validate a third radio against the emulated model/controller schema before promising native tri-band UI support; document a representational limit if necessary. Test simultaneous 40/80 MHz radios and VAP churn. |
| B6 / P1, resolved 2026-10-25 | **Previously exposed controller credential rotation remains unresolved in the ledger.** This audit did not print credentials or independently establish a completed rotation. Existing permissions are good; they do not revoke an exposed credential. | Completed in Section 8: replacement file/API verification, old-key revocation and HTTP 401 rejection verified; broker service active. Continue keeping discovery credentials off the AP. |
| B7 / P2 | **Health is stronger than before but watchdog only checks PID.** `watchdog.sh` cannot recover a live process stuck in sampling and appends without a size cap. A provider call has a 2-second timeout, but many serial calls have no shared sampling deadline or shutdown cancellation (`asus.go:45–60`, main loop). | Measure sample duration/subprocess count first; cache stable metadata and reuse radio/LLDP results, bound total sampling, propagate context, expose stage timing/errors. Failed interface queries must be distinguishable from zero clients/down links; retain last-good samples with freshness rather than manufacturing empty healthy data. Watchdog should detect stale sampling with restart backoff and bounded logs; controller outages alone should not cause restart storms. |
| B8 / P2 | **Some displayed metrics are estimates presented as measurements.** CPU is load1/cores (`asus.go:931`), not CPU busy percentage. Noise defaults to -90; Wi-Fi experience can become 100 with missing signal/counters and uses cumulative retries (`sta_info.go:53,212`). Station-summed VAP counters fall when clients leave. | Use /proc/stat deltas for CPU; retain load separately; prefer MemAvailable when present. Mark missing measurements unknown, use measured noise when available, calculate interval retry quality with reset handling, and use stable per-interface VAP counters. Describe experience as a bridge heuristic, not vendor-equivalent scoring. |
| B9 / P2 | **Topology/FDB fallbacks fabricate certainty.** Unknown LLDP port defaults to 1 (`asus.go:525,734`), preventing configured fallback in main.go. FDB age is time since learning/refresh, not connection duration; last_connection is overwritten by each MAC (`asus.go:618–684`). VLAN is fixed at 1, STP is synthesized from carrier. | Preserve unknown port as 0; validate LLDP subtype/interface and explicit port formats; retain last verified parent through short failures with freshness/source metadata. Separate FDB last-seen from actual connection history, support multiple clients, actual VLAN/bridge membership and STP state, and clear stale attachments on link loss. |
| B10 / P2 | **UI health and cluster state can mislead.** `webui:377` treats only a string containing In Sync as healthy while producer emits Local / Version Match / Config Synced; config hash is never verified. Direct status UI trusts running=true without checking age. Sync/settings still use the native iframe flow (`webui:597–658`) fixed only for update actions. | Use structured fields for reachability, freshness, version/commit, config verification, and last successful inform; keep unverified distinct from failure. Reuse the tested background transport for sync/settings, add bounded completion, render node names as text, and serialize/escape shell-generated status safely. Test stale status and unknown/unverified states. |
| B11 / P2 / major capability gap | **Adoption does not mean controller configuration was applied.** `pkg/inform/state.go:147–222` stores auth/cfgversion and recognizes set-adopt/reset/noop; it does not apply radio, SSID, VLAN, port, restart, locate, reconnect, or firmware commands. cfgversion is recorded without enforcing WLAN/radio configuration. | First publish an accurate support matrix and diagnostics for unsupported command types without logging secrets. If adding control, implement a small typed allowlist, validate hardware capability, apply with read-back/rollback, and acknowledge only verified settings. Never execute arbitrary controller strings or claim provisioning parity. |

**Deterministic checks:** Existing `go test -race ./...`, `go vet ./...`, four cluster tests, four update backend tests, UI JavaScript regressions, and all shell syntax checks passed. Temporary isolated probes (no AP mutations) reproduced: (1) both binary downloads fail and start.sh exits 42, yet updater returns 0 and emits success/up-to-date; (2) saving a custom WLAN mapping and explicit nodes discards both while local enabled=false was retained using the portable settings-file path; (3) three input radios produce two advertised radios, and actual 36l/40 MHz yields HT=80. These probes reveal missing coverage; passing existing tests does not invalidate the findings. Temporary fixtures live outside the repository and contain synthetic configuration only.

### What native UniFi AP integration provides beyond this bridge

The current bridge supplies useful monitoring: adoption/online state, station association/rates/signal, heuristic experience, partial RF utilization, Ethernet links/counters, FDB clients, and gateway topology. Asus/Merlin/MerVLAN remains responsible for actual wireless/network configuration. A function absent from the bridge may still exist in Asus firmware; this review does not establish that the Asus hardware lacks it.

| UniFi surface / capability | Current bridge gap | Sensible scope |
|---|---|---|
| WiFi settings and AP radio settings | UniFi SSID/password/broadcasting-AP, channel/width/power, security, schedules and roaming options are not applied to Asus. | Keep Asus as the configuration authority initially; consider a small opt-in control subset after B1–B5. Do not present ignored settings as applied. |
| Guest/VLAN/policy management | is_guest=false throughout; no actual VLAN classification or controller policy enforcement. Existing VLAN/isolation configuration is not audited here. | Read effective per-VAP bridge/VLAN/isolation metadata first. Guest tagging is reporting, not enforcement. Controller guest portals, PPSK, speed limits and policies need separate firmware/data-path implementation and validation. |
| Client history and roaming | Current snapshots lack explicit connect/disconnect/roam reasons, authentication/DHCP failure events, and cross-AP correlation. | Build event-backed association history and deduplicated cross-AP roam inference with confidence/timestamps; prove one real roam. Polling every 10 seconds cannot establish every brief roam or authentication failure. |
| AirView / Environment / diagnostics | Channel airtime exists; neighbor surveys, interference breakdown, detailed retries, scan workflow and capture integration do not. | Start with accurate per-radio passive statistics and last-seen neighbor data; on-demand scans/captures require supported firmware commands and disruption testing. A full proprietary AirView/Channel AI implementation is not established. |
| Device/client actions and port profiles | Locate/reboot/reconnect/radio changes and VLAN/port configuration from UniFi are not implemented. Port Manager currently reports links/attachments. | Optional typed command handlers one at a time, with read-back, permission boundaries and controlled tests; leave unsupported controls documented. |
| Lifecycle and recovery | Bridge-specific GitHub update path, heuristic cluster status and manual rollback; no native UniFi firmware lifecycle parity. | Immutable release bundles, per-node outcome/commit reporting, configuration verification and tested rollback yield more value than cosmetic capability flags. |
| Physical/model capabilities | XT8 has 3 physical radios, one 2.5G uplink + 3 GbE LAN ports; emulated U6IW is dual-radio with different port/PoE hardware. XT8 has no 6 GHz/Wi-Fi 7 or native PoE-out hardware. | Keep physical telemetry truthful. Software cannot add missing RF/PoE hardware. Wi-Fi 7/6 GHz are differences from newer UniFi models, not missing features of U6IW itself. |

Vendor references checked during this audit: [UniFi SSID and AP settings](https://help.ui.com/hc/en-us/articles/32065480092951-UniFi-WiFi-SSID-and-AP-Settings-Overview), [WiFi troubleshooting / AirView](https://help.ui.com/hc/en-us/articles/32064585817495-WiFi-Troubleshooting-Guide), [Channel AI](https://help.ui.com/hc/en-us/articles/37367741854743-UniFi-Channel-AI-and-Automated-WiFi-Optimization), [ASUS XT8 specifications](https://www.asus.com/us/networking-iot-servers/whole-home-mesh-systems/asus-zenwifi/asus-zenwifi-ax-xt8/techspec/), [U6 In-Wall specifications](https://techspecs.ui.com/unifi/wifi/u6-iw). Feature availability varies by UniFi hardware/firmware. The gap assessment is an inference from these sources and the reviewed implementation.

### Original recommended build order (superseded by Section 8)

1. **Reliable installation and configuration:** B1–B4, with failure fixtures promoted into meaningful regressions; coordinate B6 credential rotation separately. Package startup as a normal tracked script with migration for existing installs. Stable release only after its artifact/runtime identity and rollback pass.
2. **Trustworthy visibility:** B5 and B7–B10. Stable physical/VAP identities, actual width/power/CPU, truthful unknowns, no false sync alarm, bounded sampling, and topology/FDB freshness. Validate each radio and the wired HomeBase in the actual controller UI.
3. **Highest-value new features:** automatic/cached SSID-to-legacy-WLAN mapping (handle duplicate names and UUID/ObjectId differences; no new API token in WebUI), effective guest/VLAN reporting, and bounded client association/roam history. Prefer existing provision data or an approved off-AP broker/discovery job over broad controller credentials on each AP.
4. **Optional control:** decide whether this two-AP homelab benefits enough from UniFi configuration ownership to justify it. If so, start with a small opt-in command subset and read-back; SSID/security/VLAN provisioning and controller-driven RF optimization are a substantially larger project. Keep unsupported proprietary diagnostics and physical limitations explicit.

This audit changes documentation only. Findings and feature proposals are not implemented or deployed by this audit.


## 8. Approved Backlog and Fix Batch — 2026-10-25

This section supersedes the original feature recommendations where the operator narrowed scope. See the README backlog for the concise work list. B1–B11 retain their original audit evidence above; partial implementation does not close the remaining acceptance criteria.

### Operator decisions

- Fix correctness and reliability first. UniFi-triggered software updates, router firmware updates and broad controller-driven provisioning are deferred. Existing bridge-addon updater validation is a separate maintenance path; this batch does not implement a UniFi upgrade handler or router firmware flashing.
- Simple device commands such as reboot may be implemented through an explicit allowlist after the correctness batch. Do not execute a live reboot merely to test code: it interrupts Wi-Fi. Authentication, deduplication/replay handling and platform behavior must be tested before enabling a command handler.
- **Roaming correction:** the operator already sees roaming in UniFi. The controller can infer a move from station/AP snapshots; the absence of a dedicated bridge event producer does not mean roaming or its UI display is absent. Optional missing detail is sub-sampling timing, reason codes, authentication/DHCP failures and brief transitions. No parallel roam-event producer is planned without evidence it improves the current experience.
- **AirView correction:** AirView is also a diagnostics UI, not one universally available scanner. Passive channel/client statistics can use existing radio counters. Continuous dedicated-radio scans/spectrum functions are hardware-dependent; active scans on a serving radio can disrupt clients. Keep scanner/capture integration and proprietary optimization parity deferred; collect accurate passive data only.
- Dynamic WLAN mapping, effective guest/VLAN reporting, passive diagnostics and community packaging remain future features. Guest tagging must not be confused with isolation enforcement. Broad controller credentials should not be added to AP/WebUI status.

### Current implementation batch (local source; not deployed)

| Item | Implemented / verified locally | Still pending |
|---|---|---|
| B1–B2 addon integrity | Binary required; SHA-256 manifest enforced; archive contents restricted; staged binary/source version match required; failed script/UI copy or startup is an error; successful launcher without fresh matching PID/version/commit and Active heartbeat is not success. Local outcome is labeled separately from asynchronous cluster rollout. | Pin scripts and binary to one immutable source commit/full release bundle; transaction locking, automatic rollback and verified final outcomes on every node. Main/stable publication remains held; no claim of complete update transaction safety. |
| B3 configuration | Dependency-free Go settings helper validates submitted fields and merges into existing JSON. Preserves WLAN mapping, explicit nodes, topology and unknown fields. Private unique temp file, fsync/atomic rename, no write for semantically unchanged config, and removal of hardcoded homelab WLAN defaults from generic install/apply paths. | Validate UI path on the new binary and serialize concurrent settings/config sync operations. Old binaries reject the helper flag instead of rewriting configuration. |
| B4 lifecycle | Tracked `scripts/start.sh` replaces the embedded installer template; it and daemon startup honor enabled=false through the JSON-aware -is-enabled helper. Disabled status retains installed version/track/model even when an older startup script invokes the new daemon. The UI initializes the disabled radio button correctly, so unrelated saves do not silently re-enable it. Existing updater/cluster distributions include startup script. Disabled/re-enable fixtures cover boot/cluster/manual startup entry points without running real process controls. | Controlled live disable/re-enable/reboot persistence test after review; no such disruptions were performed here. |
| B5 radio identity | Discover all physical radios; uniquely name same-band radios and associate VAPs/stat counts with their actual parent; include configured virtual interfaces, skip explicitly disabled BSS, stable VAP names, actual widths and TX-chain-derived streams. Chanspec parser now handles 36l/40 MHz and hex center/control sidebands correctly. | Acceptance in actual UniFi UI, especially third-radio representation under the dual-radio emulated U6IW profile. No model/re-adoption or radio configuration changes have been made. |
| B7–B8 metrics | /proc/stat CPU deltas with guest-time double-count prevention and unknown first sample; MemAvailable when supported; no invented -90 noise/SNR or perfect experience on absent signal. Watchdog RAM log cap. | Whole-sample deadlines/cancellation/caching; interval-based retry quality; stable per-interface VAP counters; measured sampling overhead and stale-process recovery/backoff. |
| B9 topology/FDB | Unknown LLDP parent port stays 0 so configured fallback can work. FDB age becomes last-seen age, not fabricated connection uptime. Multiple MACs no longer select an arbitrary last connection; unknown VLAN omitted; STP state/cost read from bridge sysfs rather than guessed from carrier. | Broader LLDP subtype/format validation, cached verified parent freshness, actual VLAN discovery and full per-client connection history. |
| B10 UI | Separate healthy-but-unverified cluster state from errors; escape node text, exclude stopped nodes from active totals, show stale daemon status and unknown CPU honestly. | Structured commit/config verification, browser validation, background bounded completion for sync/settings forms (update actions already use that transport). |
| B11 / F5 command support | Support boundary documented; firmware/provisioning commands remain unimplemented. | Typed low-risk command handler, starting with reboot, with authenticated GCM-only policy and controlled read-back/deduplication tests. |

### GitGuardian incident investigation

The operator reported an X-API-Key finding pushed at **2026-10-25 05:23:30 UTC**. Independent history inspection found a literal controller key in `docs/AUDIT_STATE.md`, commit `69dbccc`, line 114 in that revision. It was removed by subsequent documentation changes, but remains in reachable Git history. A metadata-only GitHub secret-alert query returned no entries; GitGuardian is a separate detection provider, so that does not refute the alert. The exact GitGuardian finding URL/push attribution has not been independently obtained.

A comparison inside the existing secret-handling workflow confirmed that `/etc/ops-broker/unifi-api-key` on CT107 matched it and authenticated to the Integration API with HTTP 200 at investigation time. Only hashes/booleans/status codes were printed; the key was not copied into new documentation or chat. The operator explicitly approved replacement, broker verification and revocation of the exposed key. Browser certificate/login and credential-creation handoffs were completed. The replacement `ops-broker-2026-10-25` was securely transferred and verified before the operator confirmed revocation of the old `ops-broker` key. Rotation verification is recorded below.

Added a narrow literal X-API-Key header guard and regressions, including escaped JSON (the historical form), to complement GitGuardian. This is not a complete general secret scanner. Current source must pass the guard before push. Do not rewrite/force-push shared history without a separate reviewed decision: revocation is the primary remediation and historical copies/forks cannot be assumed erased.

### Verification and deployment boundary

This batch is being tested on local source, with synthetic update/startup failure fixtures and no real reboot, scan, radio reconfiguration or firmware flash. The runtime baseline remains v1.2.4 dev on both APs until an explicitly verified addon rollout is recorded. The new source version is v1.2.5 for update detection; changing version.txt alone is not proof of deployment. Full test/build results and credential-rotation completion are recorded below when available.

#### Local batch validation

Go race tests and vet passed. Cluster and update-check regressions passed (4 each), addon apply failure tests passed (5: missing binary, checksum mismatch, source/binary version mismatch, failed startup, and launcher without a verified fresh heartbeat), startup disable/re-enable tests passed (2), literal API-header guard tests passed (2), and actual WebUI-script regressions passed for updates, health labels, escaping and installed-track preservation on settings save. All shell scripts passed syntax checks. Native, Linux ARMv7 and ARM64 builds succeeded. A smoke test executed the actual shell settings handler with the compiled helper and synthetic configuration; custom mappings/nodes survived, enabled=false was retained, and the disabled daemon exited before opening a nonexistent profile or touching hardware. Firmware interface/TX-chain output was sampled read-only to validate parser forms, including `15 (0xf)` and configured VIFs beyond .3.

These checks establish local behavior, not deployment or three-radio compatibility in the controller. No v1.2.5 AP rollout, firmware update, scan or reboot was performed.

Rollout note: the currently deployed v1.2.4 updater does not acquire startup assets or enforce the new validation before its first upgrade. A future approved rollout must bootstrap the reviewed v1.2.5 script/binary set with checksum verification and explicitly install the tracked startup asset; do not assume the legacy one-click path performs the new checks. The daemon's own disabled guard/status supports older startup callers in the transition, but does not replace complete asset migration. Runtime remains v1.2.4 until that acceptance step is recorded.

#### Completed credential remediation and hosted validation

- **2026-10-25 16:07:09 UTC:** the revoked key returned HTTP **401** from Integration `/info`. The replacement, read from the installed `/etc/ops-broker/unifi-api-key`, returned **200** from `/info` and `/sites`. File mode is **0600**, original ownership preserved; `ops-broker.service` remained **active/running**. No blanket credential/service reset was performed. These are direct API and service-state checks, not a claim that an unimplemented broker UniFi backend was enabled.
- Credential comparisons were performed privately; no key values or fingerprints are published. The new secret was never printed or committed. The temporary local replacement-key file and private RAM-backed revoked-key copy were deleted after verification; the active broker credential remains in its canonical secret file. UniFi UI now lists only the new API key alongside the existing SIEM integration.
- The historical GitGuardian exposure is remediated by revocation. Git history was not force-rewritten; the GitGuardian dashboard finding status itself was not changed or independently verified.
- Source implementation: `6254fc592f892a5ad87da91ff4cdd1b3fd66f18e`. First CI caught a Linux `/tmp` path-substitution bug in the startup test fixture, not a daemon failure. Corrected in `be106ec33fe6790af30a600d11bd05246445c530`; tests now exercise a `/tmp` root on macOS as well. [Hosted validation and ARM release build 40210104250](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40210104250) passed and published the v1.2.5 dev artifacts.
- Main and the AP deployments were not changed by this source-fix batch. Live v1.2.5 radio/UI and upgrade-migration acceptance remain pending. Reboot-command support is queued, not implemented or enabled; UniFi-driven updates and firmware remain deferred.
