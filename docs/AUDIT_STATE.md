# Audit State & Current Operations

> **Current delivery status — 2026-10-25, 16:25–16:28 UTC:** both APs now run **v1.2.5 dev**, commit `be106ec33fe6790af30a600d11bd05246445c530`, after a sequential Node-then-Main migration. Fresh adopted GCM heartbeats, three radios per AP, gateway parents and HomeBase attachment were verified. Configuration files were unchanged. Credential remediation completed at 16:07 UTC. Asus settings/UI acceptance, controlled persistence/recovery tests and stable release publication remain pending. Latest stable tag was last checked as v1.2.1; main was not changed by this rollout.

**Last live verification:** 2026-10-25, approximately 09:25–09:28 PDT (16:25–16:28 UTC); see Section 9. Section 7 preserves the original code audit; Section 8 records current fixes and pending acceptance. Earlier sections retain dated rollout evidence.

**Status:** Both APs are verified on v1.2.5 dev (Section 9). Prior browser fixes remain recorded in Sections 5–6. The review found update integrity, configuration preservation, disabled-state and tri-radio gaps (Section 7); tested dev fixes and remaining acceptance are recorded in Section 8. Controller credential rotation is complete (Section 8); controlled outage testing remains open.

**Release track:** `dev`, daemon `1.2.5`, deployed code commit `be106ec33fe6790af30a600d11bd05246445c530` on both APs. Promotion of all current changes to `main` was approved by the operator on 2026-10-25. Section 5 supersedes earlier v1.2.2 UI-closure/provenance claims.

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
| A3 — Version / provenance | Runtime verified; release packaging open | Both APs run `1.2.5` / `be106ec…` on dev; runtime verified in Section 9. Main was promoted to `c8ebaa4…`; stable release remains v1.2.1. See B1–B2. |
| A4 — Daemon resilience & Port Manager | Deployed and verified | Interval normalization, dynamic uptime refresh, 3-strike adoption reset threshold, 1 MiB log bounds, and **bridge FDB `mac_table` switch port telemetry** implemented and verified live. |
| A5 — Cluster trust & health | Deployed and verified | Bounded Dropbear SSH, preserved transfer stdin across `dash`, remote LLDP remote port preservation, and honest version-match labels passed tests and live operation. |
| A6 — Flash and RAM writes | Partly resolved | Runtime telemetry and daemon logs use `/tmp`; daemon log cap is 1 MiB. State saves use atomic rename. v1.2.5 source also caps watchdog logs; rollout, stale-process recovery and backoff remain open (B7). |
| A7 — Feature backlog | Deferred; not a v1.2.2 blocker | Dynamic WLAN mapping and effective guest/VLAN reporting remain future features. Roaming already appears in UniFi; richer event detail is optional, with no duplicate producer planned. |
| A8 — Merge gate | Completed | Operator approved promotion; main and dev were pushed to `c8ebaa4…`. This did not publish a stable tag or change AP tracks. Subsequent source fixes remain on dev; see Section 8 for remaining gaps. |

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
- **Roaming:** Existing UniFi visibility is reported working. Only pursue richer timing/reasons or brief-transition detail if a concrete gap is demonstrated; do not build a duplicate event producer by default.

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
| Client history and roaming | Roaming is already visible in UniFi. Bridge snapshots lack explicit event reasons, authentication/DHCP failure detail and sub-sampling transitions. | Operator reports roaming already visible in UniFi through AP snapshots. No duplicate event producer is planned. Optional richer timing/reasons or brief-transition history requires a demonstrated gap; see Section 8. |
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

Section 7 records the original documentation-only audit before fixes. Section 8 records subsequent implementation and is the current disposition of those findings.


## 8. Approved Backlog and Fix Batch — 2026-10-25

This section supersedes the original feature recommendations where the operator narrowed scope. See the [README backlog](../README.md#prioritized-backlog-and-operator-decisions) for the concise work list. B1–B11 retain their original audit evidence above; partial implementation does not close the remaining acceptance criteria.

### Operator decisions

- Fix correctness and reliability first. UniFi-triggered software updates, router firmware updates and broad controller-driven provisioning are deferred. Existing bridge-addon updater validation is a separate maintenance path; this batch does not implement a UniFi upgrade handler or router firmware flashing.
- Simple device commands such as reboot may be implemented through an explicit allowlist after the correctness batch. Do not execute a live reboot merely to test code: it interrupts Wi-Fi. Authentication, deduplication/replay handling and platform behavior must be tested before enabling a command handler.
- **Roaming correction:** the operator already sees roaming in UniFi. The controller can infer a move from station/AP snapshots; the absence of a dedicated bridge event producer does not mean roaming or its UI display is absent. Optional missing detail is sub-sampling timing, reason codes, authentication/DHCP failures and brief transitions. No parallel roam-event producer is planned without evidence it improves the current experience.
- **AirView correction:** AirView is also a diagnostics UI, not one universally available scanner. Passive channel/client statistics can use existing radio counters. Continuous dedicated-radio scans/spectrum functions are hardware-dependent; active scans on a serving radio can disrupt clients. Keep scanner/capture integration and proprietary optimization parity deferred; collect accurate passive data only.
- Dynamic WLAN mapping, effective guest/VLAN reporting, passive diagnostics and community packaging remain future features. Guest tagging must not be confused with isolation enforcement. Broad controller credentials should not be added to AP/WebUI status.

### Implementation batch at validation time (subsequently deployed in Section 9)

| Item | Implemented / tested in dev; hosted CI passed | Still pending |
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

The paragraphs below preserve the pre-rollout validation boundary. Section 9 supersedes the v1.2.4 deployment and third-radio acceptance status; remaining tests are still open.

This batch passed local and hosted validation, including synthetic update/startup failure fixtures and no real reboot, scan, radio reconfiguration or firmware flash. The runtime baseline remains v1.2.4 dev on both APs until an explicitly verified addon rollout is recorded. The new source version is v1.2.5 for update detection; changing version.txt alone is not proof of deployment. Completed test/build results and credential remediation are recorded below.

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

## 9. v1.2.5 Sequential Rollout and Live Acceptance — 2026-10-25

**Evidence window:** approximately 16:25–16:28 UTC (09:25–09:28 PDT). The operator continued the proposed migration/acceptance work. Node was migrated and verified before Main. Only the telemetry bridge was restarted; no AP reboot, Wi-Fi/radio reconfiguration, firmware update, active scan or re-adoption was performed. Client counts are observations, not packet-loss measurements.

### Migration and preservation

- Downloaded published ARMv7 dev artifact; verified SHA-256 against its release manifest and restricted archive members. Scripts/WebUI/version were obtained from exact source commit `be106ec33fe6790af30a600d11bd05246445c530`, matching the binary's on-device version/commit/build-date output.
- Explicitly installed tracked `start.sh` and matching apply/sync/CLI/update/watchdog scripts plus WebUI and binary archive. This bypassed the old updater's missing startup/checksum guarantees without calling its update path or triggering automatic cluster propagation.
- Each AP retained a private RAM-backed prior asset/runtime backup at `/tmp/unifi-v125-migration/backup`. Migration had failure restoration logic, but rollback was not exercised. RAM backups do not survive reboot and are not a persistent recovery strategy.
- Each configuration file was byte-for-byte unchanged across migration. Existing adoption identity remained adopted with GCM; state files remained `0600`, and Main cluster private key remained `0600`. No adoption key or private credential was printed or copied into documentation.

### Verified results

| Check | Main | Node |
|---|---|---|
| Version / source | v1.2.5 dev / `be106ec` | v1.2.5 dev / `be106ec` |
| Runtime | Live matching PID; adopted, Active GCM | Live matching PID; adopted, Active GCM |
| Sample freshness | 3 seconds | 8 seconds |
| Radio clients (2.4 / 5-low / 5-high) | 23 / 3 / 3 | 19 / 8 / 4 |
| Radio channels / UI widths | 6 / 20 MHz; 36 / 40 MHz; 161 / 40 MHz | 6 / 20 MHz; 36 / 40 MHz; 161 / 40 MHz |
| Gateway parent in actual UniFi UI | Cloud Gateway Max Port 1; 2.5 GbE | Cloud Gateway Max Port 3; 2.5 GbE |
| Downstream wired link | LAN ports down in sample | Port 4 up, full-duplex 1 GbE; HomeBase in FDB and actual Port Manager |
| Bridge RSS | about 9.7 MB | about 9.6 MB |

UniFi overview rendered all three radios on each U6IW-emulated AP. This closes the specific third-radio overview acceptance gate without claiming compatibility with every UniFi radio-control/diagnostic surface. Node Port Manager retained Cloud Gateway Max on Port 1 at 2.5 GbE and Bridge-EufyHomeBase3 on Port 4 at GbE. The measured memory percentage is higher than v1.2.4 because it now uses kernel MemAvailable (approximately 38 MB Main / 78 MB Node available in a later sample); the values were checked against `/proc/meminfo`, not inferred from process RSS.

### Remaining acceptance and backlog

- Asus page requires login; browser settings-save/no-op preservation, update-check flow and cluster labels on v1.2.5 remain pending. Do not infer these from successful shell/runtime checks.
- Controlled disable/re-enable, reboot persistence, controller outage/recovery, rollback and extended sampling-overhead tests remain unperformed.
- Immutable complete release bundles, updater locking/automatic rollback and final per-node results still need implementation. The bounded manual migration does not close those generic updater gaps.
- Structured cluster config/commit verification, background settings/sync completion, sample deadlines/cache/backoff, interval retry/VAP counters, broader LLDP validation and VLAN discovery remain open.
- Simple authenticated reboot command support is queued. WLAN auto-discovery and guest/VLAN reporting remain future features. UniFi-driven upgrades, router firmware, broad provisioning and scanner/capture integration remain deferred.
- Main/stable publication was not changed. Stable release remains held pending remaining acceptance and release packaging work.

## 10. Updater Serialization Follow-up — 2026-10-25

**Source implemented and tested; not deployed:** checks and installs now acquire an atomic private RAM-directory lock before downloading or writing operation status. A competing request fails without replacing the active request's status or changing installed files. Normal exit and INT/TERM release only the owning shell's lock. Stale locks are deliberately not reclaimed automatically because PID reuse or interrupted installation requires inspection. SIGKILL/power interruption is not trap-recoverable; the RAM lock clears on reboot, but reboot is not the recommended first recovery action.

Removed the delayed background success-to-UP_TO_DATE rewrite, which could otherwise replace a subsequent request's status after lock release. The UI already handles SUCCESS directly. This is updater-only serialization, not a claim that settings saves, cluster propagation or arbitrary direct script calls share one transaction lock.

**Validation:** eight addon-apply tests passed, including real overlapping processes and a busy owner for both check/apply; failures release their lock and preserve the installed version; a successful intentionally disabled install also releases its lock. Four backend tests, existing WebUI update/health/track/disabled-state regressions, shell syntax and whitespace checks passed. No AP rollout or restart was performed for this follow-up. Both APs retain the previously accepted v1.2.5 `be106ec` deployment.

**Acceptance blocker:** Chrome reported another extension panel blocking automation on the Asus page. Browser settings-save/update-check acceptance remains pending user dismissal/login; this does not block the isolated source tests. Immutable bundles, automatic rollback, cross-operation locking and per-node final update outcomes remain open.

**Hosted result:** updater-locking implementation `ecec3952ca6b674605f7693253b40a986b458adf` passed [CI run 40212251082](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40212251082), including Go tests, shell/UI regressions and ARMv7/ARM64 builds. Dev artifacts were published. This does not change the installed `be106ec` APs or close the blocked Asus browser acceptance.

## 11. Autonomous Worker Plan — 2026-10-25

This is the execution entry point for the remaining audit work. Read this section first, the short current-status header, and only the source/tests needed for the active packet. Sections 7–10 contain supporting evidence; do not reload the full historical ledger for each task. The operator requested a token-efficient autonomous workflow; the launch contract below specifies the proposed standing scope instead of asking permission at every routine step.

### Operating model and current checkpoint

Use one fresh dedicated project session in `/Users/nithinag/workspace/unifi-ap-bridge`, with **gpt-6-luna** as the primary worker. Start with high reasoning for implementation; use lower effort only for mechanical documentation/status work. This model/effort choice is a starting recommendation, not a measured cost guarantee. Use a **gpt-6.1-sol** subagent for bounded design/review/escalation when required below. One implementation writer at a time; parallel agents are not the default. Do not run this session and a second worker against the same checkout concurrently.

A separate session does not automatically have a callback channel into this chat or automatically upgrade its own model. Explicitly permit its Sol subagent in the launch prompt. The worker can receive that review and continue without involving the operator. If the client cannot provide that model/subagent capability, record a concise escalation packet and proceed with independent work; do not pretend the escalation occurred.

- Installed baseline: both APs v1.2.5 dev, source `be106ec`; last runtime/UniFi UI evidence is Section 9, not a fresh sample at worker startup.
- Latest code milestone: updater serialization `ecec395`; [CI 40212251082](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40212251082) passed. Documentation checkpoint `72663d8`. Serialization is published in dev assets but not deployed to the APs. Read Git at startup rather than treating these hashes as permanently latest.
- Preserve unrelated `scripts/__pycache__/` and any subsequent user changes. Do not clean the checkout indiscriminately.
- Asus browser acceptance was blocked by another Chrome extension panel/login. Request the necessary human browser handoff once, then continue independent source packets; do not repeatedly poll a known blocked page.
- Main/stable release remains separate. No merging main, stable tag publication, history rewrite, credential change, AP reboot, radio/network reconfiguration, firmware flashing or new active scan is included in the worker launch contract.

### Ordered work packets

Execution status at 2026-10-25: W0 has a fresh read-only runtime sample; its Asus browser/update-check acceptance is awaiting the one requested login handoff. W1–W4 are implemented, locally validated and Sol-reviewed; CI/dev push are pending. W5 is active. Implement one packet at a time, preserving its stated boundary; combine tightly related updater packets into a meaningful CI/push checkpoint rather than pushing every edit.

| Packet / audit link | Implementation scope and likely entry files | Completion evidence | Model / dependency |
|---|---|---|---|
| W0 — Acceptance baseline (B3–B5/B10) | Fresh Main/Node runtime sample completed at 17:19 UTC; Asus login and browser update/settings acceptance await the one human handoff. | Both report PID, v1.2.5 dev, `be106ec`, fresh `Active (GCM)` status and the same deployed asset SHA. UniFi devices page shows both APs online. No live sync/disable/reboot. | Browser handoff requested once; independent source packets continue. |
| W1 — Complete immutable addon bundle (B1–B2) | Implemented locally: SHA-qualified dev releases and exact stable-tag resolution, complete architecture bundle, strict preflight and updater fixture suite. | Local ARMv7/ARM64 package generation and extraction succeeded; 14 updater fixtures and full local CI-equivalent checks passed. Sol reviewed bundle format and updater diff. Hosted CI/push pending. No live update. | Done locally; W2 follows. |
| W2 — Transactional installation and rollback (B2) | Implemented and Sol-reviewed in `92b7c3a`: stage before mutation; snapshot a verified prior complete bundle; journal activation; validate exact runtime identity, PID and fresh successful inform; recover or rollback assets on failure/interruption. Disabled installations retain disabled intent and are checked without expecting a heartbeat. | 19 updater fixtures and 6 startup/recovery fixtures pass, including copy, extraction, start and inform failures, interrupted activation and ACCEPTED cleanup. `go test -race ./...` and `go vet ./...` pass. No live update; legacy installed archives still require the reviewed W7 exact-SHA bootstrap. | Done locally; W3 follows. |
| W3 — Shared mutation coordination / per-node outcomes (B2–B4/B10) | Implemented and Sol-reviewed locally: settings, updater, startup and cluster paths follow update → mutation → start lock order, validate inherited ownership, refuse unfinished journals and release Main's snapshot lock before contacting peers. Shared config is merged while preserving node-local identity/topology. Per-node dispatch, operation ID, requested/observed version and commit, freshness, stage and outcome are separate. | 21 updater, 7 startup and 9 cluster fixtures pass, including lock contention/cleanup, no recursive receiver update, lost SSH reply with operation-ID verification, partial failure, stale status, disabled state, unreachable null state and fresh-but-stopped health. Go race/vet, shell syntax and diff checks pass. | Done locally after Sol re-review PASS; W4 follows. |
| W4 — Bounded Asus actions / structured cluster state (B10) | Implemented locally: `webui/unifi-bridge.asp`, apply/sync backend and JS/Python tests. Settings and sync use matched request IDs, background transport, per-request durable results and bounded polling. Structured node status includes reachability, freshness, commit/version, operation outcome and last successful inform. Text is escaped and numeric metrics are strictly validated. The UI never claims configuration equality: it remains explicitly unverified until a canonical comparison exists. | Timeout/error/stale response/overlap and malicious-metric fixtures pass; settings no-op retains unknown fields and installed track. Browser checks await login handoff; no credentials or secret-derived identifiers in status/logs. Sol re-review PASS. | Done locally; W5 follows. |
| W5 — Bounded sampling and watchdog recovery (B7) | `providers/provider.go`, `providers/broadcom/asus.go`, daemon exchange loop, watchdog and tests. Bound external commands and whole samples; propagate cancellation, cache static data with expiry, preserve truthful stale/unknown markers. Specify watchdog backoff and stale-process handling without restarting Wi-Fi or duplicating the daemon. | Fake slow/hung commands prove deadlines/cancellation/no process leak; no adoption reset for collection timeouts; cached-data expiry and restart backoff tests. Read-only live duration/RSS sampling after reviewed rollout; do not invent CPU guarantees. | Sol for interface/deadline design; Luna for scoped parser/cache/watchdog changes. Independent of W4 when one writer owns its files. |
| W6 — Counter / LLDP / VLAN correctness (B8–B9) | Broadcom parser/telemetry/tests. Use reset-safe interval retry metrics, stable VAP counters when driver supplies them, measured unknowns otherwise. Validate LLDP subtype/interface/port forms, cache last verified parent with freshness/expiry and link-loss invalidation. Discover effective VLAN/bridge data only from evidence; FDB age stays last-seen, never connection duration. | Fixtures for counter reset/wrap/client churn, unknown/multiple SSIDs/VLANs, LLDP failures/port changes, link loss and multiple learned clients. Read-only firmware samples; keep current topology UI behavior. | Luna for specified parsers; Sol review for stateful counter/topology semantics. W5 provides cache/deadline contracts. |
| W7 — Consolidated dev rollout / remaining acceptance (B1–B5/B7–B10) | Build a complete tested generation, conduct reviewed Node-first bridge-only rollout, verify, then Main. Use W1–W3 verified migration/rollback mechanics. Recheck affected Asus/UniFi UI and runtime features. Batch source fixes to avoid unnecessary restarts. | CI SHA = bundle SHA = deployed SHA; fresh matching PID/GCM, three radios, expected links/topology/HomeBase, config/identity preservation, per-node results and bounded resource sample. Restore known-good addon on failure, stop Main rollout if Node fails. | Sol release review; Luna executes reviewed steps under the launch contract. Human login/mandatory tool approvals cannot be automated away. |
| W8 — Small authenticated commands (B11/F5) | After correctness packets, implement an opt-in typed reboot handler only if actual controller command evidence is available. Require adopted authenticated GCM, validation, deduplication/replay policy and explicit unsupported-command diagnostics without secrets. Never run controller-provided shell strings. | Synthetic authenticated/unauthenticated/replay/unsupported-command tests with mocked platform runner; no real AP reboot in this scope. If wire command is unknown, return evidence gap rather than guessing. | Sol protocol/security review; Luna implements reviewed handler. Actual enablement/reboot acceptance remains a separate operator decision. |

WLAN ObjectId auto-discovery/cache, guest/VLAN classification beyond W6 evidence, richer optional roam detail and community packaging are a later feature tranche. Firmware, UniFi-triggered addon upgrades, broad provisioning and active scanner/capture remain deferred. Fixes first; do not silently expand this queue into new features.

### Token and review discipline

1. Read Git state and the active packet; narrow `rg` and bounded reads before opening files. Do not ask another agent to re-audit the repository or summarize this conversation. Prefer deterministic parsers/tests over model-generated interpretations of long logs.
2. One primary writer. Reuse its context within the packet; start a fresh context at a packet boundary when the ledger is enough. Do not blindly fork this long conversation. A fresh Sol escalation receives only the contract, relevant diff, failing test/evidence and exact question.
3. Escalate after two unsuccessful attempts at the same substantive failure, a missing architecture/protocol decision, an unintended scope expansion, a security/adoption risk, or evidence contradicting the plan. The two-attempt rule is a cost guard, not permission to make two risky live attempts. Trivial deterministic typos do not consume a full escalation.
4. Mandatory Sol review for bundle/install/rollback/shared-lock design, command authentication and any live rollout. Review only the changed packet, concrete invariants and tests. Agent review cannot grant new user permissions. Resolve findings and continue automatically within the standing contract.
5. Run packet-specific regressions while editing. Full relevant Go race/vet/shell/UI/cluster checks once at a coherent milestone; CI/ARM builds once the source checkpoint is ready. Re-run only changed/failed checks unless a new concern warrants broader validation.
6. Keep small logical local commits; one integration owner pushes dev after validation and review. No force-push. Docs-only results use `[skip ci]`. Do not publish new dev assets for every documentation or exploratory iteration.
7. Update this ledger with task state, commit, checks, deployed state and next action; short summaries replace raw successful logs. Keep implemented, CI-passed, published, deployed and live-accepted as separate states. A partial fix does not close its original audit acceptance criteria.
8. Continue independent packets when browser access or a decision is blocked. Ask the operator only for a specific outside-scope decision or required human handoff. Report unresolved blockers; do not end each packet by asking whether to continue.

### Proposed standing launch contract

The following is a copy/paste instruction for the dedicated worker. It becomes its explicit task authorization when the operator sends it; writing it here alone does not expand deployment/credential permissions. Sandbox, browser and organization policy still apply. Start the session in the bridge project so normal repository writes are within its permitted workspace; do not disable safeguards to avoid a misplaced workspace root.

```text
Work autonomously on /Users/nithinag/workspace/unifi-ap-bridge using Luna.
Use docs/AUDIT_STATE.md Section 11 as the execution plan. Read the current
status header and the active packet; load older evidence only when needed.

Execute W0–W7 in order where dependencies apply, continuing independent
source work if browser checks are blocked. W8 is code/tests only and follows
the correctness work. Use one writer, not a parallel agent swarm.

You may spawn a gpt-6.1-sol subagent for the bounded design/review/escalation
rules in the plan. Give it fresh minimal context, not the whole chat.
Resolve its findings and continue; do not ask me to broker routine handoffs.
If subagents are unavailable, record the exact escalation and continue safe
independent work rather than claiming the review was completed.

I authorize scoped code/docs/tests, local commits and validated dev pushes.
I also authorize reviewed Node-first then Main updates of this existing
telemetry addon after CI passes, bridge-daemon restarts needed by that rollout,
and restoring the verified previous addon if acceptance fails. Preserve
config, adoption state, SSH keys and actual Wi-Fi/radio/network settings.
Stop a failed canary from propagating; verify recovery before further rollout.

Do not merge main, publish a stable tag, force-push, rotate credentials,
reboot either AP, change Wi-Fi/radio/VLAN configuration, flash firmware,
enable a new UniFi control handler, or run active scans. Those are outside
this standing authorization. Browser security/login handoffs and mandatory
platform approvals still require me when the tool requires them.

Do not ask me to approve each normal implementation/test/dev-push step.
Ask only for a specific outside-scope decision or unavoidable human handoff;
continue independent work meanwhile. Update the canonical ledger after each
milestone. Finish with done/pending/blocked status and deployed commit IDs.
```

### Why this setup

One dedicated Luna session minimizes repeated parent/worker analysis for this largely sequential backlog. Sol is used where a stronger bounded review can prevent costly retries or unsafe rollout decisions, rather than kept reasoning on every shell action. Same-session subagents remain a useful alternative if automatic parent escalation is the priority, but every active agent adds model/tool work; parallelism primarily reduces elapsed time, not guaranteed token usage. This is a workflow recommendation, not a measured token-saving percentage.

Official guidance: [Codex subagents, model choices and inherited permissions](https://learn.chatgpt.com/docs/agent-configuration/subagents). No persistent worker, cross-session router, automatic model switch or expanded permissions were configured by this documentation change.

## 12. Autonomous Launch Execution — 2026-10-25

**W0 runtime sample (17:19 UTC):** read-only SSH checks reached both APs after sandbox network access was authorized for the standing check. Main reported PID `13149`; Node reported PID `10759`. Both status files reported v1.2.5, dev, source `be106ec33fe6790af30a600d11bd05246445c530`, `Active (GCM)`, and current status timestamps. Both installed `unifi-bridge.tar.gz` assets had SHA-256 `59ec1f93afe43a172a8c6e72796df6e8c5ad452c3e83b9214cae2dd04e5109e5`. No daemon was restarted and no configuration or network state was changed.

The authenticated UniFi devices page showed both XT8 APs online, with Main connected on Gateway Port 1 and Node on Gateway Port 3, each at 2.5 GbE. The Asus page redirected to its login screen. The requested one-time browser handoff is pending; update-check, installed-track, health-label and no-op/custom-field browser acceptance therefore remain open. No new live sync, disable, reboot or radio/network action was run.

**W1 implemented and locally reviewed; not deployed:** CI now creates one flat, architecture-specific bundle with the binary, installer/startup/runtime scripts, WebUI, version and JSON source metadata. Dev builds publish SHA-qualified release tags targeted at the source commit; stable builds reject a tag/version mismatch. The updater resolves a source commit and exact release, downloads only that generation's bundle/checksum, checks the complete member list and regular-file types, validates checksum, architecture, version and full source SHA against the executable, and rejects an unsuccessful `-version` command before persistent addon mutation. Existing standalone binaries, legacy archives and `sha256sums.txt` are still published for the transition. The first migration from already-installed v1.2.5 remains a reviewed exact-SHA bootstrap at W7; neither AP has received these source changes.

Sol reviewed the format/migration invariants and the implementation. Review findings on release-tag targeting and staged-binary exit status were fixed, re-reviewed and covered by fixtures. Fourteen updater fixtures pass, including bad or duplicate manifests, moved-ref pinning, wrong SHA/version/architecture, missing/extra/duplicate members, unsafe paths, symlink members and a matching-output binary that exits unsuccessfully. Local ARMv7/ARM64 binaries were generated and complete bundles extracted with expected metadata and members. `go test -race ./...`, `go vet ./...`, the workflow Python regressions, WebUI regression suite, shell syntax and secret checks pass using Go caches redirected under `/private/tmp`.

**W2 implemented and Sol-reviewed locally; not deployed:** the updater now stages and validates the full bundle before activation, takes an integrity-checked snapshot of the prior installed generation, commits assets under a durable transaction journal, verifies the matching executable/PID and fresh successful inform, and restores the prior assets on failed activation. Startup can recover interrupted `ACTIVATING` and `ACCEPTED` journals; disabled configurations retain disabled status without spawning a process. The Sol review found no remaining P1/P2 issues after follow-up fixes.

**W3 implemented and Sol-reviewed locally; not deployed:** mutation paths coordinate through validated owner records and bounded lock waits. The lock order is update → mutation → start, cluster propagation holds no Main mutation lock while waiting on a peer, and exact receiver updates do not recursively fan out. Shared settings merge through an explicit field allowlist, preserving node-local topology, identity and unknown fields. Cluster JSON separates dispatch from verified completion and reports per-node operation ID, requested/observed version and commit, freshness, stage and outcome; a fresh but stopped enabled node remains unknown, and unreachable enabled/running fields are null. The Sol review identified and closed lock cleanup, remote PID-query quoting, health verification, recursive propagation and unreachable-state issues. W3 fixtures pass (21 updater, 7 startup, 9 cluster); Go race/vet and shell syntax checks pass.

**W4 implemented and Sol-reviewed locally; not deployed:** settings actions bind a validated request ID to the captured JSON payload before waiting on the mutation lock. Settings and sync each write atomic per-request results, retain a bounded history, and distinguish saved state from runtime/node verification. Background UI requests correlate the response and status by ID; dispatch loss and timeout stay unknown without automatic resubmission. The cluster table escapes remote text, strictly validates numeric metrics, and reports operation stage/outcome, freshness, requested/observed version and commit, disabled state and last successful inform. The badge reports health only when every reported node has fresh running or disabled health; otherwise health remains unverified. Configuration equality always remains unverified because a canonical shared-config comparison has not been implemented. Sol re-review PASS. UI, backend (7), sync (10), updater (21), startup (7), Go race/vet, shell syntax and diff checks pass. The first Go check used the host default build cache and failed on cache permissions; rerunning with `GOCACHE` under `/private/tmp` passed.

W0 browser acceptance remains pending the user's login handoff. W5 is next. Hosted CI/dev publication, exact-SHA rollout, and controlled persistence/recovery checks remain pending.
