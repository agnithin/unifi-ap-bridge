# Full-System Audit & Current Operational State

**Audit Date:** 2026-10-24 21:38 PDT  
**System Status:** **DEV UPDATE DEPLOYED; AP AND CONTROLLER HEARTBEATS HEALTHY; TOPOLOGY FOLLOW-UP OPEN**
**Active Release Track:** `dev`, live `1.2.2`, build commit `1b6ce07affb18f30ef86a822e5939b9217b3648f` (verified on both APs)
**Repository Path:** `/Users/nithinag/workspace/unifi-ap-bridge`

---

> **Evidence precedence:** The deployment evidence in section 8 supersedes earlier live-state and pending-deployment statements. Section 7 records implementation review; section 5 preserves the earlier independent live sample. Those sections preserve the earlier reported baseline; they are not all independently verified. In particular, controller topology, zero client disruption, and deployment provenance are not established by the follow-up checks.

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


---

## 7. Implementation Review — 2026-10-24, after operator authorization

**Authorization:** Operator requested evaluation of the worker's pushback and implementation of fixes. Local source changes are in progress on `dev`; the former standby directive is superseded. Live daemon deployment, artifact replacement, permission changes, and merging have not been performed by this implementation pass.

### Evaluation of worker pushback

- **A5 — Partly reasonable.** Keeping a single Dropbear `-y` (trust unknown host keys) is proportionate for this homelab. Live `ssh -h` confirms that a second `-y` would disable checking; these are different behaviors. SSH host-key trust does not require a certificate authority or DNS. The assertions that keys routinely regenerate on ordinary reboot and that the backhaul is exclusively private are not independently established. Strict PKI was never required by the audit. Existing fallback identities are preserved; dedicated-key migration remains a separate decision.
- **A5 timeout proposal — Corrected for firmware compatibility.** Dropbear v2025.89 on Main lists no `ConnectTimeout` option, and BusyBox has no `timeout` applet. The implementation uses supported `BatchMode`, keepalive/idle options, plus a shell watchdog bounding the full SSH operation (30 seconds for status, 120 for transfers). A live probe also found no `stat` command; mode checks use available `ls`/`awk` and skip redundant chmod operations during periodic status collection.
- **A6 — Accept the reduced measurement scope, reject the absolute claim.** No flash-wear benchmark is needed. Source review shows `StateManager.Save` is invoked when controller responses change identity or `cfgversion`, and on adoption reset; it is not limited to first adoption or manual settings edits. Temporary-file rename already provides atomic replacement. Runtime files under `/tmp` reduce flash writes, but an unbounded log still consumes RAM. Daemon log output is now capped at 1 MiB; watchdog logging and broader write-path review remain follow-up work.
- **A7 — Reasonable.** WLAN discovery, guest tagging, and roam events are future features. They do not gate v1.2.2 if limitations are documented accurately. Keep their acceptance criteria in section 5.
- **A2 — API mismatch confirmed live.** The existing documented key successfully authenticated to `/proxy/network/integration/v1/info` and `/sites` through CT107. The legacy endpoint's 401 did not establish that the credential was invalid. Removing the literal from the current document does not revoke the active key or remove it from Git history. Rotation remains open and requires a coordinated credential change.
- **A3 — Root cause was overstated.** The stale fallback `1.2.1` is confirmed in source. Prior build flags and deployed binary provenance were not proven; do not label that as a confirmed explanation for the installed artifact. CI already injected version metadata. Local defaults and package build commands are corrected, with runtime commit/build-date fields added for future verification.

### New controller-side evidence

Read-only official Integration API checks at approximately **22:09 PDT** established:

- UniFi Network application version: `10.6.109`.
- Main and Node: `ONLINE`, model `U6 IW`; heartbeat timestamps `2026-10-25T05:09:28Z` and `2026-10-25T05:09:29Z`.
- Controller AP ports match physical samples: Port 1 at 2500 Mbps on both; Node Port 4 at 1000 Mbps; remaining AP LAN ports down.
- Gateway and Flex switch are `ONLINE`.
- Both AP detail responses have **`uplink: null`**. The official response therefore does not establish the claimed parent gateway/port mapping. This may be an API representation limitation or a topology defect; inspect the controller topology presentation and relevant live LLDP/payload evidence before declaring resolution.

The query printed only selected device/port/heartbeat fields. The existing key was loaded privately from the earlier documentation for this bounded verification and was not printed or copied to a new file. TLS certificate validation was disabled for these diagnostic checks as in the earlier audit; the documented reusable example now expects a trusted CA file.

### Fixes prepared locally

| Area | Change | Remaining verification |
|---|---|---|
| Permissions (A1) | Installer, updater, and cluster script enforce addon directory `0755` and bridge `.ssh` `0700`, including remote sync/update paths. Protecting only `.ssh` while its parent stays `0777` is insufficient. | Apply to both APs and verify ownership, modes, and cluster access. Existing key file modes remain unchanged. |
| Version/provenance (A3) | Default version `1.2.2`; WebUI status includes commit/build date; CI builds the full command package so helper files are compiled. | Deploy identified artifacts, compare checksums, and verify runtime metadata on both APs. |
| Daemon resilience (A4) | Normalize non-positive configured intervals to 10 seconds; update uptime from each system sample; require three consecutive HTTP 401/404 rejections before adoption recovery. HTTP 400 no longer resets identity. | Targeted regressions plus live outage/recovery observation after deployment. Three rejections remain a recovery policy, not cryptographic proof of forgetting. |
| Accurate heartbeat status (A4) | `last_attempt`, `last_successful_inform`, and epoch `status_updated_at`; legacy `last_heartbeat` now means last successful inform. Failed attempts no longer fabricate success; WebUI surfaces inform errors. | Live failure/recovery verification; controller and AP clocks checked for consistency. |
| Key validation (A4) | Reject malformed non-hex or wrong-length auth keys from controller responses and persisted state rather than using a bootstrap fallback in adopted mode. | Regression tests preserve the existing identity on malformed responses. |
| Cluster health (A5) | Bound SSH operations; require a live remote PID and fresh status on the node's own clock; stale legacy status is conservatively unhealthy. Matching versions are labeled `Version Match (Config Unverified)`, not `In Sync`. | The new wrapper successfully performed a Main-to-Node PID read on actual Dropbear without installing files or restarting anything. Full installed status/sync validation remains pending. Timestamp freshness tracks daemon sampling; inform success is reported separately. |
| RAM logging (A6) | Bound regular-file daemon logs to 1 MiB, preserving terminal/pipe output; oversized events retain their bounded tail. | Confirm bounds and useful recent logs on the AP; broader watchdog/write review remains open. |
| CI | Run Go regressions, cluster watchdog/freshness tests, and shell syntax checks before ARM builds. | Hosted CI on a future authorized push; local validation recorded below. |

### Validation and release disposition

Local validation passed: Go tests with the race detector, Go vet, three cluster regression tests (including hung SSH termination, preserved transfer stdin, and stale/stopped node detection), shell syntax checks, and extracted WebUI JavaScript syntax checking. ARMv7 and ARM64 command-package builds succeeded with metadata `1.2.2`, `3107678-audit-fixes`, and UTC build date `2026-10-25`. The commit label explicitly identifies uncommitted audit fixes; it does not claim those changes are part of commit `3107678`.

Build outputs remain in `/private/tmp/unifi-fixes-9oj5polz/artifacts`; tracked `bin/` artifacts have not been overwritten. Final rebuilt artifact checksums are recorded in that directory's manifest. Hosted CI and live deployment validation are still pending.

**Release status:** Keep `dev`. Source fixes do not close live deployment verification. A2 credential rotation and topology investigation remain open; A4 full architecture review is not claimed complete. Do not merge to `main` without explicit approval.


---

## 8. Development Build Deployment — 2026-10-24, approximately 22:20–22:23 PDT

**Operator authorization:** Commit and push changes to the dev build and update the addon to the latest version. No merge to `main` was performed.

### Published source and build

- `5a40d05`: daemon, cluster, permission, key validation, logging, CI, and audit fixes.
- The first hosted run caught a real portability defect: dash discarded transfer stdin for a background SSH command despite `<&0`. The APs were not updated from that failed build.
- `1b6ce07`: preserve caller stdin on file descriptor 3 before launching background SSH. Local tests and the Ubuntu CI regressions passed afterward.
- Successful build/release: https://github.com/agnithin/unifi-ap-bridge/actions/runs/40165119811
- The published ARMv7 archive matched the release SHA-256 manifest. Running its version command before installation reported `1.2.2`, full commit `1b6ce07affb18f30ef86a822e5939b9217b3648f`, build date `2026-10-25` (UTC).

### Installation and verification

Main's existing updater ran `update.sh apply dev`, restarted the telemetry bridge, and propagated the package/scripts/configuration to Node through the updated cluster script. Rollback archives were created first on each AP at `/tmp/unifi-bridge-before-dev-1b6ce07/addon.tar.gz` inside private directories. These are RAM-backed temporary backups and will not survive reboot; they contain sensitive addon state and must remain private.

| Check | Main (`10.100.1.10`) | Node (`10.100.1.11`) |
|---|---|---|
| Runtime version | `1.2.2` | `1.2.2` |
| Runtime commit | `1b6ce07affb18f30ef86a822e5939b9217b3648f` | Same |
| Release track | `dev` | `dev` |
| New daemon PID | `8418` | `15893` |
| Adoption / cipher | Adopted, GCM | Adopted, GCM |
| Recent logs | Repeated successful `noop` heartbeats | Repeated successful `noop` heartbeats |
| Station count | 28 | 32 |
| Addon / SSH directory modes | `0755` / `0700` | `0755` / `0700` |
| Persistent state file mode | `0600` | `0600` |
| Installed script, WebUI, version checksums | Match committed source | Match committed source |
| Controller state / model | `ONLINE` / `U6 IW` | `ONLINE` / `U6 IW` |
| Controller heartbeat age at final query | 5.2 seconds | 2.2 seconds |
| Controller Port 1 | UP, 2500 Mbps | UP, 2500 Mbps |
| Controller Port 4 | DOWN | UP, 1000 Mbps |

All seven checked installed files matched local committed source via SHA-256: apply handler, cluster script, CLI, updater, watchdog, WebUI ASP, and version file. The state files retained their pre-update sampled modification times; no re-adoption was needed. A total of 60 associated stations was observed before and after; this is not a packet-loss or zero-disruption measurement.

The cluster table briefly showed cached pre-restart status during the update. After both new daemons produced fresh telemetry, Main's `sync_nodes.sh status` regenerated the view: both nodes `RUNNING` at `1.2.2`, with Node labeled `Version Match (Config Unverified)`. No production Wi-Fi service restart was requested; only the telemetry addon was restarted.

### Remaining open work

- **A1/A3:** Permission enforcement and deployed version/provenance now verified on both APs.
- **A2:** Official API access and current-document credential removal verified. Rotation of the previously exposed, still-active credential remains open.
- **A4/A5:** Implemented regressions and normal live operation passed. Controlled outage/recovery tests and the wider architecture review remain open; do not claim them complete.
- **A6:** Daemon log cap is deployed. Watchdog log growth and broader persistent-write review remain open.
- **Topology:** The final official device responses still return `uplink: null` for both APs. Gateway parent-port topology remains unresolved despite correct physical/controller port links.
- **Release:** Keep `dev`. No approval to merge into `main` has been given. This deployment record is documentation-only and does not change the published build's code identity.
