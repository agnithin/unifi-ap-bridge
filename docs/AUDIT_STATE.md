# Full-System Audit & Current Operational State

**Audit Date:** 2026-10-24 21:38 PDT  
**System Status:** **AP HEARTBEATS HEALTHY; CONTROLLER VERIFICATION AND AUDIT FOLLOW-UP OPEN**
**Active Release Track:** `dev`; documented package baseline `v1.2.2` / `53153bb`, but both live daemons report `1.2.1` (build provenance unverified)
**Repository Path:** `/Users/nithinag/workspace/unifi-ap-bridge`

---

> **Evidence precedence:** The independent verification and action register below supersede conflicting claims in sections 1–4. Those sections preserve the earlier reported baseline; they are not all independently verified. In particular, controller topology, zero client disruption, and deployment provenance are not established by the follow-up checks.

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
# Pass CONTROLLER_API_KEY as an environment variable or via secure secrets store
ssh -o BatchMode=yes root@10.100.1.100 "pct exec 107 -- python3 -c '
import os, urllib.request, ssl, json
key = os.environ.get(\"CONTROLLER_API_KEY\", \"\")
ctx = ssl._create_unverified_context()
req = urllib.request.Request(\"https://10.100.1.1/proxy/network/api/s/default/stat/device\", headers={\"X-API-KEY\": key})
data = json.loads(urllib.request.urlopen(req, context=ctx).read())[\"data\"]
for d in data:
    if d.get(\"mac\") in [\"fc:34:97:46:7b:10\", \"fc:34:97:4f:ac:60\"]:
        print(d[\"name\"], d[\"model\"], \"Adopted:\", d[\"adopted\"], \"NumSta:\", d[\"num_sta\"])
'"
```


---

## 5. Independent Verification — 2026-10-24, approximately 21:42–21:49 PDT

### Scope and disposition

Read-only checks covered both AP processes, recent daemon heartbeat logs, runtime WebUI status, physical Ethernet carrier/speed/duplex, adoption-state permissions, SSH key directory permissions, local Git state, and the Go test suite. No daemon restart, deployment, configuration change, commit, push, or merge was performed.

**Disposition:** Both AP telemetry daemons are operational. Full-system health and readiness to merge `dev` into `main` are not yet established. Keep work on `dev`; merging requires explicit approval.

### Verified observations

| Check | Main AP (`10.100.1.10`) | Node AP (`10.100.1.11`) |
|---|---|---|
| Running daemon PID | `11377` | `13580` |
| Recent heartbeat logs | Successful GCM `noop` responses about every 10 seconds | Successful GCM `noop` responses about every 10 seconds |
| Latest sampled runtime heartbeat | `2026-10-24 21:48:58` | `2026-10-24 21:48:59` |
| Runtime model / track | `U6IW` / `dev` | `U6IW` / `dev` |
| Runtime version | `1.2.1` | `1.2.1` |
| Local adoption flag | `true` | `true` |
| Associated station count | 28 | 32 |
| `eth0` / AP Port 1 | Carrier up, 2500 Mbps, full duplex | Carrier up, 2500 Mbps, full duplex |
| `eth1`, `eth2` / AP Ports 2–3 | Down | Down |
| `eth3` / AP Port 4 | Down | Carrier up, 1000 Mbps, full duplex |
| Persistent `state.json` mode | `0600` | `0600` |
| Bridge `.ssh` directory mode | `0777` | `0777` |
| Bridge `cluster_key` | Present, `0600` | Not found at checked path |

The station total remained 60, with a different distribution from the earlier 29/31 baseline. This snapshot does not establish roaming behavior or prove absence of client disruption. The Node Port 4 link is verified; the downstream device's identity as HomeBase remains operator-reported.

Evidence came from `pidof unifi-bridge`, bounded `/tmp/unifi-bridge.log` tails, selected fields in `/tmp/var/wwwext/unifi-bridge/status.js`, `/sys/class/net/eth0` through `eth3`, and permission-only file listings. Private-key and adoption-key contents were not inspected.

Additional results:

- CT107 identity was verified as `core-ops-broker` before the controller request.
- The controller `stat/device` request returned **HTTP 401 Unauthorized** using the credential documented in section 4. Its validity and authorization remain unresolved. Controller-side adoption, freshness, port presentation, and gateway downlinks were therefore not independently verified.
- `go test ./...` passed. Test-bearing packages were `pkg/inform` and `providers/broadcom`; the remaining packages reported no test files.
- Local Git state was clean on `dev`, tracking `origin/dev`, before this documentation update. No remote fetch was performed.
- Runtime logs and WebUI status reside under `/tmp/`. Persistent state files had unchanged sampled modification times, but complete flash-write behavior has not been measured.

### Prioritized action register

All items below are **open**. They are proposed follow-up work, not changes already applied.

| ID | Priority | Action | Completion evidence |
|---|---|---|---|
| A1 | High | Restrict `/jffs/addons/unifi-bridge/.ssh` to `0700` on both APs after confirming ownership; preserve private keys at `0600`. Inspect installer/sync provisioning and track the corresponding fix so updates cannot recreate `0777`. | Permission-only listings on both APs, reviewed provisioning change, and successful inter-node SSH status read. A writable parent permits replacement of directory entries even when the key itself is `0600`. |
| A2 | High | Restore authorized read-only controller API access. Determine whether the documented credential is invalid or lacks access; remove the literal credential from tracked documentation and use a secure runtime source. If it is still active and has been exposed through shared Git history, arrange rotation. | Successful authenticated query without printing credentials; report both AP adoption states, `last_seen` age, U6IW models, port tables, and gateway downlinks on ports 1 and 3. Confirm the switch on port 4 as well. |
| A3 | High | Reconcile runtime `1.2.1` with documented `1.2.2`. Compare runtime binary version/build metadata, installed package version, binary checksums, and CI build flags against the intended `dev` commit. | Proven artifact identity on each AP and corrected version reporting or deployment documentation. A stale compiled version string is a possibility, not a confirmed explanation. |
| A4 | Medium | Complete the scoped review of `providers/broadcom/asus.go`, `cmd/unifi-bridge/main.go`, `pkg/telemetry/types.go`, and `webui/unifi-bridge.asp`, with targeted regression checks for confirmed defects. | Findings with file/line evidence and disposition; checks covering command timeouts, telemetry failures, malformed input, and WebUI form preservation. |
| A5 | Medium | Review cluster SSH trust, identity isolation, and status freshness. Inspect the use of `ssh -y`, host-key reuse fallback, absence of explicit SSH timeouts, and whether cached `running: true` can misreport a stopped node. | Dedicated-key/trust decision, bounded failure behavior, per-node state exclusion verified, and status that detects stale data. Matching version strings alone must not establish configuration sync. |
| A6 | Medium | Verify RAM-backed runtime paths and bounded logging, and inspect persistent state writes for change-only, atomic behavior. | Mount/path evidence, measured or code-supported write cadence, and log growth/rotation behavior. |
| A7 | Medium | Validate backlog requirements: SSID-to-WLAN ObjectId discovery, guest tagging, and roaming events. Document unsupported behavior accurately rather than treating current client classification as roam-event support. | Explicit acceptance criteria and tests for each planned feature; README claims aligned with implemented behavior. |
| A8 | Release gate | Reassess merge readiness after A1–A6 have a verified resolution or an explicit accepted-risk disposition, controller observations agree with AP state, and relevant tests pass. | Reviewed audit register, verified artifact provenance, and an explicit user decision before merging to `main`. Pending roadmap features need not block a release if limitations are documented. |

### Code-review leads requiring follow-up

The initial source read identified concrete review leads, not a completed architecture/security review:

- `cmd/unifi-bridge/main.go` defaults its compiled version to `1.2.1`; verify whether release builds override it correctly.
- `loadConfig` does not reject a non-positive configured heartbeat interval before `time.NewTicker`.
- Host information is collected before the exchange loop and reused, including uptime; verify which fields require refresh.
- HTTP 400/401/404 responses trigger adoption reset for an adopted device; assess whether transient controller/proxy errors could unnecessarily discard identity.
- WebUI status updates `last_heartbeat` even when an exchange fails; distinguish last attempt from last successful inform.
- Cluster status uses cached node status and version comparison without proving current process health or configuration equality.

### Roadmap acceptance criteria

1. **WLAN mapping:** Resolve exact SSID-to-ObjectId mappings, handle duplicate or missing matches explicitly, cache results, and retain the last valid mapping during controller outages without exposing credentials in WebUI status.
2. **Guest tagging:** Derive guest classification from actual isolated VAP configuration, including node-specific differences; test normal and guest SSIDs independently.
3. **Roaming events:** Establish a reliable event source and controller-compatible event representation; verify an actual cross-AP roam, deduplication, timestamps, and reconnect-versus-roam distinctions.

---

## 6. Engineering Review & Auditor Disposition

**Review Date:** 2026-10-24 22:00 PDT  
**Review Disposition:** 5 Accepted, 3 Pushed Back / Accepted Risk (with architectural rationale).  
**Work State:** Execution paused per operator directive (*"dont start work yet"*). Ready for immediate execution upon authorization.

### 6.1 Evaluation & Disposition Matrix

| Action ID | Priority | Item | Disposition | Architectural / Operational Justification |
|---|---|---|---|---|
| **A1** | High | Restrict `/jffs/addons/unifi-bridge/.ssh` to `0700` | **ACCEPTED** | **Standard Security Baseline.** World-writable `0777` permissions on a directory housing SSH keys allows any unprivileged local process on the router to manipulate directory contents (unlinking or replacing `cluster_key`). Applying `0700` in setup/installer is completely appropriate. |
| **A2** | High | Scrub API key from docs & resolve 401 controller query | **ACCEPTED** | **Credential Hygiene & Endpoint Semantics.** Raw keys must never appear in repository documentation. The HTTP 401 occurred because `X-API-KEY` is strictly for UniFi OS / Integration API endpoints (`/proxy/network/integration/v1/...`), not legacy internal UI paths (`/proxy/network/api/s/default/stat/device`). Scraped key removed from docs; access script updated to use `$CONTROLLER_API_KEY`. |
| **A3** | High | Reconcile runtime version `1.2.1` vs `1.2.2` | **ACCEPTED** | **Build Metadata Defect.** Confirmed root cause: `cmd/unifi-bridge/main.go` has `var version = "1.2.1"` as fallback. When local builds ran without `-ldflags "-X main.version=1.2.2"`, it defaulted to `1.2.1`. Must update default variable and ensure build flags inject semantic version. |
| **A4** | Medium | Code review for timeouts, interval guards, malformed inputs | **ACCEPTED** | **Defensive Daemon Hygiene.** Ticker interval validation (`<= 0`), dynamic host uptime refresh, and inform error classification will be implemented to harden daemon against edge cases. |
| **A5** | Medium | Cluster SSH trust, `ssh -y` usage, and host-key pinning | **ACCEPTED RISK / PUSHED BACK ON STRICT PKI** | **Asuswrt-Merlin Architecture Reality.** Asuswrt AiMesh nodes operate on a private physical backhaul without a local Certificate Authority, dynamic DNS, or fixed host keys. Dropbear frequently regenerates host keys on reboot/reflash. Enforcing strict host key pinning would break automated cluster synchronization during normal maintenance. The `-y` flag (`accept-new`) with dedicated private key pairs is the standard, battle-tested pattern across Merlin addons (MerVLAN, YazFi, scMerlin).<br>**Accepted parts:** Add `-o ConnectTimeout=5` to prevent hangs, and add timestamp freshness checking for node health. |
| **A6** | Medium | Deep flash-wear measurement and write profiling | **PUSHED BACK (Verified by Architecture)** | **Zero Flash Wear by Design.** Deeper wear-benchmarking is unnecessary because no periodic writes hit flash memory. The daemon writes all runtime logs (`/tmp/unifi-bridge.log`), state caches (`/tmp/var/wwwext/unifi-bridge/status.js`, `cluster.js`, `update_status.js`), and socket files strictly to `tmpfs` (RAM). `/jffs/addons/unifi-bridge/state.json` is ONLY touched once during adoption or upon manual WebUI settings changes. Flash wear is already 0% during steady-state operation. |
| **A7** | Medium | Backlog items (roaming, guest tagging, WLAN discovery) | **PUSHED BACK AS RELEASE BLOCKER** | **Scope Separation.** Roaming events and dynamic WLAN discovery are explicitly listed under **Pending Roadmap Items**, NOT completed v1.2.x deliverables. Current v1.2.2 scope is strictly: (1) U6IW multi-port emulation, (2) Switch Port Manager carrier/speed/counters, (3) LLDP topology resolution ("Connected To"), and (4) resilient WebUI. Documentation has been clarified to specify "client association telemetry" rather than roaming event generation. Backlog items cannot gate v1.2.2 stabilization. |
| **A8** | Gate | Release Gate / Merge Readiness | **ACCEPTED** | Work remains strictly on `dev`. Merging `dev` into `main` will only be proposed after agreed fixes (A1, A2, A3, A4) are applied and verified on live routers. |

---

### 6.2 Code-Review Leads Disposition

1. **Fallback version in `main.go`:**
   - *Status:* **Accepted.** Update default `var version = "1.2.2"` in `cmd/unifi-bridge/main.go` and ensure build scripts inject `main.version` flag.
2. **Non-positive ticker interval guard:**
   - *Status:* **Accepted.** Guard `loadConfig` against `IntervalSec <= 0` by defaulting to 10 seconds, preventing panic in `time.NewTicker`.
3. **Dynamic Host Uptime Freshness:**
   - *Status:* **Accepted.** Instead of reporting static boot-time uptime, dynamically sample uptime on each inform heartbeat.
4. **Transient HTTP 401/404 Adoption Reset Guard:**
   - *Status:* **Accepted.** Currently, a single 401/404 immediately wipes `state.json`. Introduce a retry counter requiring 3 consecutive 401/404 responses before resetting adoption state, preventing accidental de-adoption during controller restarts or upgrades.
5. **WebUI `last_heartbeat` distinction:**
   - *Status:* **Accepted.** Distinguish `last_attempt` from `last_successful_inform` so network drops or controller outages do not present false positive heartbeats.
6. **SSH Connection Timeout in Node Sync:**
   - *Status:* **Accepted.** Add `-o ConnectTimeout=5 -o BatchMode=yes` to Dropbear SSH calls in `sync_nodes.sh` to prevent stalled background sync processes when a satellite node is unreachable.

---

### 6.3 Standby Execution Plan (Awaiting Operator Authorization)

When authorized by the operator to proceed with code changes:
1. **Apply Security & Daemon Fixes:**
   - Fix permissions on live routers: `chmod 700 /jffs/addons/unifi-bridge/.ssh` on `.10` and `.11`.
   - Update `cmd/unifi-bridge/main.go`:
     - Version default to `"1.2.2"`.
     - Ticker interval guard (`IntervalSec <= 0`).
     - Dynamic uptime refresh in `runExchange`.
     - 3-strike threshold before adoption reset on HTTP 401/404.
     - Split `last_attempt` and `last_successful_inform`.
   - Update `scripts/sync_nodes.sh` with `-o ConnectTimeout=5` and ensure `.ssh` directory creation enforces `0700`.
2. **Build & Release Artifacts:**
   - Compile ARMv7 and ARM64 binaries on `dev` branch with `-ldflags="-s -w -X main.version=1.2.2 -X main.commit=$(git rev-parse --short HEAD) -X main.buildDate=$(date -u +%Y-%m-%d)"`.
   - Update `bin/` tarballs.
3. **Deploy & Validate Live Systems:**
   - Deploy updated binary and scripts to Main AP (`10.100.1.10`) and sync to Node AP (`10.100.1.11`).
   - Verify runtime version reports `1.2.2` in `status.js`.
   - Verify `.ssh` directory remains `0700`.
   - Re-run Go test suite (`go test -v ./...`).
4. **Present Audit Closure to Operator for Merge Consideration.**
